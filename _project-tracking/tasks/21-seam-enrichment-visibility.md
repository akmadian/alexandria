# 21 — Seam: enrichment visibility, pause/effort controls

**Areas:** seam, frontend-contract. **Blocked by:** 18-backend-enrichment-engine.md.
**References:** D28 (pull-decorated visibility, aggregate events), C8/C9/C13/C15,
`docs/seam-events-jobs.md`, `internal/seam/events.go` (the catalog), grid honest-states
requirement (D25: enriching / ready / failed rendered distinctly).

The seam face of the engine: how the frontend sees enrichment without per-asset event floods.

## Scope

- **Decoration**: asset responses (detail + grid rows) gain an `enriching` bitmask field fed
  by `tracker.RunningBatch` (one lock acquisition per page). Frontend derives per-artifact
  state: data present = ready, bit set = enriching, DLQ = failed, neither = pending. Stage
  constants + the field flow through `cmd/generate` (C15) — no hand-written TS.
- **Failed state**: expose per-asset enrichment errors (kind, reason, attempts) through the
  existing error/detail read path — decide shape in-task; no new event type.
- **Aggregate events only** (C9): enrichment progress rides `jobs/progress` with a queue-depth
  payload extension (throttled ticks); completion batches ride `catalog/changed`. Write
  ordering (DB → clear bit → emit) means an invalidation never yields a stale re-fetch.
- **Controls**: seam methods for pause/resume (global + per-kind), the effort dial (settings
  value), and the viewport priority hint (debounced asset-ID set; replace-wholesale semantics;
  documented as hint-never-truth).

## Out of scope

Frontend rendering of the states (frontend rebuild epic), the dev-corner live view (22's
snapshot endpoint is its future feed), heavy-signal UX.

## Acceptance

- Emit-catalog additions validated by `ValidateEventCatalog`; no new topic; forbidigo still
  green (single EventsEmit chokepoint).
- Decoration unit test: two assets mid-enrichment in a 200-row page → exactly those rows carry
  bits; bits gone after engine commit + clear (ordering asserted).
- Pause via seam stops dispatch (engine test reused through the seam); hint call reorders the
  hot lane; effort dial persists through settings and applies to the budget.
- `make generate` committed diff: stage union, decorated model, queue-depth payload in TS.
