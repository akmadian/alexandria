# 35 — The block-model widen: retire the 500-row page cap

**Areas:** frontend. **Blocked by:** nothing (runs parallel to 34 by decision; lands first —
34's optimistic row-patch helper rebases onto the block-shaped caches).

**References:** `docs/frontend-architecture.md` §Fetching and performance (the AG-Grid-style
infinite row model is specced there — implement that, don't invent), the `ponytail:` markers at
`frontend/src/api/queries.ts` (the single-wide-page cap; "touches only this hook" was
optimistic — the grid's anchor-index derivation moves too) and `frontend/src/features/grid/grid.tsx`
(anchor index from the loaded page), and the 2026-07-19 dated note in the architecture record
(cursor index hint: `findIndex` over the loaded page today, `IndexOfAsset` at this widen).
Binding law: C2 (the grid stays a pure renderer), C4 (arrangement is in the block key — a block
is a window into an ordered result).

## Scope

- Replace the single-page `useQueryAssets` with fixed-size blocks keyed by
  `(serialized query+arrangement, blockIndex)` via `useQueries`: fetch viewport + buffer blocks
  only, debounce during fling, LRU-cap resident blocks, `total` sizes the scrollbar before any
  block lands. Unloaded rows render the grey placeholder mat (already the loading convention).
- Cursor index derivation moves to the seam where the loaded blocks can't answer:
  `indexOfAsset` (already bound) when the cursor's asset is outside resident blocks; loaded
  blocks answer locally. Range-selection interiors materialize via `assetIdSlice` (already
  bound) — but ONLY if the gesture already exists; this round widens the data layer, it does
  not add gestures.
- The page-cap warn log in `queries.ts` retires with the cap.
- Out of scope: the inspector, the api contract (offset/limit paging already supports blocks),
  mutations (task 34 owns them), any grid visual change beyond placeholder mats for
  not-yet-loaded rows.

## Acceptance

- Mock catalog (64 rows, dev-sized blocks in tests) exercises: block fetch on scroll, LRU
  eviction, scrollbar sized by `total` pre-load, cursor arrow-nav across a block boundary.
- Real catalog (`wails dev`) scrolls a working set larger than 500 without truncation — the
  old cap's permanent placeholder rows are gone.
- `make check` green; the queries.ts ponytail markers that name this widen are removed; the
  architecture record's dated note gets its "landed" companion note only if wording there
  demands it (never a status ledger); fold-and-delete of this file prepared in the working
  tree (uncommitted).
