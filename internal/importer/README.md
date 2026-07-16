# internal/importer

Turns files on disk into catalog rows. This is the ingest engine: it walks a
source, figures out what each file *is* relative to what the catalog already
knows, extracts metadata, and writes everything the catalog needs — durably,
concurrently, and idempotently. Thumbnails are deliberately NOT its job (D25):
they are enrichment artifacts, produced post-commit by `internal/enrichment`.

This README is the reference for the pipeline. The `D<n>` references below point at
[`docs/decisions.md`](../../docs/decisions.md).

## The five ideas that explain everything else

Most of the code makes sense once these land:

1. **Identity is minted, not intrinsic (D9).** A file's path, name, size, and
   bytes are all mutable, so we don't key an asset on any of them. Ingest mints a
   UUIDv7 the first time it sees new content; every later encounter is a
   *matching* problem, not a creation. The rules that match a file back to its
   existing identity are the **identity matrix** (see below).

2. **Observations vs. judgments (D8).** Columns the filesystem owns (path, size,
   EXIF, `file_status`…) are *observations*; columns the user owns (rating, flag,
   label, note, deletion) are *judgments*. **Ingest writes observations only.**
   It literally cannot touch a judgment column — the `Importer` holds writer-scoped
   interfaces (`AssetObservationWriter`, `AssetDerivedWriter`) that have no
   judgment method. This is why a re-import can never clobber your ratings.

3. **Events are hints; truth is re-derived.** "This file changed" only means "go
   look." We never trust an event's claim — we re-scan, re-hash, and re-run the
   matrix. Idempotency is what makes this safe: running twice on an unchanged tree
   writes nothing.

4. **Committed = identity + observation complete; enrichment converges (D25).**
   An asset commits as soon as ingest knows what it is and what the filesystem
   says about it. Derived artifacts (thumbnails, signals) are produced
   post-commit by the enrichment engine — a NULL derived column IS the pending
   state, honestly renderable (EXTRACT already yields dimensions/orientation
   for a correctly-shaped placeholder cell), and every eligible asset converges
   to enriched or to a durable `enrichment_errors` row, never an ambiguous
   blank. This supersedes D12's "commit only when fully processed" — the LrC
   trauma was *dishonest* placeholders, not placeholders per se.

5. **Idempotency is the recovery mechanism.** There is no retry queue. A failed
   or canceled run leaves committed work intact and re-drives cheaply: the next
   run skips what's unchanged and re-does the rest. "Import again" *is* the repair.

## The pipeline

```
walk ─► SCAN ─► HASH ─► MATCH ─► EXTRACT ─► WRITE ─► post-commit
       1 grtn   pool     1        pool      1 grtn
                (4)    (matrix)   (2)      50/txn
```

Five stages connected by bounded channels; a full channel blocking a send *is* the
backpressure. Every channel is created, wired, and closed in exactly one function
(`pipeline.run`); stages take directional channel params and never make channels.
One `errgroup` owns all goroutines.

| Stage | File | Job | Notes |
|-------|------|-----|-------|
| SCAN | `stage_scan.go` | Walk the tree, emit a candidate per file | The bouncer: drops hidden files, ignore-list hits (`Importer.Settings.MatchIgnore` — the D18 list and matching are owned by `internal/settings`), unknown extensions, empty files, and unchanged files (the skip gate). Routes sidecars. Records visited paths for the missing-diff. |
| HASH | `stage_hash.go` | Fingerprint (xxhash of first 64KB + size) | Also runs the magic-byte `Sniff` on the same buffer — see the mismatch policy below. |
| MATCH | `stage_match.go` | Run the identity matrix, mint IDs | **Singleton** — one goroutine reads a serializable view of the catalog it's mutating. |
| EXTRACT | `stage_extract.go` | Pull normalized metadata | Best-effort: a decode failure is a DLQ row, never a stop. |
| WRITE | `stage_write.go` | Commit a batch in one transaction | **Singleton** — SQLite has one writer, so this goroutine *is* the batching point (50 items, a 500ms lull, or stream end). Builds the asset/patch/sidecar rows; a reimport's transaction also runs the D28 staleness clear (derived columns + enrichment DLQ rows). |

`MATCH` and `WRITE` are single goroutines on purpose; `HASH`/`EXTRACT` are worker
pools (`Machine.Workers.Ingest`, falling back to `defaultPools` in `pipeline.go`).
There is no decode stage: ingest never touches pixels, which is why its
throughput is hash/extract-bound (the D30 traces showed thumbnailing at 95% of
the old run time — that work now happens post-commit, under the enrichment
engine's CPU budget). WRITE's post-commit hook (`OnAssetCommitted`) is where the
composition root chains the enrichment dispatcher nudge (`Engine.RequestScan`) —
a hint, never truth; the missing-artifact scan stays the authority.

## Entry points

- **`Importer.Run` / `RunJob`** (`pipeline.go`) — the concurrent pipeline over a
  whole source. `RunJob` adds a job id for progress events (see `jobs.go`); `Run`
  is `RunJob` with no id. This is the normal path.
- **`Importer.IngestFile`** (`importer.go`) — the same stages for a single path,
  run sequentially (a batch of one). This is the seam the watcher feeds hints into
  (impl/05): present → ingest, gone → mark missing (`markGone`), the *same* per-path
  decision the walk makes. Per **D20** it never auto-relinks or merges a move — a
  gone path is simply marked missing. It reuses the stage transforms directly.
- **Reconcile is not a component** (D14) — "reconcile is a schedule, not a
  component": it's just `Run` in full-walk mode. The walk-end diff (`markMissing`)
  marks vanished files missing; a file that reappears at its original path is
  restored via reimport. The old standalone `Reconcile` retired in impl/05.3; its
  only unique piece — the whole-source-offline flip — moved to the watcher's poll
  monitor (`internal/watcher`, the one sanctioned `sources.connectivity` write).

## The identity matrix (precedence)

In `classify` (`stage_match.go`), checked top-down — the order *is* the policy.
Per **D20** the matrix never auto-changes identity: it acts on a known *path* and
otherwise detects-and-flags. There is **no relink/move verdict**.

1. **Reimport** — path matches an existing asset → refresh observations (and
   restore online if it was missing and reappeared at its **original** path). Path
   identity wins for a known address.
2. **Duplicate** — content matches another asset (present **or** missing, any
   source) → mint a NEW identity + a `pending` `duplicates` row. A detection FLAG
   only, never a mutation of the matched asset: a match against a *missing* asset is
   a probable move, against a *present* one a plain duplicate; the review queue
   derives which (DEFERRED §5).
3. **New** — no match → mint.

The in-run hash map is why a duplicate *pair* imported together (a copy that
exists before its original is committed) is still caught — the original isn't in
the DB yet, but it's in the map.

## Rules you must not break

- **Never write a judgment column from here.** The scoped interfaces make it a
  compile error on the single-file path; on the batch path, `writeItem` uses only
  the observation/derived/dup/sidecar/import repos on `Repos`. Keep it that way.
- **Channels are born, wired, and closed only in `pipeline.run`.** Stages receive
  directional channels; they never allocate or close one.
- **Never decode pixels in ingest.** Thumbnails (and every future
  pixel-derived signal) belong to the enrichment engine — reintroducing a
  decode stage re-couples commit latency to the slowest work (the D25
  reversal, with D30's measurements as the receipt).
- **The reimport staleness clear stays inside the reimport's transaction**
  (`clearStaleDerived`): derived columns and the asset's enrichment DLQ rows
  flip together with the observation patch, or not at all (D28).
- **Per-file failures never abort the run.** They become `import_errors` (DLQ)
  rows and the run continues. Only catastrophic failures (DB down, source root
  unreachable at start) return an error.
- **Mint IDs via `domain.NewID`** (UUIDv7), never inline.

## Tracing (gospan)

The pipeline is instrumented with [gospan](https://github.com/akmadian/gospan)
spans when `Importer.Tracer` is set (nil = off; every call on a nil tracer is a
~4ns no-op — the entire test suite runs untraced). The span vocabulary:

- `import.run` — the root; every other span in a run nests under it. Attrs:
  source, session, final counts. A canceled run reads status `canceled`.
- `import.scan` — the walk. Its duration includes SCAN-channel backpressure —
  honest: that is what the walk spends its time on.
- `import.asset` / `import.sidecar` — one trace root per item (distinct names so
  aggregates never mix their timings). The span rides the item (`pipelineItem.ctx`),
  starts at SCAN emit, and ends after WRITE commits — so an item still in flight
  at cancel/crash shows as an incomplete span (`end_ns IS NULL`), by design.
- `import.hash` / `import.match` / `import.extract` — per-stage child spans,
  ended **before** the downstream send, so the gap between one stage's end and
  the next stage's start *is* the queue time. No work = no span (sidecars have
  no match/extract spans). A stage failure `Fail`s its span; the DLQ row
  remains the durable record. (Thumbnail spans live in the enrichment
  vocabulary: `enrichment.thumbnail`.)
- `import.await-commit` — WRITE arrival → commit: the batching latency made
  queryable (batch fill + lull + transaction).
- `import.write-batch` — each commit is its own tiny trace (a batch serving N
  item traces belongs to none of them); items and batch share a `batch_seq` attr.

The dev harness traces by default (`--trace=false` for A/B runs) into
`<catalog>/traces/`, one plain-SQLite file per run — query it directly or drop
it on the gospan viewer. The trace file is observational exhaust, never catalog
state: deleting it loses diagnostics, nothing else. Measured cost on the
throughput fixture: ~2–3% at ~2,000 files/s (small-file worst case), zero
dropped events at the default buffer.

## Bookkeeping: sessions, the DLQ, and jobs

- Each run opens an `import_sessions` row (counts + per-extension skip tallies).
- Every per-file failure is an `import_errors` row keyed to that session — this
  *is* the dead-letter queue (D13). There is no retry timer; the next scan is the
  retry.
- `jobs.go` is the entire v1 job system: a `jobID → cancelFunc` map plus an
  `OnProgress` callback. River replaces it only if durable background jobs ever
  arrive (D17). Cancelling a job cancels the context; WRITE commits its current
  batch and stops.

## Mismatch, sidecars, the skip gate — the sharp edges

- **Extension vs. content (D7, `mismatch.go`).** A `.png` whose bytes are really a
  JPEG is reclassified to the content's handler, badged with an
  `extension_mismatch` marker in `extended_metadata`, and logged. Only the
  unambiguous raster formats are validated this way; container/RAW families share
  magic, so the extension legitimately refines them.
- **Sidecars** (`.xmp`, `.aae`, …) route SCAN→HASH→WRITE and land in
  `sidecar_files` — tracked, never treated as assets. The grouping engine attaches
  them later; v1 only records them.
- **The skip gate** (`unchanged` in `stage_scan.go`) skips a file when size and
  mtime match a known asset. It reads `ListKnownFiles`, which returns **online**
  assets only — so a *missing* file that reappears unchanged is deliberately *not*
  skipped: it must flow through the matrix to be restored.

## Tests

`pipeline_test.go` is the acceptance suite — the five matrix verdicts, the in-run
duplicate pair, the D7 mismatch, the cross-engine corrupt→heal loop, the
reimport staleness clear, batch-progress visibility, and the re-derived commit
invariant (assets commit with no thumbnails; a real enrichment engine then
converges them, after a full import and after a mid-run cancel) — plus a
throughput benchmark that now measures ingest alone.
Pure transforms (`partialHash`, the matrix) get direct unit tests; the channel
plumbing is tested once via the end-to-end runs, not re-tested per stage.

Run the engine by hand with the dev harness: `go run ./cmd/dev import <path>`
(`--catalog <dir>` for a browsable DB, `--debug` for a pprof/expvar UI).
