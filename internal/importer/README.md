# internal/importer

Turns files on disk into catalog rows. This is the ingest engine: it walks a
source, figures out what each file *is* relative to what the catalog already
knows, extracts metadata, generates thumbnails, and writes everything the
catalog needs — durably, concurrently, and idempotently.

If you read one thing first, read
[`impl/done/04-ingest-pipeline.md`](../../docs/project-tracking/backend/impl/done/04-ingest-pipeline.md)
— this package is its implementation. The `D<n>` references below point at
[`02-decision-log.md`](../../docs/project-tracking/backend/02-decision-log.md).

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

4. **An asset commits only when fully processed.** Thumbnailing happens *before*
   the database write, not after. There is no "imported but still generating"
   half-state — no placeholder cards (an emphatic product decision, D12). If a
   committed asset has no thumbnail, there is a DLQ row saying why.

5. **Idempotency is the recovery mechanism.** There is no retry queue. A failed
   or canceled run leaves committed work intact and re-drives cheaply: the next
   run skips what's unchanged and re-does the rest. "Import again" *is* the repair.

## The pipeline

```
walk ─► SCAN ─► HASH ─► MATCH ─► EXTRACT ─► THUMB ─► WRITE ─► post-commit
       1 grtn   pool     1        pool       pool     1 grtn
                (4)    (matrix)   (2)        (2)     50/txn
```

Six stages connected by bounded channels; a full channel blocking a send *is* the
backpressure. Every channel is created, wired, and closed in exactly one function
(`pipeline.run`); stages take directional channel params and never make channels.
One `errgroup` owns all goroutines.

| Stage | File | Job | Notes |
|-------|------|-----|-------|
| SCAN | `stage_scan.go` | Walk the tree, emit a candidate per file | The bouncer: drops hidden files, ignore-list hits (`Importer.Settings.MatchIgnore` — the D18 list and matching are owned by `internal/settings`), unknown extensions, empty files, and unchanged files (the skip gate). Routes sidecars. Records visited paths for the missing-diff. |
| HASH | `stage_hash.go` | Fingerprint (xxhash of first 64KB + size) | Also runs the magic-byte `Sniff` on the same buffer — see the mismatch policy below. |
| MATCH | `stage_match.go` | Run the identity matrix, mint IDs | **Singleton** — one goroutine reads a serializable view of the catalog it's mutating. |
| EXTRACT | `stage_extract.go` | Pull normalized metadata | Best-effort: a decode failure is a DLQ row, never a stop. |
| THUMB | `stage_thumb.go` | Generate the thumbnail file | Precedes WRITE (idea #4). Skipped for types with no generator. |
| WRITE | `stage_write.go` | Commit a batch in one transaction | **Singleton** — SQLite has one writer, so this goroutine *is* the batching point (50 items, a 500ms lull, or stream end). Builds the asset/patch/sidecar rows. |

`MATCH` and `WRITE` are single goroutines on purpose; `HASH`/`EXTRACT`/`THUMB` are
worker pools (defaults in `defaultPools`, `pipeline.go`). Pool sizes are hardcoded
until per-machine config lands.

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
- **THUMB stays before WRITE.** Don't "optimize" by committing first and
  thumbnailing after — that reintroduces the half-imported state.
- **Per-file failures never abort the run.** They become `import_errors` (DLQ)
  rows and the run continues. Only catastrophic failures (DB down, source root
  unreachable at start) return an error.
- **Mint IDs via `domain.NewID`** (UUIDv7), never inline.

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
duplicate pair, the D7 mismatch, corrupt→heal DLQ, batch-progress visibility, and
the sacred full-processing-invariant cancel test — plus a throughput benchmark.
Pure transforms (`partialHash`, the matrix) get direct unit tests; the channel
plumbing is tested once via the end-to-end runs, not re-tested per stage.

Run the engine by hand with the dev harness: `go run ./cmd/dev import <path>`
(`--catalog <dir>` for a browsable DB, `--debug` for a pprof/expvar UI).
