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
2. **Seam round:** SPECCED (2026-07-09) into three build docs under `impl/` — **14**
   (bindings & generation harness; also creates the Wails composition root that
   `../backend/impl/12-app-host.md` later grows) first, then **15** (method surface) ∥ **16**
   (events & jobs). Structure locked: root `main.go`/`app.go`/`wails.json`/`build/`, bound
   services in `internal/seam`, generated TS committed with a CI freshness gate.

Frontend implementation unblocks after these — and now means the **ground-up rebuild** per
`../frontend/09-ground-up-redesign-notes.md` (architecture locked 2026-07-08); its seam-method
requirements are listed in `01-queries-and-commands.md` §Additions.
