# Seam — START HERE

*Task-tree head: [`../00-START-HERE.md`](../00-START-HERE.md) — check it first for what's next.*

**Date:** 2026-07-07 (carved out of the frontend design round during the docs reconciliation
pass — the seam is its own concern: the contract between the Go engine and the React UI, owned
jointly, belonging to neither.)

## What the seam is

- **Transport:** Wails v2 (LOCKED, Ari 2026-07-07 — resolves backend open question #6). Go struct
  methods bound directly; TS bindings generated. The engine stays runtime-agnostic by
  construction (D1), so this remains a packaging decision, not an architecture one.
- **Types:** Go domain models are the single source of truth; TS is generated (C13).
  `frontend/src/models/` retires when bindings land.
- **The living artifact:** `frontend/src/api/contract.ts` — deliberately network-shaped, mock
  behind it. It remains design-authoritative for method shapes *until* the seam round reconciles
  it against the engine; the reconciliation ledger in `01-queries-and-commands.md` §Reconciliation
  lists every known delta.

## The doc set

| Doc | Contents |
|---|---|
| `01-queries-and-commands.md` | The query AST (C6), token registry, workhorse methods (C7), structural methods, and the contract.ts reconciliation ledger. |
| `02-events-jobs-and-binary.md` | Event envelope + topic catalog (C8), the Job envelope (C9), the binary URL channel, the error shape. |
| `impl/14-bindings-and-generation.md` | BUILD SPEC: Wails composition root, `internal/seam` skeleton, TS generation (Wails + hand-rolled vocabulary generator), Makefile/CI modularity. First. |
| `impl/15-method-surface.md` | BUILD SPEC: bound services, ApiError mapping, ledger rows #1–6/#8–10, §Additions exposure. After 14, parallel with 16. |
| `impl/16-events-and-jobs.md` | BUILD SPEC: event envelope + catalogs, job envelope over `importer.Progress`, engine→seam bridge, ledger row #7. After 14, parallel with 15. |

Cross-cutting rules live in `../CONSTANTS.md` (C6–C9, C13 are the seam's).

## Sequencing

1. **Query-layer round (backend):** ✅ DONE (impl/13, 2026-07-08) — `internal/ast` is the
   single query authority; the catalog surface consumes it.
2. **Seam round:** three build docs under `impl/` — **14** (bindings & generation harness) then
   **15** (method surface) ∥ **16** (events & jobs).
   - **impl/14: ✅ DONE (2026-07-09).** Wails composition root at the repo root
     (`main.go`/`app.go`/`wails.json`/`build/`); `internal/seam` walking skeleton (`ListSources`)
     bound end to end; TS generation live — Wails reflects struct models, and a hand-rolled
     generator (`internal/seam/generate`) emits the grammar unions from `internal/ast` + the
     domain-enum unions discovered by type-checking `internal/domain` (no EnumBind). Committed
     generated TS is freshness-gated on the backend path; three path-filtered CI jobs
     (backend/frontend/app) enforce it and the toolchain isolation. The composition root is the
     `../backend/impl/12-app-host.md` seed.
   - **impl/15: Phase 1 shipped (2026-07-09).** The backed synchronous surface — per-entity bound
     services (`Asset`/`Collection`/`Settings`/`Source`), the `ApiError` normalization layer, and a
     generated `errors.ts` code catalog — landed webkit-free. ~40% of the contract surface was
     **deferred, not stubbed** (no backing engine), and the contract.ts/`models` TS reconciliation is
     deferred to the `wails dev` pass (both with triggers in `../backend/impl/DEFERRED.md` §7).
   - **impl/16: ✅ DONE (2026-07-10).** The C8 event catalog + single `Emit` choke point
     (`events.go` pure; `events_wails.go` the sole `runtime.EventsEmit` caller, forbidigo-enforced),
     the C9 job envelope over `importer.Progress`, a real `ImportService` (first producer —
     `startImport`/`cancelJob` under `importer.Jobs`, wired in the app host), `catalog/changed`
     emits in impl/15's asset/collection write methods, and `events.ts` from the generator.
     Deviation: `Emit` derives topic from the catalog (stricter than `Emit(topic, type, payload)`).
     Deferred with triggers: event payload TS *interfaces* (DEFERRED §7), the frontend event-pump
     consumer (frontend/09 §Event pump).

**The seam round is COMPLETE.** Frontend implementation is unblocked — the **ground-up rebuild** per
`../frontend/09-ground-up-redesign-notes.md` (architecture locked 2026-07-08); its seam-method
requirements are listed in `01-queries-and-commands.md` §Additions.
