# Alexandria Frontend

React 19 + TypeScript + Vite desktop UI for the Alexandria DAM. Talks to a Go backend through a
typed seam (Wails v2, locked 2026-07-07); runs against an in-memory mock until the backend binds.

> **Status (2026-07): frontend implementation is DEFERRED** until the backend query-layer round
> and the seam round complete — sequencing in `../_project-tracking/seam/00-START-HERE.md`.
> `src/api/contract.ts` remains design-authoritative and deliberately network-shaped; its known
> deltas against the locked design are the reconciliation ledger in
> `../_project-tracking/seam/01-queries-and-commands.md` — apply them there-first, not ad hoc.

**Read these first** — they are the source of truth, not this file:
- `../_project-tracking/CONSTANTS.md` — the load-bearing invariants (C1–C14). Never
  contradict them casually.
- `../_project-tracking/frontend/00-START-HERE.md` — the design handoff index: flows/views,
  state model, search UX, keyboard/palette, culling, Review.
- `../_project-tracking/frontend/07-code-disposition.md` — per-path verdicts over `src/`
  (what stands, what evolves, what retires). Grounded in the code; specs win conflicts.
- `../_project-tracking/seam/` — the query AST, workhorse methods, events/jobs envelopes.

## Commands
```bash
bun run dev     # vite dev server (mock catalog)
bun run check   # typecheck + lint + tests — must pass before committing
bun run test:watch
```

## Non-negotiables (details in the docs above)
- **Files kebab-case.** RAC imports aliased `Aria*`.
- **Server state → `api/queries.ts` hooks only.** Never import `mock-api`/`wails-api` in components (ESLint enforces). Event subscription is the one direct-`api` exception (`features/jobs/use-jobs.ts`).
- **Shared client state → the `LibraryProvider` reducer** (`app/library-state.tsx`). Derive, don't store. Its target shape is the state equation (C2): `view state = viewMode(query + arrangement, selection + cursor)`.
- **Styling → CSS Modules + semantic tokens only** (`--bg-surface`, `--text-primary`, `--accent`). Never raw hex or `--grey-*` in components. Chrome is hue-free; hue is for data (labels, tags, accent).
- **No hardcoded display text.** Every string is an i18n key (C14); enums map through `lib/enum-display.ts`; dates/numbers through `lib/format.ts` (`Intl`).
- **Components** are RAC-based primitives (`components/`) that know nothing about the domain; **features** (`features/`) compose them with hooks and never import each other.
- **Registries dispatch, conditionals don't** (C10): per-type presentation goes through the type registry with `satisfies Record<K, V>` completeness; nil capability = one fallback at the dispatch point.
- **`src/models/` is frozen** — it retires when generated bindings land (C13). Don't hand-grow it.
