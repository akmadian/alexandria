# Alexandria Frontend

React 19 + TypeScript + Vite desktop UI for the Alexandria DAM. Talks to a Go/Wails
backend through a typed seam; runs against an in-memory mock until the backend binds.

> **Status (2026-07): frontend work is DEFERRED** until the backend milestones and the seam design
> round complete — see `../docs/v2/post-ingest-design/00-START-HERE.md`. Where the v2
> decision log conflicts with the frontend docs below, the decision log wins (known pending changes:
> `SourceStatus` → `enabled` + `connectivity`; keybinding overrides move to the settings KV via
> `getUIState/setUIState`; UI runtime — Wails vs alternatives — is an open question).
> `src/api/contract.ts` remains design-authoritative and deliberately network-shaped.

**Read these first** — they are the source of truth, not this file:
- `../docs/frontend-architecture.md` — the API seam: contract, query model, caching, call patterns. `src/api/` and `src/models/` implement it and are settled.
- `../docs/frontend-ui-architecture.md` — the UI: layout, components, state, styling, i18n, logging, keyboard, testing.
- `../docs/frontend-implementation-guide.md` — hands-on: how to run, conventions with snippets, what's built, what's next, gotchas.

## Commands
```bash
bun run dev     # vite dev server (mock catalog)
bun run check   # typecheck + lint + tests — must pass before committing
bun run test:watch
```

## Non-negotiables (details in the docs above)
- **Files kebab-case.** RAC imports aliased `Aria*`.
- **Server state → `api/queries.ts` hooks only.** Never import `mock-api`/`wails-api` in components (ESLint enforces). Event subscription is the one direct-`api` exception (`features/jobs/use-jobs.ts`).
- **Shared client state → the `LibraryProvider` reducer** (`app/library-state.tsx`). Derive, don't store.
- **Styling → CSS Modules + semantic tokens only** (`--bg-surface`, `--text-primary`, `--accent`). Never raw hex or `--grey-*` in components. Chrome is hue-free; hue is for data (labels, tags, accent).
- **No hardcoded display text.** Every string is an i18n key; enums map through `lib/enum-display.ts`; dates/numbers through `lib/format.ts` (`Intl`).
- **Components** are RAC-based primitives (`components/`) that know nothing about the domain; **features** (`features/`) compose them with hooks and never import each other.
