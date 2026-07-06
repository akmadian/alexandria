# impl/04 — Ingest Pipeline (The Milestone)

**Scope:** rework `internal/importer` into the concurrent pipeline. New: minimal job envelope.
**Blocked by:** impl/01, impl/02, impl/03. **References:** D12, D13, `03-data-model.md` §6.

The existing sequential importer has the right stage factoring and matrix core — this is a
restructure around it, not a rewrite. Preserve its semantics where this spec is silent.

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

**HASH**: read first 64KB, xxhash+size fingerprint, run `Sniff` on the same buffer (impl/03
policy). Short files fine (EOF tolerated). Sidecars hash here too, then bypass to WRITE.

**MATCH** (the matrix, precedence per `03-data-model.md` §6):
unchanged already filtered by SCAN. Order: (1) content+name vs MISSING asset → relink ·
(2) path hit → reimport · (3) content vs PRESENT asset → duplicate · (4) new → mint UUIDv7.
Consults the catalog AND the **in-run map** (hash→assetID minted this run) — without it,
first-import duplicate pairs are invisible. Emits (action, asset, existing).

**EXTRACT**: registry dispatch; nil capability → skip; failure → best-effort (empty metadata +
error record, asset still proceeds — D13 self-heal doctrine). Moves skip extraction.

**THUMB**: registry dispatch; sizes [512] for v1 (`ponytail:` the 1024/2048 preview tiers arrive
with the loupe). Failure → asset proceeds without thumbnail (thumbnail_at NULL, error recorded).
Moves skip. **THUMB precedes WRITE by design**: an asset is committed only fully processed —
no placeholder cards, ever (explicit user decision).

**WRITE**: accumulate 50 items (or 500ms lull, or stream end) → one `InTx`:
per action — mint / ApplyFilePatch / UpdatePath+SetFileStatus / mint+duplicate row; sidecar
upserts; import_errors rows; session count update. FTS triggers fire in-txn.
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
