# Alexandria Frontend — agent orientation

React 19 + TypeScript + Vite desktop UI for the Alexandria DAM. Talks to a Go engine
through a typed seam (Wails v2, locked 2026-07-07); until the backend binds it runs
entirely against an in-memory mock (`bun run dev`, no Wails/Go).

**Rebuild status (read this honestly):** the frontend is mid-ground-up-rebuild on the
`frontend-design-v3` line. The design system was redesigned from scratch (2026-07-12/13,
D29); `src/` contains a partial rebuild — **the tree is the status; don't trust prose
for it**, and expect `make check-frontend` to be broken between rounds on WIP branches.
Anything in `src/` predating the rebuild is disposable by decision.

## 1. Read before you build — in dependency order

| Doc | Why |
|---|---|
| `../docs/CONSTANTS.md` (C1–C15) | Cross-cutting invariants. About to contradict one? You're wrong; stop. |
| `../docs/frontend-architecture.md` | **The architecture record (LOCKED):** state planes, store shape, seam integration, fetch/retry policy, optimistic-mutation discipline. |
| `../docs/design-constitution.md` + `design/CLAUDE.md` | The design law + the token source map. Load these when work touches look, spacing, type, or color — for pure logic work the §3 contract below suffices. |
| The frontend epic corpus (`ls` the epics directory under project tracking) | State glossary, filter-bar UX, flows/views, keyboard, culling, Review. Read the one your feature lives in. |
| `../docs/seam-contract.md` | The engine↔UI contract the mock implements. |

React Aria is documented live — use the React Aria MCP server; don't guess RAC APIs.

## 2. The mental model

- **The state equation (C2):** `view state = viewMode(query + arrangement, selection +
  cursor)`. View modes are pure renderers over shared state.
- **Three state planes, never blurred:** catalog view-state → the one Zustand store;
  server state → TanStack Query; pre-paint chrome prefs → localStorage. The store never
  holds borrowed server state.
- **The query model is the AST (C6/C7):** every predicate is a `Query{scope, where}`
  tree with generated unions. A new filter capability is a new token, never a new method.
- **Three layers own three things:** the design system owns *look* (tokens), React Aria
  owns *behavior* (ARIA, focus, keyboard), the store/seam own *state/data*.

## 3. The design-system contract for code (the short version)

Truth lives in `design/` (constitution + token source; see `design/CLAUDE.md` for the
full map). What code must know:

- **Semantic variables only.** Components consume `--alx-*` CSS variables; never raw
  colors/sizes, never primitives, never values copied from rendered CSS.
- **The grammar exists — compose it, don't freelance:** type roles are units (size +
  line-height + tracking + weight together — setting `font-size` alone is a defect);
  row intents bind heights to permitted type roles (control 28 / list 24 / text 16);
  spacing is quantum multiples via `--alx-space-N`; radius encodes detachment; the
  accent is never the sole signal for a state.
- **Interim reality:** there is deliberately NO token plumbing in `src/` right now —
  the interim runtime generator and its frozen snapshot were removed (D29) so nobody
  builds on them. `--alx-*` variables arrive when the Phase C compiler emits them from
  `design/tokens/`; until then src CSS references intentionally-unresolved vars and
  the tree does not compile. Building the compiler is the code round's first task.
- **Assets are sacred (§11):** no CSS filters/blends/opacity on content pixels;
  selection shades the cell mat, never the photo; thumbnails fit, never crop.

## 4. The data-flow spine

```
ui → feature hook → TanStack Query → api/ (contract → mock | wails) → engine
ui → store.dispatch(action)                    [the C2 view state]
```

- **All I/O lives in `api/`** (`contract.ts`, `mock.ts`, `client.ts` swap point,
  `queries.ts`). Never import a concrete adapter outside `api/`.
- **`query-model/` is pure** (no I/O, no React); leaves via the registry's strict
  constructors — no bare field/operator literals.
- **`stores/`**: one reducer-style `dispatch`; curated selector hooks only; derive,
  don't store (canonical query/key/counts are memoized derivations).
- Mutations carry absolute values (idempotent); optimistic discipline per the
  architecture record; reads `retry: false` by default; every consumer renders an
  explicit error state.

## 5. Generated types (C13) — never hand-author models

`src/_generated-types/` comes from Go (`make generate` at repo root): vocabulary,
enums, errors, events, models. Never edit; never mirror. Literal unions + `as const` /
`satisfies Record<Key, Entry>` completeness idioms (C10); generated enums are
types-only — bridge to runtime with the completeness trick.

## 6. Building UI

- **RAC for chrome** (menus, dialogs, trees, popovers, forms), imported aliased
  (`Button as AriaButton`); state via RAC `data-*` attributes in CSS Modules.
- **Content surfaces are bespoke** (grid, loupe, cull, compare, filmstrip): bare
  `tanstack-virtual` + store-owned selection. Never RAC collections there.
- **Primitives vs features:** `components/<name>/` is domain-blind chrome;
  `features/<name>/` composes primitives + domain context; features never import
  other features. In doubt, build feature-local, promote later.
- **Build method (ratified):** one primitive at a time, leaves first (Button →
  ToggleButton → Checkbox → Switch), container + item type as one unit, composites
  last. Every primitive lands its matrix in the design-library route
  (`#/design-library`) — specimens and product share one implementation.
- No hardcoded display text (C14): i18n keys, display registries, `Intl` via
  `lib/format.ts`. Logging through `lib/logger.ts` — milestones at Info, detail at
  Debug. Motion: CSS-first, `transform`/`opacity` only, `prefers-reduced-motion` by
  construction.

## 7. Commands · layout

```bash
make check      # tsc + eslint (+ stylelint when restored) + vitest — from frontend/
bun run dev     # vite dev server (mock catalog)
```

```
src/_generated-types/  Go-generated (never edit)
    api/               seam: contract, mock, client swap, TanStack hooks
    query-model/       pure AST: types, registry, serializer, assembler
    stores/            catalog view-state (Zustand) + curated hooks
    components/        PRIMITIVES — RAC behavior + token look
    features/          domain compositions; never import each other
    app/               shell, providers, boot
    styles/            app-base.css · fonts (token vars arrive with the Phase C compiler)
    lib/  i18n/        cx, format, logger, theme · i18next
design/                THE DESIGN SOURCE — see design/CLAUDE.md
```

## 8. Gotchas

- Until the compiler lands, ALL `var(--alx-…)` references are intentionally
  unresolved — that's the construction-site state, not a bug to patch around
  (and never by hardcoding a value).
- Restart the dev server after touching `vite.config.ts` or generated sources
  (HMR serves stale modules). The browser pane's console keeps a stale error
  backlog for deleted files — trust `make check-frontend` and the DOM, not that
  console.
- Verify interactive work in the real browser (`bun run dev`) — RAC portals and
  overlays are hard to assert in happy-dom.
- `cx` is at `@/lib/cx`. Chrome is hue-free; test every state with the accent unset.
