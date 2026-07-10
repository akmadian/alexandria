# Alexandria Frontend Design Handoff — START HERE (frontend area)

*Task-tree head: [`../00-START-HERE.md`](../00-START-HERE.md) — check it first for what's next.*

**Date:** 2026-07-07 · architecture superseded by the ground-up redesign round 2026-07-08 (`09`)
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
- **Ground-up rebuild (Ari, 2026-07-08):** all of `frontend/src/` is disposable; the rebuild's
  architecture is re-derived from the requirements, CONSTANTS, the seam design, and `08` — the
  locked record is `09-ground-up-redesign-notes.md`.

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
| `08-design-language.md` | Visual language (2026-07-08 round): no-accent neutral color rules, middle-grey canvas + darker panels, flat + transient blur, dot-matrix glyph system, motion, font candidates, layout amendments. Supersedes the `src/styles/` visual decisions; feeds the Claude Design handoff. |
| `09-ground-up-redesign-notes.md` | **The frontend architecture record (LOCKED 2026-07-08)**: state planes + the store (shape, action vocabulary, invariants), seam integration, module structure, fetch/perf/retry policy, token/AST frontend contract (triad, value kinds, negation, dates, versioning), optimistic-mutation × undo discipline, and the seam requirements fed to the backend query round. Supersedes `07`'s verdicts; where it conflicts with 01–08 it wins for architecture. |

## Where the project is right now

**Design complete; architecture locked (2026-07-08, `09`); ground-up rebuild IMPLEMENTATION
STARTED (2026-07-10).** The backend query-layer + seam rounds are done, so the rebuild is
unblocked and underway, built **in isolation** (a contract-faithful mock, no Wails/Go — `bun run
dev`) via a **thin end-to-end vertical, then widen** strategy.

**Foundation vertical landed (2026-07-10):** the pre-rework `frontend/src/` cluster is deleted
(the `09` disposability decision, executed); the new bottom-up foundation is in place —

- `query-model/` — pure AST (`ast.ts`), token registry seeded from the generated `fieldGrammar`
  (`registry.ts`), stable query serializer (`serialize.ts`); `leaf`/`validate` gates. Tests.
- `api/` — the reconciled `AlexandriaAPI` **contract** (AST query model, not the retired flat
  filter); the **mock** = a real in-memory AST query engine (`evaluate`/sort+tiebreaker/page/total)
  standing in for SQL; the `client` swap point + `useQueryAssets` hook. Tests.
- `stores/catalog-store.ts` — the full C2 `CatalogViewState` (Zustand, reducer `dispatch`,
  `{ids}|{all}` selection, curated selectors, memoized canonical-query derivation).
- `features/grid/` — the bespoke `tanstack-virtual` grid on the DS `GridCell` port, over the real
  `queryAssets`; store-owned selection/cursor. `app/` shell + providers + DS wiring.

This is the frontend-side of the seam `contract.ts` reconciliation the seam round deferred to the
"wails-dev pass" (`../seam/01` ledger, `../backend/impl/DEFERRED.md §7`). **Widen next:** windowed
block fetch (+ switch range materialization to the `assetIdSlice` seam call, + cursor auto-seed via
a `working-set-changed(total)` echo — both marked `ponytail:` in the store/grid) · filter bar +
pills (query-model `parseValue`/kind-editors) · sidebar Browser trees · mutations + optimistic/undo ·
event pump · inspector/status-bar/palette.

The patterns worth keeping were re-ratified on their merits in `09`, not preserved as code.

### What this round deliberately deferred

- Review **automation rules** (design reserved in `06-review.md`)
- The **NL→AST local-LLM tier** (deterministic parser ships first)
- **Map view**, font specimen deep-dive, compare polish (P2/P3 per requirements)
- Home/landing view beyond the minimal sketch
