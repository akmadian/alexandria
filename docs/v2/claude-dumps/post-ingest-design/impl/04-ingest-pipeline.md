# impl/04 — Ingest Pipeline (The Milestone)

> **Status: ✅ DONE (2026-07-06).** Built in `internal/importer/{pipeline,jobs,mismatch,ignore}.go`
> + `internal/sqlite/{sidecar_repo,import_repo}.go` + `cmd/dev`. All acceptance criteria below have
> tests in `internal/importer/pipeline_test.go` (matrix verdicts incl. relink-outranks-reimport and
> the in-run duplicate pair, sidecar tracking, D7 mismatch, corrupt→heal DLQ, batch visibility, the
> full-processing cancel invariant) plus a throughput benchmark. Deferred within scope, with
> `ponytail:` markers at each site: the `--debug` observability server (impl/08), pool-size flags
> (machine.json era), per-item re-commit on a poisoned batch tx, and the D7 hard-reject branch
> (unreachable with the current Sniff table — see mismatch.go).

**Scope:** rework `internal/importer` into the concurrent pipeline. New: minimal job envelope.
**Blocked by:** impl/01, impl/02, impl/03. **References:** D12, D13, `03-data-model.md` §6.

The existing sequential importer has the right stage factoring and matrix core — this is a
restructure around it, not a rewrite. Preserve its semantics where this spec is silent.

> **Note (2026-07-06):** impl/01–03 are DONE. This spec predates them in places; the reconciled
> view is the two sections immediately below. Where they and the older prose disagree, they win.

## What impl/01–03 already built (consume, don't rebuild)

The seam this pipeline plugs into now exists and is tested:

- **Writer-scoped catalog interfaces** (`catalog`): the pipeline holds `AssetReader`,
  `AssetObservationWriter` (`Create` / `ApplyFilePatch` / `UpdatePath` / `SetFileStatus` /
  `MarkConnectivityBySource`), `AssetDerivedWriter` (`SetThumbnailAt`), and `DuplicateRepository`
  (`Log`) — and by construction CANNOT touch judgment/sync columns. `importer.Importer` already
  carries exactly these fields (`Reader/Obs/Derived/Dups/Thumbnail/Log`); the reimport path already
  uses `ApplyFilePatch` (clobber-safe). Restructure around this — do not revert it.
- **Transaction seam** (`sqlite`): `Store.InTx(ctx, func(Repos) error)` is the WRITE stage's batch
  boundary; `Repos` bundles tx-bound Assets/Sources/Dups. (Deferred BEGIN today; `_txlock=immediate`
  is the upgrade if write contention shows.)
- **Type dispatch** (`assettype`, renamed from `filetype`): `Classify(ext) (Handler, bool)`,
  `Handler{Ext,MIME,Type,Metadata,Thumb}`, `IsSidecar(ext)`, `Sniff(head) (ContentFamily, bool)`.
  `scannedFile` already carries `handler assettype.Handler`; EXTRACT/THUMB dispatch off it.
- **Derived rebuild:** `sqlite.RebuildFTS`. **IDs:** `domain.NewID()` (UUIDv7). **Catalog open:**
  `sqlite.Open(dir) (*Catalog, error)` (WAL, synchronous(FULL), instance lock).

## New repositories to build in this milestone (deferred from impl/02 — no consumer existed until now)

- **`sidecar_files` repo** — `UpsertObservation`, `DeleteByPath`, `ListByKey(source, dir, stem)`.
  SCAN routes `assettype.IsSidecar` files here (they HASH, then WRITE upserts the observation row).
  Feeds grouping later; v1 only tracks them.
- **`import_sessions` / `import_errors` repo** — `Start`, `UpdateCounts`, `Finish`, `LogError`
  (attempts-upsert on same session+path+stage). This IS the DLQ (D13); the session row also holds
  the per-extension `skipped_unknown_json` / `skipped_ignored_json` tallies.
- **`SetAssetTags` FTS-tags maintenance** — the pipeline writes no tags, so this stays deferred to
  the tags feature. Noted, not forced.

## Topology

```
walk ─► SCAN ─► HASH ─► MATCH ─► EXTRACT ─► THUMB ─► WRITE ─► post-commit hooks
       1 grtn   pool     1        pool       pool     1 grtn
       streams  (dflt 4) (matrix) (dflt 2)   (dflt 2) batches of 50/txn
```

- Bounded channels (size ~2× consumer pool) between stages; blocking sends ARE the backpressure.
- **All channels created, wired, and closed in ONE function** (`pipeline.go: run()`). Stages are
  plain funcs `func(ctx, in <-chan T, out chan<- U) error` — directional types; stages never make
  channels. Writer-side closes propagate shutdown. One `errgroup` owns all goroutines.
- Pool sizes: hardcoded defaults for now (`hash=4, extract=2, thumb=2`) in one struct — machine.json
  arrives later.
- **MATCH is a singleton**: the identity matrix reads catalog state the pipeline is mutating;
  one goroutine = serializable reads for free. Pure indexed lookups; never the bottleneck.
- **WRITE is a singleton**: SQLite is single-writer; the goroutine IS the batching point.

## Stage responsibilities

**SCAN** (bouncer at the front door, D18/D13):
ignore-list match (baked defaults const; per-extension tally) → hidden-file rule → registry
classify: asset / sidecar / unknown (per-extension tally, no row) → nonzero size, readable →
skip gate (known-map: size exact + mtime within 2s) → emit. Also: count total as it walks
(progress upgrades indeterminate→determinate when walk completes). Records every *visited* path
for the walk-end missing diff.

**HASH**: read first 64KB, xxhash+size fingerprint, run `assettype.Sniff` on the same buffer.
Short files fine (EOF tolerated). Sidecars hash here too, then bypass to WRITE.

**Sniff mismatch policy (D7 — impl/03 built `Sniff` and deferred this wiring here):** compare the
extension's `handler.Type` against `Sniff(head)`:
- agree, or `Sniff` returns `ok=false` for a supported extension → proceed on the extension.
- disagree (ext says X, content says Y) → **trust content**: reclassify via a
  `ContentFamily → domain.FileType` map (build it here — the closed set is ~15 entries; keep Y's
  handler/MIME where one exists), write an `extension_mismatch` marker into `extended_metadata`,
  and log an informational `import_errors` row (reason `ext_mismatch`). The asset still indexes.
- supported extension but `Sniff` says the bytes are not the claimed container AND nothing usable
  (zero/garbage) → bouncer reject: `import_errors` row (`no_usable_content`), NO identity minted.

**MATCH** (the matrix, precedence per `03-data-model.md` §6):
unchanged already filtered by SCAN. Order: (1) content+name vs MISSING asset → relink ·
(2) path hit → reimport · (3) content vs PRESENT asset → duplicate · (4) new → mint UUIDv7.
Consults the catalog AND the **in-run map** (hash→assetID minted this run) — without it,
first-import duplicate pairs are invisible. Emits (action, asset, existing).

**EXTRACT**: dispatch off `sf.handler.Metadata` (nil → skip); failure → best-effort (empty metadata
+ error record, asset still proceeds — D13 self-heal doctrine). Moves skip extraction.

**THUMB**: dispatch off `sf.handler.Thumb` via the injected `thumbnailer.Registry.Generate(gen, r,
assetID)` (sizes come from the Registry — v1 default `[512]`, set in impl/03; a nil gen is a no-op).
Failure → asset proceeds without thumbnail (thumbnail_at NULL, error recorded). Moves skip.
**THUMB precedes WRITE by design**: an asset is committed only fully processed — no placeholder
cards, ever (explicit user decision).

**WRITE**: accumulate 50 items (or 500ms lull, or stream end) → one `Store.InTx`. Per action, via
the tx-bound `Repos`: mint (`Obs.Create`) / `Obs.ApplyFilePatch` / `Obs.UpdatePath`+`SetFileStatus`
/ mint + `Dups.Log`; `Derived.SetThumbnailAt` for the thumbnail marker; sidecar `UpsertObservation`;
`import_errors` rows; session count update. FTS triggers fire in-txn.
**Post-commit hooks, in order:** flush dirty (dir, stem) keys → grouping recompute *stub*
(no-op func for now — the seam exists, the engine is a later milestone) → emit JobProgress →
emit catalog-changed (callback field, wired to Wails later) → nothing else.

**Walk-end**: known-map minus visited = vanished → mark missing in one batch (kind='import'
sessions do this only for full walks). This is reconcile fused into import.

## Cancellation & errors

ctx cancels all stages; WRITE commits its current batch then exits — never rolls back completed
work. Committed work is skip-gated next run; in-flight work is lost and re-runs cheap. Per-file
errors NEVER abort the run: they become import_errors rows (path, stage, reason_code, message,
attempts). Only catastrophic failures (DB unavailable, source root unreachable at start) abort.

## Job envelope (minimal, D17)

```go
type Jobs struct{ mu sync.Mutex; m map[string]context.CancelFunc }
func (j *Jobs) Start(kind string, fn func(ctx context.Context)) (jobID string)
func (j *Jobs) Cancel(jobID string) 
```

Importer gains `OnProgress func(Progress)` (nil-safe). `Progress{JobID, Kind, Done, Total int,
TotalKnown bool, Stage string}`. Emitted per batch commit + per walk completion. This is the
entire v1 job system; River replaces it only when durable jobs arrive (D17).

## Explicitly out of scope for this milestone

Dependency fleet (pure-Go formats only) · grouping engine (stub the recompute hook; sidecar_files
rows are written so backfill Just Works later) · watcher (but `IngestFile`-style single-path entry
stays, it becomes the hint consumer) · XMP parsing (sidecars are tracked, not read) ·
machine.json · Wails wiring (callbacks stay callbacks).

## Acceptance

- **Idempotency**: import a fixture tree twice; second run = 100% skipped, zero writes, near-instant.
- **Batch visibility**: OnProgress fires ~N/50 times; each firing's Done matches committed rows.
- **Full-processing invariant**: kill the run mid-way (cancel); every committed asset either has a
  thumbnail file on disk or an import_errors row explaining why. NO committed asset is
  half-processed. This is the LrC-trauma test — treat it as sacred.
- **Matrix**: table-driven tests for all five verdicts + the relink-outranks-reimport case
  (delete-and-copy fixture) + in-run duplicate pair (copy exists before first import).
- **Corrupt file**: truncated JPEG fixture → asset exists, no dims, no thumb, error row with
  reason `decode_failed`; then "fix" the file (touch mtime, full bytes) → re-run → healed, error
  cleared (or superseded).
- **Throughput sanity**: ≥500 JPEGs/min on the dev machine with default pools (NFR-2), measured by
  a benchmark over a generated fixture set.
- **Backpressure**: with thumb pool=1 and a big fixture set, memory stays flat (channels bounded).
