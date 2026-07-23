# Alexandria Frontend — agent orientation

React 19 + TypeScript + Vite desktop UI for the Alexandria DAM. Talks to a Go engine
through a typed seam (Wails v2, locked 2026-07-07). The read slice is bound for real
(2026-07-18): `wails dev` runs the full stack (real catalog, real thumbnails), while
`bun run dev` (no Wails/Go) keeps the in-memory mock — `api/client.ts` picks by
runtime presence of the Wails bridge.

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
- **The compiler is the bridge (D31, 2026-07-17):** `design/compiler/` (bun + TS)
  resolves `design/tokens/`, executes `contracts.json` — a failing contract blocks
  emission — and emits `src/styles/tokens.css` + `tokens.ts` (the generated theme
  vocabulary) + `tokens-reference.json`. Regenerate: `bun run generate:tokens`;
  `bun run check` freshness-gates the committed output. Emitted names are the strict
  path mirror (`--alx-` + token path, dots → hyphens) plus one `.alx-type-<role>`
  unit class per type role. Never edit emitted files; never resurrect a runtime
  generator (D29).
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
- **Structural containers group under `components/layout/`** (`Pane`, `StatusBar`; Storybook
  title `Layout/*`) — the docked shells that hold other chrome, kept apart from control
  primitives. Flat `components/<name>/` stays the default; promote a container into `layout/`
  once a second one exists (the rule that moved Pane + StatusBar there).
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
make check      # token generate + freshness + tsc + eslint + stylelint + vitest
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
    styles/            tokens.css|.ts (GENERATED — bun run generate:tokens) · app-base.css · fonts
    lib/  i18n/        cx, format, logger, theme · i18next
design/                THE DESIGN SOURCE — see design/CLAUDE.md
```

## 8. Gotchas

- Emitted `--alx-*` variables are real now (`styles/tokens.css`); a var that
  doesn't resolve is a NAME error against the strict path mirror (look it up in
  `styles/tokens-reference.json`) — never something to hardcode around.
- Restart the dev server after touching `vite.config.ts` or generated sources
  (HMR serves stale modules). The browser pane's console keeps a stale error
  backlog for deleted files — trust `make check-frontend` and the DOM, not that
  console.
- Verify interactive work in the real browser (`bun run dev`) — RAC portals and
  overlays are hard to assert in happy-dom.
- The browser pane's synthesized clicks/keys CANNOT drive RAC checkbox-family
  toggles (label→hidden-input press routing; proven 2026-07-17 against Adobe's
  own live Checkbox docs — identical dead clicks there). Verify those with unit
  tests + an in-page `input.click()` probe; a dead pane click on a checkbox is
  not a product bug.
- Two more pane traps (2026-07-17): `navigate` to the SAME hash URL is a soft
  hash-change — the stale bundle keeps running while Vite serves fresh source;
  reload with in-page `window.location.reload()`. And the pane's
  ResizeObserver NEVER fires (not even the initial observation) — resize
  behavior can't be verified there.
- `cx` is at `@/lib/cx`. Chrome is hue-free; test every state with the accent unset.
