# Alexandria Frontend Design Handoff — START HERE

**Date:** 2026-07-07
**Produced by:** a ground-up frontend/UX design session (Ari + Claude Fable) that worked forward
from `docs/functional-requirements.md` and the conventions of LrC, Eagle, Photo Mechanic, and
Capture One. All decisions were made deliberately, with tradeoffs discussed.
**Audience:** a Claude instance (or Ari) doing further design refinement and/or implementation.

This set **supersedes** the deleted `frontend-architecture.md` / `frontend-ui-architecture.md` /
`frontend-implementation-guide.md`. (`frontend/CLAUDE.md` still references those — fix it as part
of the pending docs cull.) `frontend/src/api/contract.ts` remains design-authoritative for the
seam *until* `04-query-system.md`'s workhorse methods are reconciled into it.

## Headline decisions

- **UI runtime: Wails v2 — LOCKED** (Ari, 2026-07-07). Go struct methods bound directly;
  TS models generated from Go. Unblocks the project-tracking index's "blocked on runtime" row.
- **Boring skeleton, signature concepts.** The shell is the conventional DAM layout
  (FilterBar / Browser / Grid / Inspector / StatusBar). Novelty budget is spent on: the query
  model, cull speed, the Review surface, and transparency-as-chrome — not on layout invention.
- **The query AST is the spine of the whole frontend** — and it is a *joint* frontend/backend
  design object. See `04-query-system.md`.

## The doc set

| Doc | Contents |
|---|---|
| `01-constants.md` | **Read first.** The load-bearing invariants: vocabulary, state equation, boundary rules, registry rules. Contradicting this file means deliberately reopening a decision. |
| `02-flows-and-views.md` | The four user flows; the task-shaped test; view inventory (catalog, task views, home); shell layout; transparency chrome. |
| `03-state-model.md` | The glossary in full: scope, filter, query, working set, selection, cursor, arrangement, view mode. The state equation and command-targeting rule. |
| `04-query-system.md` | The query AST, token registry, filter bar / pills, save-as-smart-collection, search tiers (deterministic → optional local LLM → FTS), seam workhorse methods. |
| `05-keyboard-and-actions.md` | Action registry, context dispatch, verb grammar, per-type media verbs, command palette, keybinding preset sets. |
| `06-culling-and-signals.md` | Cull view mode UX; signals-as-columns; the ENRICH ingest stage (cheap) vs enrichment jobs (heavy); burst collapse; suggested rejects. |
| `07-review.md` | The Review task view: categories, review grammar, resolution actions; automation rules (deferred, vocabulary reserved). |

## Where the project is right now

**Design complete for the surfaces above; implementation not started.** Frontend implementation
remains sequenced *after* the backend seam/query-layer design round (backend → seam → frontend),
per the standing decision. The existing `frontend/src/` skeleton (shell, tokens, themes, mock
API, models) is consistent with this design's skeleton; `src/models/` is slated for replacement
by generated types once Wails bindings are wired.

### What this round deliberately deferred

- Review **automation rules** (design reserved in `07-review.md`; build after Review v1 is used)
- The **NL→AST local-LLM tier** (deterministic parser ships first; LLM is an optional add-on)
- **Map view**, font specimen deep-dive, side-by-side compare polish (P2/P3 per requirements)
- Home/landing view beyond the minimal sketch in `02-flows-and-views.md`

### Next steps

1. Backend query-layer design round → the AST grammar and `QueryAssets` land in the seam.
2. Reconcile `frontend/src/api/contract.ts` against `04-query-system.md`.
3. Docs cull/reconciliation pass over the whole `docs/` surface (Ari to initiate).
