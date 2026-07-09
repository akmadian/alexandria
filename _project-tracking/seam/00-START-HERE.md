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

Cross-cutting rules live in `../CONSTANTS.md` (C6–C9, C13 are the seam's).

## Sequencing

Two pending design/implementation rounds, in order (both pre-shaped by the 2026-07-07 frontend
round; see backend open questions #4/#5):

1. **Query-layer round (backend):** the AST grammar in Go + the SQL compiler — the single query
   authority that `QueryAssets`, smart collections, and Review projections all reuse. The AST
   *shape* is designed (`01-queries-and-commands.md`); this round makes it compile.
2. **Seam round:** reconcile `contract.ts` per the ledger, wire Wails bindings + TS generation,
   land the event/job envelopes.

Frontend implementation unblocks after these — and now means the **ground-up rebuild** per
`../frontend/09-ground-up-redesign-notes.md` (architecture locked 2026-07-08); its seam-method
requirements are listed in `01-queries-and-commands.md` §Additions.
