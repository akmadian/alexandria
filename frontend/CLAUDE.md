# Alexandria Frontend

React + TypeScript + Vite desktop UI for the Alexandria DAM. Talks to a Go backend through a
typed seam (Wails v2, locked 2026-07-07); runs against an in-memory mock until the backend binds.

> **Status (2026-07-08): ground-up rebuild decided — everything under `src/` is disposable.**
> The rebuild's architecture is LOCKED: `../_project-tracking/frontend/09-ground-up-redesign-notes.md`.
> Implementation remains DEFERRED until the backend query-layer round and the seam round complete
> (sequencing in `../_project-tracking/seam/00-START-HERE.md`). Do not evolve the existing
> `src/` — it is reference-only until the rebuild replaces it.

**Read these first** — they are the source of truth, not this file:

- `../_project-tracking/CONSTANTS.md` — the load-bearing invariants (C1–C14). Never contradict
  them casually.
- `../_project-tracking/frontend/09-ground-up-redesign-notes.md` — the architecture record:
  state planes, the store, seam integration, module structure, fetch/perf policy, token/AST
  contract, optimistic-mutation discipline. The rules below are its enforcement summary.
- `../_project-tracking/frontend/00-START-HERE.md` — the design handoff index (flows/views,
  state model, search UX, keyboard/palette, culling, Review, design language).
- `../_project-tracking/seam/` — the query AST, workhorse methods, events/jobs envelopes.

## Commands

```bash
make check      # typecheck + lint + tests — must pass before committing
make lint       # eslint only
make test       # vitest only
bun run dev     # vite dev server (mock catalog)
bun run test:watch
```

## Coding standards for the rebuild (rationale in `09`)

### State — three planes, never blurred

- Catalog view state → the one Zustand store (single reducer-style `dispatch`; the action
  vocabulary is the app's internal API). Server state → TanStack Query only. Pre-paint UI
  chrome prefs (theme, pane widths, density, view mode) → localStorage.
- **The store never holds borrowed server state.** If the backend returned it, it lives in the
  query cache and is read from there at render.
- Components never write inline store selectors: `stores/` exports curated selector hooks
  (`useCursor()`, `useIsSelected(id)`, …); store internals are private (ESLint
  `no-restricted-imports`). Selectors return primitives or stable references.
- Derive, don't store (C2): the canonical query, query key, and counts are memoized
  derivations — never `set` anywhere.

### Seam and data

- **All I/O lives in `api/`** (contract, Wails adapter, mock, event pump, `ApiError`
  normalization). Components → feature hooks → TanStack → `api/`. Never import adapters or the
  Wails runtime outside `api/`.
- Long `staleTime` + event-driven invalidation (the engine pushes C8 events — we own the
  freshness signal). `refetchOnWindowFocus` and `refetchOnReconnect` are off.
- **Mutation payloads carry absolute values, never deltas** — idempotent by construction.
- Optimistic discipline (per `09` §Optimistic mutation): cancel-on-mutate + invalidation gate
  while catalog-editing mutations are in flight; ONE ordered lane for mutations + undo/redo;
  undo/redo render pessimistically; optimism only for ids-shaped targets; failure = revert +
  toast, loud never silent.
- Reads: `retry: false` by default; named transient codes opt in (1–2 retries, capped backoff).
  Every query consumer renders an explicit error state — nothing spins forever.

### Types and the query model

- `_generated-types/` is generated from Go (C13) — never hand-edited; no hand-maintained
  parallel model types, ever.
- **No bare field/operator strings**: `TokenField` / `TokenOperator` generated literal unions
  only (no TS `enum` keyword). Leaves are built through the registry's strict constructors;
  `validate()` gates persisted trees (unknown token → inert pill, never a crash or a dropped
  predicate).
- `query-model/` stays pure: no I/O, no React imports. Vocabulary (tag/source/camera names) is
  passed in by feature hooks, never fetched from inside.

### Structure

- bulletproof-react layout; kebab-case files; import boundaries enforced by ESLint:
  shared → features → app, and **features never import other features** (shared code moves
  down instead).
- Registries dispatch, conditionals don't (C10): `satisfies Record<Key, Entry>` completeness +
  `never`-checked switches; nil capability = one fallback path at the dispatch point.

### UI

- **RAC for chrome** (menus, dialogs, trees, popovers, palette, forms), imports aliased
  `Aria*`. **Content surfaces are bespoke** (grid, loupe, cull, compare, filmstrip — bare
  virtualizer, store-owned selection).
- CSS Modules + semantic tokens only (never raw hex or primitive `--grey-*` in components).
  Chrome is hue-free; the accent is optional enhancement, never the sole signal — every state
  must read with the accent unset (`08-design-language.md` is the visual authority).
- Motion: CSS-first; animate `transform`/`opacity` only; per-frame content mutation goes
  through `useRafLoop` (auto-cancel on unmount); everything gated by `useMotion()`
  (`prefers-reduced-motion` degrades ambient motion to instant swaps).
- **No hardcoded display text** (C14): every string is an i18n key; enums map through display
  registries; dates/numbers/sizes through `Intl`.
- Comprehensive logging through the frontend logger bridge: milestones and results at Info,
  per-item detail at Debug — not error-only.
