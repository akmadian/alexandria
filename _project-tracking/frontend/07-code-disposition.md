# Frontend Code Disposition

> **AMENDED 2026-07-08:** superseded by the ground-up redesign round
> (`09-ground-up-redesign-notes.md`). All of `frontend/src/` is disposable — the per-path
> verdicts below no longer bind. Kept for reference: the *patterns* worth re-deriving
> (contract/mock swap, reducer store, virtualized grid) are re-ratified on their merits in `09`.

**Date:** 2026-07-07 (docs reconciliation pass, grounded by reading the actual code — not
assumptions). Verdict format mirrors `../backend/05-code-disposition.md`: specs win every
conflict; this table is the license for what happens to `frontend/src/` when implementation
resumes.

## The headline

**"Very stale, probably nuked" is wrong — the pleasant surprise of this pass.** The staleness is
*vocabulary and drift* (Settings fields, SourceStatus shape, keybinding storage, flat filter),
not rot. The architecture layer anticipated most of the new design: contract.ts already has
scope×filter×sort×page, sparse patches, one job envelope, typed error codes, and a binary-URL
channel; library-state already is one reducer with derive-don't-store and a mouse/keyboard-shared
action model; the grid is already virtualized (@tanstack/react-virtual) with the sparse-window
upgrade path documented in a ponytail comment. Evolve in place; rebuild nothing wholesale.

## Per-path verdicts

| Path | Verdict | Notes |
|---|---|---|
| `api/contract.ts` | **KEEP + EVOLVE** | The reconciliation ledger (`../seam/01` §Reconciliation) is the exact work list. Its header conventions are adopted as standing seam conventions. |
| `api/mock-api.ts`, `api/mock.ts`, `api/queries.ts` | **KEEP** | The mock-behind-contract pattern is what lets frontend work proceed before bindings; evolves in lockstep with the contract. |
| `api/mock-api.check.ts` | KEEP | Contract-conformance check; keeps mock honest. |
| `models/*` | **RETIRE at bindings** | C13: generated from Go (Wails bindings/tygo). Until then it's the stand-in; don't hand-grow it further. |
| `app/library-state.tsx` | **EVOLVE — vocabulary refactor** | Architecture confirmed (reducer, split state/dispatch contexts, pure + tested). Rename/reshape to the locked model: `BrowseTarget`→scope (tag becomes a scope kind, not a filter field — ledger #2); `FilterBarState` splits into filter (AST leaves) + arrangement (sort/group); `density` moves out of query state into localStorage prefs; `viewMode` gains `compare`/`cull`; `lastSelectedId` formalizes into **cursor** (C1); `deriveListQuery` becomes the AST builder. |
| `app/shell.tsx`, `shell.module.css` | KEEP | The boring skeleton is the design. Gains task-view host + activity drawer mount. |
| `app/error-boundary.tsx` | KEEP | Per-pane boundaries are P1 requirements. |
| `styles/tokens.css`, `styles/themes/*` | KEEP | Semantic tokens + hue-free chrome confirmed by the design round. Graphite theme still to add (only dark/light exist). |
| `lib/keys.ts` | **EVOLVE → action registry** | Currently a handler-registration helper; grows into the `{id, title, aliases, context, handler, binding}` registry of `04-keyboard-and-actions.md`. Tests exist — keep them green through the refactor. |
| `lib/` (cx, format, theme, logger, enum-display) | KEEP | All still design-conformant (C14, logging bridge, tokens). |
| `i18n/` | KEEP | Mechanism is a P1 requirement; already wired. |
| `components/*` (RAC primitives: tree, modal, toast, button, …) | KEEP | Domain-blind primitives; the palette, pills, and Review rows compose from these. |
| `features/grid` | **KEEP + EXTEND** | Already virtualized with selection + keyboard triage dispatching reducer actions. Documented next step (sparse windowed fetch) stands. Gains overlays config, group/stack cards, Review corner ticks. |
| `features/browser`, `features/inspector`, `features/filter-bar`, `features/jobs` | **EVOLVE per specs** | Browser: tree modes → scope setting stands. Filter bar: rebuilds around pills/AST (largest delta). Inspector: gains per-type adaptation via the type registry. Jobs/status-bar: adopts the reconciled envelope + activity drawer. |
| `frontend/CLAUDE.md` | **REWRITTEN this pass** | Was pointing at three deleted docs. |

## New surfaces with no code yet

Loupe/Compare/Cull view modes, command palette, task-view flows (Import/Review/Settings/first-
run), pill filter bar, activity drawer, home view. All compose from kept primitives + the
evolved state store; none requires discarding existing code.

## Sequencing reminder

None of the EVOLVE work starts until the backend query round and seam round land
(`../seam/00-START-HERE.md`) — refactoring library-state to a vocabulary whose AST types don't
exist yet would just be churn.
