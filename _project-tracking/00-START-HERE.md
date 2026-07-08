# Alexandria — START HERE (master)

**This is the head node of the implementation task tree.** Single entry point for any session,
human or Claude: it answers *what's next, right now*, and links down. Area trackers answer *how
and why*. **Maintenance contract:** whoever completes (or reprioritizes) a frontier item updates
this file in the same change — a stale head is worse than no head.

**Last updated:** 2026-07-08.

**Layout:** `backend/` · `seam/` · `frontend/` (area trackers + specs) ·
[`functional-requirements.md`](functional-requirements.md) (the backlog, P0–P4) · `design/`
(designs written ahead of their milestone — CI/hygiene, release, telemetry, local AI, RAW export
dispatch, testing strategy, CONTRIBUTING outline) · `ops/` + `perf/` (repo-setup and performance
working references) · `_scratch/` (raw notes). Durable contributor-facing reference lives in
`docs/` instead — deliberately lean pre-release. **Graduation rule:** when an area stabilizes,
its durable artifacts move to `docs/` — named candidates: the backend decision log, the data
model, `CONSTANTS.md` (graduate with v1), and each `design/` doc once built (it then describes
what *is*).

## Cold-start reading order

1. [`CONSTANTS.md`](CONSTANTS.md) — cross-cutting invariants (C1–C14). Non-negotiable everywhere.
2. This file — pick a frontier item.
3. The owning area tracker for what you picked:
   [`backend/00-START-HERE.md`](backend/00-START-HERE.md) ·
   [`seam/00-START-HERE.md`](seam/00-START-HERE.md) ·
   [`frontend/00-START-HERE.md`](frontend/00-START-HERE.md)

## The frontier — current head, multiple valid picks

| Pick | What | Area | State |
|---|---|---|---|
| **A** | **impl/06 XMP sync — the wiring increment**: DB application across the three writers in one tx, outbound sidecar write (merge + atomic rename), ingest/watcher triggers + per-asset outbound debounce, `xmpWriteBack`/`xmpConflictResolution` settings consumers, watcher-side echo check | Backend | In progress — read path, conflict grid, judgment apply, keyword union all DONE |
| **B** | **Query-layer round**: the AST→SQL compile authority (the one query builder `QueryAssets`, smart collections, and Review projections reuse). Grammar and token contract already designed. Scope folded in by the 2026-07-08 audit: collections CRUD (`CollectionRepository` has no implementation and no other owner), the prior-state bulk read undo needs, and the FTS⋈tags slice | Backend→Seam | Unblocked now — spec in [`seam/01-queries-and-commands.md`](seam/01-queries-and-commands.md); residual decisions + folded scope in [`backend/04-open-questions.md`](backend/04-open-questions.md) #4 |
| **C** | **CI wiring** per [`design/ci.md`](design/ci.md) + [`design/repo-hygiene-backend.md`](design/repo-hygiene-backend.md) (+ the `format`/`format:check` script gap in [`design/repo-hygiene-frontend.md`](design/repo-hygiene-frontend.md)) | Ops | Unblocked, parallel to anything |

A and B are independent; do in either order or interleave. C is background-sized.

## The tree below the frontier (dependency order)

```
impl/06 XMP wiring ──┐
                     ├─→ seam round ──→ frontend implementation begins
query-layer round ───┘   (reconcile contract.ts per the ledger in seam/01;
                          Wails v2 bindings + generated TS models)

frontend implementation → view modes → palette/keyboard → task views → Review v1
signals milestone (ENRICH stage + enrichment jobs, backend/06) → cull force multipliers
grouping deep-dive (open question #7) → burst/stack collapse

seam round → impl/12 app host (Wails wiring, startup sequence, watcher supervision,
                               live pool resize — backend/impl/12-app-host.md)
```

Unscheduled design tasks (2026-07-08 audit — each needs its own design session, pick up
deliberately): **mid-scan volume disconnect / walk-completeness** (open question #15 — do before
the frontend renders missing badges at scale) and **catalog backup system** (open question #16 —
urgent at first release; the backup-before-migration floor is owned by impl/12).

Deliberately parked (with triggers, don't pick up early): Review automation rules (after Review
v1 usage), NL→AST local-LLM tier (after deterministic parser), impl/09 LrC migration build
(design-only), River jobs (when durable background work is real), Windows pass (budgeted late
per milestone).

## Status at a glance

| Area | Status | Tracker |
|---|---|---|
| Backend | impl/01–05 + 11 done; impl/06 in progress; impl/07 exiftool slice done; impl/10 consumer slice done | [`backend/00-START-HERE.md`](backend/00-START-HERE.md) |
| Seam | Design pre-shaped; awaits query-layer round | [`seam/00-START-HERE.md`](seam/00-START-HERE.md) |
| Frontend | Design complete (2026-07-07, Wails v2 locked); implementation awaits seam | [`frontend/00-START-HERE.md`](frontend/00-START-HERE.md) |
| Ops / Testing | Specs waiting in [`design/`](design/) (CI, release, telemetry, testing strategy); no milestone tracking yet | — |
