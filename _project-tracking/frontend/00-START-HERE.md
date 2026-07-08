# Alexandria Frontend Design Handoff — START HERE (frontend area)

*Task-tree head: [`../00-START-HERE.md`](../00-START-HERE.md) — check it first for what's next.*

**Date:** 2026-07-07
**Produced by:** a ground-up frontend/UX design session (Ari + Claude Fable) that worked forward
from `_project-tracking/functional-requirements.md` and the conventions of LrC, Eagle, Photo Mechanic, and
Capture One, followed by a same-day docs reconciliation pass that split the material by concern.
**Audience:** a Claude instance (or Ari) doing further design refinement and/or implementation.

This set **supersedes** the deleted `frontend-architecture.md` / `frontend-ui-architecture.md` /
`frontend-implementation-guide.md`. What isn't here lives next door:

- **`../CONSTANTS.md`** — the cross-cutting invariants (C1–C14). Read it first, always.
- **`../seam/`** — the engine↔UI contract: query AST, workhorse methods, events, jobs, binary
  channel, and the contract.ts reconciliation ledger. The seam is its own concern, owned jointly.
- **`../backend/06-signals-and-enrichment.md`** — the engine side of AI-assisted culling
  (ENRICH stage, enrichment jobs).

## Headline decisions

- **UI runtime: Wails v2 — LOCKED** (Ari, 2026-07-07).
- **Boring skeleton, signature concepts.** The shell is the conventional DAM layout; the novelty
  budget is spent on the query model, cull speed, Review, and transparency-as-chrome.
- **The state equation** (C2): `view state = viewMode(query + arrangement, selection + cursor)`.

## The doc set

| Doc | Contents |
|---|---|
| `01-flows-and-views.md` | The four user flows; the task-shaped test; view inventory (catalog, task views, home); shell layout; transparency chrome. |
| `02-state-model.md` | The glossary in full: scope, filter, query, working set, selection, cursor, arrangement, view mode. State equation and command-targeting rule. |
| `03-search-and-filter-ux.md` | Filter bar, pills, save-as-smart-collection, plain-language narration, the search tiers (deterministic → optional local LLM → FTS). |
| `04-keyboard-and-actions.md` | Action registry, context dispatch, verb grammar, per-type media verbs, command palette, keybinding preset sets. |
| `05-culling-and-signals.md` | Cull view mode UX; signals-as-columns (C11); burst collapse; suggested rejects. Engine side in `../backend/06-signals-and-enrichment.md`. |
| `06-review.md` | The Review task view: categories, review grammar, resolution actions; automation rules (deferred, vocabulary reserved). |
| `07-code-disposition.md` | Per-path verdict over the existing `frontend/src/` — what stands, what evolves, what rebuilds. Grounded in the actual code, not assumptions. |

## Where the project is right now

**Design complete for the surfaces above; implementation not started.** Sequencing (see
`../seam/00-START-HERE.md`): backend query-layer round → seam round (contract reconciliation +
bindings) → frontend implementation. The existing `frontend/src/` skeleton is *better* than
expected — see `07-code-disposition.md`; "nuke and rebuild" is warranted almost nowhere.

### What this round deliberately deferred

- Review **automation rules** (design reserved in `06-review.md`)
- The **NL→AST local-LLM tier** (deterministic parser ships first)
- **Map view**, font specimen deep-dive, compare polish (P2/P3 per requirements)
- Home/landing view beyond the minimal sketch
