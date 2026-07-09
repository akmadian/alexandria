# Alexandria — START HERE (master)

**This is the head node of the implementation task tree.** Single entry point for any session,
human or Claude: it answers *what's next, right now*, and links down. Area trackers answer *how
and why*. **Maintenance contract:** whoever completes (or reprioritizes) a frontier item updates
this file in the same change — a stale head is worse than no head.

**Last updated:** 2026-07-09.

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
| **A** | **impl/06 XMP sync**: bidirectional sidecar sync — inbound read, conflict grid, judgment apply, keyword union, outbound merge-write, settings consumers, ingest/watcher triggers, per-asset debounce | Backend | Core DONE (2026-07-08). **Remaining:** caption/title inbound (blocked on sparse observation writer), `alexandria:Flag` custom namespace (OQ #8) |
| ~~B~~ | ~~Query-layer round~~ | Backend | **✅ DONE (2026-07-08)** — `internal/ast` + full surface + collections + FTS⋈tags. Old `AssetFilter`/`List` deleted. **Seam round is now unblocked.** |
| ~~C~~ | ~~CI wiring~~ | Ops | **✅ DONE (2026-07-09)** — root `Makefile` (`make check-backend`) + `.github/workflows/ci.yml` (native path filter) + `.golangci.yml` (invariants mechanized via depguard/forbidigo) + govulncheck + 70% coverage gate. Frontend + app CI jobs added by impl/14 (`ci-frontend.yml`, `ci-app.yml`); `format:check` gap still deferred until frontend rebuild. |

With B and C done, the **seam round** is the frontier.
[`seam/impl/14-bindings-and-generation.md`](seam/impl/14-bindings-and-generation.md) is now
**✅ DONE (2026-07-09)** — Wails composition root at the repo root, `internal/seam` walking
skeleton (`ListSources`) bound end to end, and the TS generation harness: Wails reflects struct
models; a hand-rolled generator (`internal/seam/generate`) emits the `TokenField`/`TokenOperator`/
`ValueKind` unions from `internal/ast` and the domain-enum unions *discovered by type-checking
`internal/domain`* (no EnumBind, no hand-maintained lists — see the impl/14 status block for the
two deviations). Enforced by a freshness gate on the backend path + three path-filtered CI jobs
(backend / frontend / app), which also now prove the toolchain isolation. The composition root is
the impl/12 app-host seed. **Frontier picks now:** [`seam/impl/15-method-surface.md`](seam/impl/15-method-surface.md)
∥ [`seam/impl/16-events-and-jobs.md`](seam/impl/16-events-and-jobs.md) (parallel). A (impl/06
remainder) is independent and small.

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
| Backend | impl/01–06 + 11 + 13 done (06 core — caption/title + flag pending); impl/07 exiftool slice done; impl/10 consumer slice done | [`backend/00-START-HERE.md`](backend/00-START-HERE.md) |
| Seam | **Build-ready**: impl/14/15/16 specs written 2026-07-09; structure locked (root Wails scaffolding, `internal/seam`, committed generated TS + CI freshness) | [`seam/00-START-HERE.md`](seam/00-START-HERE.md) |
| Frontend | Design complete (2026-07-07, Wails v2 locked); **architecture locked by the ground-up redesign round (2026-07-08, `frontend/09`)** — `frontend/src/` is disposable, rebuild fresh; implementation awaits seam | [`frontend/00-START-HERE.md`](frontend/00-START-HERE.md) |
| Ops / Testing | CI + backend hygiene BUILT (2026-07-09, `Makefile` + `ci.yml`); release, telemetry, testing-strategy specs still waiting in [`design/`](design/) | — |
