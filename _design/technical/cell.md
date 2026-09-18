# The grid cell

Cell round, 2026-09-18. Companion to `grid.md`, which deliberately left
cell content ("badges/labels/layout — cell round") to this document.

## What the cell is for

The grid is a visual surface: you recognize a work by its picture and pull
it out to act. Non-visual narrowing (browse, filter, search) happens before
a cell is ever scanned, so the cell is content-first — its face, plus only
what the picture can't say. Chrome earns space for judgments and
structural/status facts; anything that re-describes the capture (lens,
exposure, filename) belongs to the inspector. Carried from the prior
repo's cell round as prose, re-ratified here.

## The two things (RATIFIED)

A cell is **content + decoration**, one SwiftUI component:

- **Content** — the thumbnail slot: an AppKit leaf (`ThumbnailLeafView`)
  the collection-view item owns, laid out by SwiftUI through a
  representable but painted imperatively by the coordinator. Pixels never
  enter SwiftUI state; the grid round's atomic layer-contents swap (the
  swap-flash fix) lives on the leaf, unchanged. The leaf is never a click
  target.
- **Decoration** — everything else (`GridCell`), driven by one doorway:
  a `CellState` mutated in place across reuse. The cell asks for nothing
  and runs no query — "cells submit nothing" stands.

Metadata crosses the doorway as **canonical records** (RATIFIED
2026-09-18): `CellState` carries the `Asset` and representative `File`
rows themselves — the cell is a schema consumer like the inspector, and a
thin display-struct translation layer was struck as ceremony. The
coordinator batch-fills records riding the same resolution pass that
elects thumbnails (`Catalog.representativeRecords` — election semantics
unchanged, file-less assets now ride with a nil file so judgments show
without pixels; a nil election is SOFT, re-asked on display and stamp
events, so a file joining the asset later still heals — review fix,
2026-09-18), pushes them to live cells when resolution lands, and
keeps them live through a judgments watch (one `DatabaseRegionObservation`
over assets, re-reading visible ids — same shape as the thumbnail heal).
Position is the coordinator's id↔position table, refreshed over visible
cells after each delivery because a diff shifts positions without
reconfigure. User-customizable field display (a product requirement, per
Ari 2026-09-18) therefore becomes a pure rendering concern: the records
are always there; a future chooser only decides what the cell prints.

Seam proven by spike (2026-09-18, scratchpad, throwaway): zero body
evaluations per imperative paint, one hosting view per item ever, native
selection/keyboard intact through a hit-test-transparent hosting view.
Two spike findings worth keeping: NSHostingView captures its whole frame
by default (pass-through must be explicit), and inactive-window clicks
consult `acceptsFirstMouse`, which SwiftUI chrome answers differently
than plain views.

## Four-state (RATIFIED, hover deferred)

`idle / selected / cursor`, cursor outranking selection. Resolved in one
function, styled in one spot (the ring in `GridCell`), values in
`Theme.Grid` — a restyle or a new state touches those and nothing else,
ruled as a hard requirement. Hover is deliberately unsettled: nothing
bespoke until something needs it; it joins as a fourth case.

## Two modes, not three (carried, re-ratified)

Bare (built) and a future metadata-shown mode. The middle "compact" rung
is ruled out. The metadata mode is a documented seam in `GridCell` — a
second body, same state, same slot — and must not reopen the item or the
coordinator.

## Deliberately unsettled

- **Cues** (member-file count — kin to the Stack count badge in
  requirements; unreachable/offline): the transport now exists (records
  ride the resolution pass), but member counts and mount state aren't on
  the records yet, and the cue vocabulary itself is unratified. Own round.
- **Decoration composition**: what the row shows today (position,
  filename, rating) is a starting set for Ari's styling pass, not a
  ratified bare-cell composition — the brief's lean-bare ruling (judgments
  and structure yes, capture re-description no) still governs the eventual
  shape.
- **Interactive controls in the cell** (rating stars, metadata mode):
  ratified as a requirement; machinery deferred. The proven design is a
  registered-rects carve in the hosting view's hitTest (spike-validated,
  including a real click landing on a SwiftUI button while adjacent
  clicks stayed native). Lands with the first control, ~10 lines.
- **Kind-dependent faces** (audio glyph, document): when faces diverge,
  the kind switch lands in `GridCell`, once, compiler-exhaustive — never
  in the coordinator.
- **Eye-gate values**: ring widths/opacity, spacing, flush-vs-spaced are
  Ari's to dial in `Theme.Grid`; nothing here records a final look.
- **Live scroll measurement**: `grid.md`'s "re-evaluate with measurement
  at the cell round" is partially discharged (headless spike); the
  fast-scroll hitch check with real thumbnails remains open.
