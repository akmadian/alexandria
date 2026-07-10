# Alexandria Frontend — agent orientation

React 19 + TypeScript + Vite desktop UI for the Alexandria DAM. Talks to a Go engine through a
typed seam (Wails v2, locked 2026-07-07); until the backend binds it runs entirely against an
in-memory mock (`bun run dev`, no Wails/Go).

**This file is your map.** It says what to load into your head, which docs to read and why, and how
to build UI here (the design-system ↔ React Aria reconciliation). The deep specs are the source of
truth — this orients you to them and encodes the operational know-how that isn't written elsewhere.
Don't start building until §1–§4 are in your head.

> **Status (2026-07-10): ground-up rebuild UNDERWAY.** Architecture LOCKED:
> `../_project-tracking/frontend/09-ground-up-redesign-notes.md`. The pre-rework `src/` is deleted;
> the new foundation is bottom-up and real (`query-model/` pure AST + registry, `api/` contract +
> AST mock engine + client/hook, `stores/` catalog store, `features/grid/` virtualized DS grid),
> built **in isolation** against a contract-faithful mock, thin-vertical-then-widen. The first
> widen slice landed the **primitives layer** (`components/` button/popover/menu/field — RAC
> behavior, DS-ported look) and an interactive **filter bar** (`features/filter-bar/`): a generic
> pill + a **per-kind value-editor registry** (enum/numeric/text today; date/tag/source next) +
> `query-model/assemble` + store filter actions. Status + next slices:
> `../_project-tracking/frontend/00-START-HERE.md`.

---

## 1. Read before you build — the map, in dependency order

These decide things; don't reinvent them. Read top-down — each assumes the ones above. You don't
need all of 01–08 every time; always load CONSTANTS + `09`, then the doc your feature lives in.

| Doc | Why you need it |
|---|---|
| `../_project-tracking/CONSTANTS.md` (C1–C14) | The cross-cutting invariants every surface obeys — vocabulary, the state equation, query-model-is-the-AST, registries, generated-types, i18n. About to contradict one? You're wrong; stop. |
| `../_project-tracking/frontend/09-ground-up-redesign-notes.md` | **The architecture record (LOCKED).** State planes, the store shape + action vocabulary + invariants, seam integration, fetch/perf/retry policy, the token/AST contract, optimistic-mutation discipline. Wins over 01–08 for architecture; §6 below is its enforcement summary. |
| `../_project-tracking/frontend/02-state-model.md` · `03-search-and-filter-ux.md` | The state glossary (scope/filter/query/selection/cursor/arrangement/view-mode) and the filter-bar/pill/search-tier UX. Read before touching the store or the filter bar. |
| `../_project-tracking/frontend/{01,04,05,06,08}` | Flows/views · keyboard+actions+palette · culling+signals · Review · **design language** (08 is the visual authority: neutral chrome, canvas, flat construction). Read the one your feature is in. |
| `../_project-tracking/seam/01-queries-and-commands.md` | The engine↔UI contract: the query AST, the workhorse methods (`queryAssets`/`updateAssets`/`assetIdSlice`/`indexOfAsset`/`distinctValues`), the event/job envelopes. The mock implements this; the Wails adapter will too. |
| `src/styles/ds-reference/CLAUDE.md` + `PORTING.md` | How to consume the design system: golden rules (semantic tokens, hue-free chrome, flat construction), the token vocabulary, the component-port recipe. `components-spec/*.css` are per-component **visual specs** — read to port, never import. |

**React Aria is documented live** — use the React Aria MCP server (`list_react_aria_pages`,
`get_react_aria_page`, `get_react_aria_page_info`) as your reference. Don't guess RAC APIs or invent
props. Before porting a primitive, read that component's RAC page — its composition (which
sub-components nest) and its `data-*` state model.

## 2. The mental model to hold

- **The state equation (C2):** `view state = viewMode(query + arrangement, selection + cursor)`.
  Everything on screen derives from these five. View modes are pure renderers over shared state;
  switching is instant and stateless.
- **Three state planes, never blurred** (rules in §6): catalog view-state → the one Zustand store;
  server state → TanStack Query; pre-paint chrome prefs → localStorage. **The store never holds
  borrowed server state.**
- **The query model is the AST (C6).** Every predicate over assets is a `Query{scope, where}` tree
  of `Leaf{field, cmp, value}`, where `field`/`cmp` are the generated unions. A new filter
  capability is a new *token*, never a new query method (C7). Never hand-write a flat filter object
  — that shape is retired.
- **Primitives vs features** — the distinction that decides where code goes (§3).
- **Three layers own three things:** the **design system** owns *look* (tokens + the `.css`
  specs), **React Aria** owns *behavior/structure* (ARIA, focus, keyboard, interaction), and **the
  store/seam** own *state/data*. You compose them; you reinvent none of them.

## 3. Building UI — reconcile the Design System with React Aria

This is the core skill. The DS ships each component as a `.css` **visual spec**
(`src/styles/ds-reference/components-spec/<Name>.css`) plus a token layer already vendored into
`src/styles/alexandria-ds.css`. You do **not** ship the DS's `.jsx` or its `.ax-*` classes — you
re-express each spec as a colocated CSS Module driven by a React Aria primitive.

**Read the `.css` as a spec for LOOK — be critical of its STRUCTURE.** The DS is built as a
standalone library, so it bakes behavior into its markup: hand-rolled `position:fixed` menus, its
own `data-active`/`data-open` attributes, monolithic components that fuse a control with its
dropdown. That decomposition is *not* automatically right for us. React Aria already solves
positioning, focus containment, dismiss, keyboard, and ARIA — take those from RAC and drop the DS's
hand-rolled versions. Port the rule *bodies* (every `var(--…)` is already correct — both sides share
the tokens); restructure the *composition* the RAC way. (Worked example: the DS `FilterPill.css`
bakes an entire fixed-position operator menu into the pill; we dropped it and used the `menu`
primitive, porting only the segment look.)

**The three-move port** (full recipe + state table in `PORTING.md`):
1. **Mount the RAC primitive** — look up the DS→RAC mapping in `PORTING.md`, then read its React
   Aria page. It brings ARIA/focus/keyboard for free.
2. **Class → module class:** `.ax-btn {…}` → `.button {…}`, applied via `cx(s.button, …)`. Copy rule
   bodies verbatim.
3. **Remap state selectors** to RAC `data-*`: `:hover`→`&[data-hovered]`, `:active`→`&[data-pressed]`,
   `:focus-visible`→`&[data-focus-visible]`, trigger-open→`&[aria-expanded="true"]`,
   `[disabled]`→`&[data-disabled]`, selected→`&[data-selected]`.

RAC primitives are imported aliased `Aria*` (`import { Button as AriaButton }`).

**Primitives vs features — where code goes:**
- A **primitive** (`components/<name>/`) is domain-blind chrome: it knows nothing about assets or
  queries. A thin RAC wrapper + a DS-ported module, reusable everywhere. Built so far:
  `components/button`, `components/popover`, `components/menu` — **study these as the template.** The
  glass *surface* lives once on `popover`; menus/selects mount inside it (never re-port the surface
  into each dropdown).
- A **feature** (`features/<name>/`) composes primitives **plus domain context** — which token,
  which operators are legal, how a value formats, what action dispatches. `features/filter-bar/` is
  the worked reference: it composes `menu`/`button` with the query-model registry. Its pill
  *segment* look is feature-local (a capsule isn't a shared primitive), but every interactive
  segment is still a RAC `Button`/`MenuTrigger` underneath — **even inside a feature, interactive
  bits use RAC as the behavioral base.**
- **Content surfaces are bespoke, NOT RAC:** grid, loupe, cull, compare, filmstrip render on
  `tanstack-virtual` with store-owned selection. RAC's collection/Virtualizer components own
  selection — wrong tool here. Their *look* still ports from the DS `.css`; only behavior is
  hand-built. `features/grid/` is the reference.

Decide with: "domain-blind and reusable?" → primitive. "encodes how the user drives
assets/queries?" → feature. In doubt, build it feature-local; promote to a primitive when a second
feature needs it.

## 4. The data-flow spine

```
ui (feature components)
  ├─ read  ──→ feature hook ──→ TanStack Query ──→ api/ (contract → mock | wails) ──→ engine
  └─ write ──→ store.dispatch(action)                       [the C2 view state]
```

- **All I/O lives in `api/`** — `contract.ts` (the `AlexandriaAPI` interface), `mock.ts` (the
  in-memory AST query engine standing in for SQL), `client.ts` (the one-line swap point),
  `queries.ts` (TanStack hooks). Components → feature hooks → TanStack → `api/`. Never import a
  concrete adapter or the Wails runtime outside `api/`.
- **`query-model/` is pure** (no I/O, no React): `ast.ts` types · `registry.ts` (token dictionary
  seeded from the generated `fieldGrammar`, strict `leaf()` + the `validate()` gate) · `serialize.ts`
  (the stable cache key) · `assemble.ts` (tree edits behind the pill row).
- **`stores/` is the C2 view state:** `catalog-store.ts` (Zustand, one reducer-style `dispatch`,
  `{ids}|{all}` selection, curated selector hooks). The store never fetches, never holds server
  state; the canonical query + key + counts are memoized *derivations*, never stored.
- **The swap is one line.** The mock is a faithful `AlexandriaAPI` implementation, so when the Wails
  adapter lands, `client.ts` points at it and nothing else changes.

## 5. Generated types (C13) — never hand-author models

`src/_generated-types/` is generated from Go (`internal/seam/generate`): `vocabulary.ts`
(TokenField/TokenOperator/ValueKind + `fieldGrammar`), `enums.ts`, `errors.ts`, `events.ts`. Never
edit them; never hand-maintain a parallel model type. Build leaves through the registry's strict
constructors — no bare field/operator string literals, no TS `enum` keyword (literal unions +
`as const` / `satisfies Record<Key,Entry>` are the idiom, C10). One caveat you'll hit: the generated
enums are **types-only** (no runtime value arrays), so when you need enum members at runtime (a value
picker), bridge with the completeness trick — a `Record<TheEnum, …>`-keyed literal whose keys you
read back — so a newly generated member breaks the build until handled. See
`features/filter-bar/enum-fields.ts`.

## 6. Coding standards (the rebuild rules, rationale in `09`)

### State — three planes
- Catalog view state → the one Zustand store (single reducer-style `dispatch`; the action
  vocabulary is the app's internal API). Server state → TanStack Query only. Pre-paint chrome prefs
  (theme, pane widths, density, view mode) → localStorage.
- **The store never holds borrowed server state.** If the backend returned it, it lives in the query
  cache and is read there at render.
- Components never write inline store selectors: `stores/` exports curated selector hooks
  (`useCursorId()`, `useIsSelected(id)`, …); store internals stay private (convention — see §7).
  Selectors return primitives or stable references.
- Derive, don't store (C2): the canonical query, query key, and counts are memoized derivations —
  never `set` anywhere.

### Seam and data
- Long `staleTime` + event-driven invalidation (the engine pushes C8 events — we own the freshness
  signal). `refetchOnWindowFocus` / `refetchOnReconnect` off.
- **Mutation payloads carry absolute values, never deltas** — idempotent by construction.
- Optimistic discipline (per `09` §Optimistic mutation): cancel-on-mutate + invalidation gate while
  catalog-editing mutations are in flight; ONE ordered lane for mutations + undo/redo; undo/redo
  render pessimistically; optimism only for ids-shaped targets; failure = revert + toast, loud never
  silent.
- Reads: `retry: false` by default; named transient codes opt in (1–2 retries, capped backoff).
  Every query consumer renders an explicit error state — nothing spins forever.

### Structure
- bulletproof-react layout; kebab-case files; import boundaries: **shared → features → app, and
  features never import other features** (shared code moves down instead).
- Registries dispatch, conditionals don't (C10): `satisfies Record<Key, Entry>` completeness +
  `never`-checked switches; nil capability = one fallback path at the dispatch point.

### UI
- **RAC for chrome** (menus, dialogs, trees, popovers, palette, forms), aliased `Aria*`. **Content
  surfaces are bespoke** (grid, loupe, cull, compare, filmstrip — bare virtualizer, store-owned
  selection).
- CSS Modules + **semantic tokens only** (never raw hex or a primitive `--n-*`/`--grey-*` in a
  component). Chrome is hue-free; the accent is optional enhancement, never the sole signal — every
  state must read with `--accent` unset. `08-design-language.md` is the visual authority.
- Motion: CSS-first; animate `transform`/`opacity` only; per-frame content mutation goes through a
  `useRafLoop` (auto-cancel on unmount); gate ambient motion behind `useMotion()`
  (`prefers-reduced-motion` degrades to instant swaps).
- **No hardcoded display text (C14):** every string is an i18n key; enums map through display
  registries; dates/numbers/sizes through `Intl` (`lib/format.ts`).
- Comprehensive logging through the frontend logger bridge (`lib/logger.ts`): milestones/results at
  Info, per-item detail at Debug — not error-only.

## 7. What the machine enforces vs what you hold yourself

`make check` (from `frontend/`) runs `tsc -b --noEmit && eslint src && vitest run`. It catches:
- **Type completeness (tsc):** generated-type exhaustiveness, `satisfies Record<Key,Entry>` registry
  completeness, strict constructors.
- **jsx-a11y** recommended, **react-hooks** (rules-of-hooks, exhaustive-deps, set-state-in-effect).
- **One import boundary:** only `api/` may import a concrete backend impl.

It does **not** yet mechanically catch (hold these by discipline + review — the linter won't stop
you, the pre-commit review will): semantic-tokens-only, features-never-import-features,
store-internals-only-through-curated-hooks, no-hardcoded-display-strings. Treat a violation as a
defect, not a style nit.

## 8. Commands · where things live

```bash
make check      # tsc + eslint + vitest — must pass before commit (run from frontend/)
make lint       # eslint only          make test   # vitest only
bun run dev     # vite dev server (mock catalog)    bun run test:watch
```
From the repo root: `make check-frontend`.

```
src/_generated-types/  Go-generated types (never edit): vocabulary, enums, errors, events
    api/               the seam: contract, mock engine, client swap, TanStack hooks, ApiError
    query-model/       pure AST: types, token registry, serializer, assembler (no I/O, no React)
    stores/            catalog view-state store + ids; curated selector hooks
    components/         PRIMITIVES — RAC behavior + DS-ported look (button, popover, menu, …)
    features/           grid, filter-bar, … — primitives + domain context; never import each other
    app/               shell, providers, boot
    styles/            alexandria-ds.css (vendored tokens+fonts) · ds-reference/ (specs + PORTING) · app-base
    lib/               cx, format (Intl), logger, theme
    i18n/              i18next init + locales/en.json
```

## 9. Gotchas

- **Tokens must resolve.** If a `var(--control-bg)` renders unstyled, you referenced a token not in
  `alexandria-ds.css` — grep it before porting a rule that uses it.
- Don't resolve semantic tokens down to primitives or hex; the theme + material system depend on the
  semantic layer.
- Don't reach for a RAC collection just to get virtualization — content surfaces are bespoke on
  `tanstack-virtual`, selection from the store.
- `cx` is at `@/lib/cx` — use it (matches every existing component).
- Chrome is hue-free and every state must read with `--accent` unset — test selection/focus/hover
  with no accent set.
- Verify interactive work in the browser (`bun run dev`), not just via tests — RAC overlays/portals
  and DS glass are hard to assert in happy-dom; drive the real flow.
