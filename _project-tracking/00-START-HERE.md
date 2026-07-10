# Alexandria — START HERE (master)

**This is the head node of the implementation task tree.** Single entry point for any session,
human or Claude: it answers *what's next, right now*, and links down. Area trackers answer *how
and why*. **Maintenance contract:** whoever completes (or reprioritizes) a frontier item updates
this file in the same change — a stale head is worse than no head.

**Last updated:** 2026-07-10.

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
the impl/12 app-host seed. [`seam/impl/15-method-surface.md`](seam/impl/15-method-surface.md) is now
**Phase 1 shipped (2026-07-09)** — the backed Go surface (`AssetService`/`CollectionService`/
`SettingsService`/`SourceService`), the `ApiError` normalization layer + generated `errors.ts` code
catalog, per-method tests, all webkit-free. ~40% of the contract surface was **deferred, not stubbed**
(no backing engine yet) with per-row triggers, and the contract.ts/`models` TS reconciliation is
deferred to the `wails dev` pass — see [`backend/impl/DEFERRED.md`](backend/impl/DEFERRED.md) §7.
[`seam/impl/16-events-and-jobs.md`](seam/impl/16-events-and-jobs.md) is now **✅ DONE (2026-07-10)**
— the C8 event catalog + single `Emit` choke point (`internal/seam/events.go`/`events_wails.go`,
forbidigo-enforced), the C9 job envelope + a real `ImportService` (first producer: `startImport`/
`cancelJob` over `importer.Jobs`), `catalog/changed` emits wired into impl/15's asset/collection
write methods, and the generator extended to emit `events.ts`. Payload TS *interfaces* deferred to
the wails-dev pass with a hard trigger (DEFERRED §7); the frontend event-pump consumer belongs to
the rebuild (frontend/09 §Event pump). **The seam round is COMPLETE.**

**Frontier pick now:** the **frontend ground-up rebuild** is **UNDERWAY** (started 2026-07-10) —
building in isolation against a contract-faithful mock, thin-vertical-then-widen. The foundation
vertical has landed (query-model + `AlexandriaAPI` contract + AST mock engine + catalog store +
virtualized DS grid; old pre-rework `src/` deleted), and the **first widen slice — the interactive
enum filter bar — has landed** (RAC-based `components/` primitives: button/popover/menu; a
`filter-bar` feature composing them with the query-model; `query-model/assemble`; store filter
actions + the working-set echo that retires the cursor-seed debt). See `frontend/00-START-HERE.md
§Where the project is right now`. Widen slices continue. Independent alternatives: the **impl/12
app-host** round (watcher supervision, startup sequence, live pool resize) and A (impl/06 remainder,
small).

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
| Seam | **COMPLETE** — impl/14 DONE; impl/15 Phase 1 DONE (backed Go surface + ApiError + `errors.ts`); **impl/16 DONE** (2026-07-10 — event catalog + `Emit` choke point + `ImportService` + `catalog/changed` emits + `events.ts`). Deferred (documented, DEFERRED §7): unbacked impl/15 methods, event payload TS types, contract.ts reconciliation — all to the wails-dev pass / their engines | [`seam/00-START-HERE.md`](seam/00-START-HERE.md) |
| Frontend | Design + architecture locked (`frontend/09`); **rebuild IMPLEMENTATION STARTED 2026-07-10** — foundation vertical landed (query-model + `AlexandriaAPI` contract + AST mock engine + catalog store + virtualized DS grid), then the **filter-bar widen slice** (RAC `components/` primitives incl. field; a generic pill + per-kind value-editor registry — enum/numeric/text; `query-model/assemble` + store filter actions/working-set echo); pre-rework `src/` deleted; remaining filter kinds (date, tag/source) + group editor next | [`frontend/00-START-HERE.md`](frontend/00-START-HERE.md) |
| Ops / Testing | CI + backend hygiene BUILT (2026-07-09, `Makefile` + `ci.yml`); release, telemetry, testing-strategy specs still waiting in [`design/`](design/) | — |
