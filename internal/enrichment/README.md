# internal/enrichment

The convergent-lane background engine (D25/D28): derives work from missing
artifacts, produces them under a global CPU budget, and commits results through
the catalog's third writer class. `thumbnail` is the first real job kind
(decode for raster formats; RAW via the exiftool daemon's embedded-preview
extraction); task 20 adds the cheap signals. The `D<n>` references point at
[`docs/decisions.md`](../../docs/decisions.md).

## The model

The engine is a **directed acyclic graph managed as a build system** — never a
dataflow pipeline. **Nodes are job definitions** (registry rows); **edges are
their prerequisite declarations**, realized at runtime as enqueues; **queues
hold jobs** — work orders carrying an asset ID and a priority, never a payload;
and **ground truth lives only in the catalog**: artifact column NULL = missing,
set = present, DLQ row = failed. Queued/running live only in memory. In one
sentence: *the engine schedules jobs over assets; the catalog accumulates
artifacts.*

The four-layer vocabulary of one job: the **asset** is the operand (the
identity the job is about); its **input** is what it physically reads — its
parents' artifacts (a signal job reads the thumbnail file, not the original
bytes), which is why prerequisites gate on artifact-present; the **artifact**
is the value it computes; the **side effect** is the durable commit (`Produce`
returns the artifact packaged as an `ApplyFunc` — the side-effect closure the
writer runs).

Every enqueue is a **claim, never a record**: each pop is rechecked against
the catalog before producing, so crash recovery is a rescan, a dropped queue
loses nothing, and a wrong or stale enqueue degrades to suboptimal *order*,
never wrong data. There is no job journal and no run identity — "the missing
artifact IS the queue" (D17, generalized).

## The pieces

| Piece | File | One-liner |
|-------|------|-----------|
| Job-definition registry | `registry.go` | ONE file is the whole graph (D28 commitment #1): every definition is a node — kind, lane, applicability over `assettype` capabilities, artifact column, prerequisite edges, pool default, timeout policy, weight, producer. `Validate`/`MustValidate` topo-sort it: cycles, danglers, definitions applicable to nothing all fail boot and the suite. `Definitions(…)` takes the producers' runtime dependencies (the thumbnailer, a source resolver) and returns the canonical rows. |
| Thumbnail producer | `thumbnail.go` | The first real node: resolves the asset's absolute path and executes whatever strategy the `assettype` row holds (`handler.Thumb` — a method value on `thumbnailer.Thumbnailer`; decode vs. RAW embedded preview is the strategy's business, invisible here). Failures map onto the DLQ taxonomy: `tool_unavailable` (exiftool undiscovered), `read_failed`, `decode_failed`. |
| Job queues | `queue.go` | One `container/heap` per node. `Less` is the composite priority key — hinted band (in hint order) over import recency — and is the ONLY place dispatch order exists, never a DB column. Hint promotion/demotion are `heap.Fix` (`promote`/`demote`). |
| Dispatcher | `dispatcher.go` | One goroutine owning every queue and the dedup ledger. Three job sources into the same queues: **scans** (the authority — on open, on demand, on drain-refill), **edge emissions** (an applied completion enqueues the node's dependents, inheriting the asset's live hint priority — the frontier advances a level per commit), **hints** (speculative, replace-wholesale). Workers rendezvous for jobs; pause parks them. |
| Workers | `worker.go` | Per-definition pools (`Workers.Enrichment.<kind>`, registry default fallback), every worker running the identical node template: pop → fetch asset → recheck eligibility → I/O token → weighted budget → produce. Only the definition's data differs. Watchdog definitions get a heartbeat-resettable stall timer instead of a wall clock. |
| Budget | `budget.go` | Weighted CPU semaphore above the pools — caps the SUM (admission control is Go's only throttle). The effort dial (paused/low/normal/full, `machine.json`) resizes it by *reservation*; jumbo weights clamp to the dialed capacity — they serialize, never deadlock. Per-SOURCE I/O tokens cap concurrent reads (per-device awareness deferred, DEFERRED §11). |
| In-flight tracker | `tracker.go` | `map[assetID]KindSet` under RWMutex — the transient queued/running truth the seam decorates from (task 21). Process restart empties it, and that is correct. |
| Writer | `writer.go` | The engine's ONE catalog mutator (one-cook): batched transactions (50 items / 500ms lull), applies results through `catalog.AssetDerivedWriter` — judgment/observation columns are unreachable by type. Ordering contract: **DB write → clear bit → emit.** Its completion reports drive edge emission. |
| DLQ | `sqlite/enrichment_repo.go` | `enrichment_errors(asset_id, kind, …)` — absence is ambiguous, so failure is durable. Exhaustion (5 attempts) is terminal for scans AND hints; success deletes the row. The scan SQL is engine-internal by decision — never routed through `internal/ast` (see the D28 dated note). |

## Rules you must not break

- **No priority column in the DB, ever.** The heaps' `Less` is the only place
  ordering exists; a confused queue degrades to suboptimal *order*, never
  incorrectness — that invariant is what lets hints be hints.
- **Artifact values never ride queues.** Queue entries are work orders; a
  decoded buffer parked in a queue is retained memory outside admission
  control. When measurements demand artifact handoff (D28's fusion trigger),
  the sanctioned shapes are a bounded lookaside cache (miss = the disk read
  that must exist anyway for rescan recovery) or decode fusion — one read,
  several artifacts, still independently applied.
- **Producers never touch the catalog.** They return an `ApplyFunc`; only the
  writer goroutine runs it, inside a batch transaction, against the derived
  writer interface.
- **Cancellation is not failure.** An engine shutdown mid-produce writes no
  DLQ row — the rescan re-derives the work. Only real producer failures
  (including `stalled`/`timeout`) earn a row.
- **Eligibility is rechecked at dispatch.** Scans go stale, hints are
  speculative, and emissions can race; the one cheap probe before produce is
  what keeps all three harmless.
- **A new capability is a new registry row** (C10) — plus its derived column
  on the `sqlite` allowlist and, when user-filterable, a vocabulary field
  (C7/C15, task 20's recipe).

## Tracing (gospan)

Instrumented from birth (D30): `enrichment.<kind>` roots (attrs: asset,
hinted, tokens, size, outcome, batch_seq) with an `enrichment.produce` child —
the gap after produce ends is await-commit time, same reading recipe as
import — plus `enrichment.scan` passes and `enrichment.write-batch` fan-in
traces. Nil tracer = off, ~4ns.

## Tests

`enrichment_test.go` is the acceptance suite, built on the **fake definition**
— the permanent test instrument: poisonable (DLQ exhaustion), gateable (pause,
tracker), weightable (budget ceiling), chainable (prerequisite gating + edge
emission), instant (hint ordering). Registry validation is a table test; the
budget's blocking/dial mechanics are unit tests (`budget_internal_test.go`).
The canonical registry is validated by the suite on every run, and the
importer's acceptance suite runs a REAL engine end-to-end (import → converge,
cancel → converge, corrupt → DLQ → heal).
