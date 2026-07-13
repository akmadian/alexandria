# 22 — Enrichment observability: live view, graph renderer, budget gauges

**Areas:** ops, backend. **Blocked by:** 18-backend-enrichment-engine.md.
**References:** D28 legibility commitments #2 and #4; `cmd/dev` (the harness);
DEFERRED §9 (dev-harness observability surface — this task is a partial trigger-fire, scoped
to enrichment only).

The visibility half of the no-workflow-engine decision. Anti-goal, from the pprof lesson:
never generic dumps — everything speaks asset / kind / artifact / queue vocabulary.

## Scope

- **Snapshot endpoint** (dispatcher-owned, JSON): per-kind queue depths (hot/cold), in-flight
  (asset, kind, started, tokens held), budget gauges (capacity, in-use, per-kind holds),
  per-kind duration histograms + per-(kind, asset) token cost, DLQ counts by reason. This JSON
  contract is the future in-app dev-corner feed — design it as the contract, the page below is
  just its first consumer.
- **Live dev-harness page** (`cmd/dev --debug`): polls/SSEs the snapshot; asset × kind matrix
  (done/queued/running/failed per cell), queue + budget views. Hand-rolled HTML, stdlib only
  (DEFERRED §9's constraint stands until its own trigger fires).
- **`cmd/dev jobs graph`**: renders the registry as DOT (+ ASCII fallback), per asset type —
  the hierarchy presentation over the flat registry rows.

## Out of scope

The in-app dev corner itself (frontend; consumes this endpoint later), pprof/runtime metrics
(DEFERRED §9 proper), any persistence of metrics (in-memory ring buffers suffice; histograms
reset on restart — matching the tracker's semantics).

## Acceptance

- With a synthetic backlog running, the page shows depths draining, budget in-use ≤ capacity,
  and a poisoned fixture appearing in the DLQ view with its reason code.
- `jobs graph` output for the raster type shows thumbnail → {sharpness, clipping, phash}
  edges; DOT renders under `dot -Tsvg` without warnings.
- Snapshot handler has a unit test (shape + a race-detector pass under concurrent dispatch).
