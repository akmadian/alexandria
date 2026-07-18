# 24 — Restore the design library; Button as the first v3 primitive

**Areas:** frontend. **Blocked by:** 23-frontend-token-compiler.md.
**References:** `docs/design-constitution.md` §4 (rung ladder), §17 (hero = fill + fun
overlay), §22.2 (components consume semantic roles exclusively), `frontend/CLAUDE.md` §3/§6
(build method: one primitive at a time; every primitive lands its matrix in the library;
specimens and product share one implementation), `docs/frontend-architecture.md` (module
structure), C10 (registry completeness), C14 (display text is data).

The pre-v3 Button and `#/design-library` route were deleted with the frozen legacy (D29);
this task rebuilds both against the compiler's real emitted variables — the first consumer
of Phase C output, and the branch's first end-to-end green `make check-frontend`.

## Deliverables

1. **Button** (`components/button/`): React Aria Components base, aliased import; `rung`
   prop over the §4 ladder (`ghost | outline | tint | fill | hero`) as an exhaustive C10
   registry; sizes `control` (24, default) and `control-lg` (28); states via RAC `data-*`
   attributes in the CSS Module; consumes only emitted `--alx-*` variables — zero raw
   literals, stylelint-clean. Hero renders fill + the fun-layer overlay from the existing
   hypothesis tokens (§30.8 values are PIN; the recipe round re-tunes values, not the API).
   Register shifts follow the theme's family direction; chrome stays hue-free with the
   accent unset.
2. **The design-library route** (`features/design-library/`, hash-routed at
   `#/design-library` as before): Button's full matrix (rungs × interaction states, forced
   via data-attrs on the component's own markup) plus a live row; theme switcher across all
   four themes; the compiler's generated reference table rendered (swatches/type roles), so
   the library replaces the deleted static swatch pages as the value-inspection surface.
3. **app.tsx compiles.** The three dead feature imports resolved: the library import is
   restored by (2); the grid/filter-bar imports are trimmed to whatever minimal shell
   renders honestly today (decided in-round — a shell that compiles and shows the library
   link beats stubs pretending to be features).
4. **Acceptance:** `make check-frontend` fully green — tsc, eslint, stylelint (including
   the current app.module.css findings, fixed with real tokens), vitest, and the task-23
   freshness gate. `bun run dev` renders the library correctly on all four themes (verify
   in the real browser; RAC portals don't assert well in happy-dom).

## Non-goals

No further primitives (the ladder resumes after task 26). No grid/filter-bar features. No
i18n scaffolding expansion — specimen labels may stay literals with a `ponytail:` marker,
as before (C14 applies when product chrome adopts them).
