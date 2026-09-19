# Drag and drop: round record

**Status: RATIFIED** (drag round, 2026-09-18; built, review-fixed, and
suite-green the same day). This is the round's decision record — rulings,
alternatives considered and why they lost, the spike evidence the
mechanism choices rest on, and the seams future rounds inherit. Register:
every numbered ruling is Ari's; spike findings are measured evidence from
this machine, dated; anything marked DERIVED is reasoning, not ratification.

Provenance: the collections round's chunk 4 (2026-09-14) struck its first
drag build — SwiftUI `dropDestination` inside a `List` atop an undeclared
runtime UTType — and mandated this round, "opening on the prior-art
question before any code." The spike below answered that question with
evidence instead of forum hearsay, and rewrote the post-mortem: the List
was largely innocent; the undeclared type was the killer.

## What this is

Intra-app drag and drop, three gestures over one contract:

- **Grid cells → sidebar collection row**: add the dragged assets to the
  collection (`addMembers`).
- **Sidebar collection row → another row / the section header**:
  re-parent the collection (`moveCollection`); the header means "to root."
- **Grid cells → a gap between grid cells**: reorder the viewed
  collection's manual order (`reorderMembers`), or — from another sort —
  adopt the on-screen order as the manual order behind an explicit
  confirmation (`setManualOrder`).

No drag round writes a new membership concept: every drop lands on a verb
the collections round already ratified, plus one new verb (adoption) this
round earned.

## Rulings, with alternatives considered

1. **Intra-app only.** No drag-out to Finder ("definitely not" — Ari).
   AppKit's non-local dragging mask defaults to `.none`, so the ruling
   costs zero code; the payload seam holds a slot for a file-URL
   representation if a drag-out round ever opens. DELIBERATELY UNSETTLED:
   drag-out (which file of an asset, unmounted-volume behavior), and
   drag-INTO the app (import-by-drop) — each its own round.

2. **The contract is construction-agnostic.** The sidebar's construction
   (SwiftUI List today; maybe OutlineGroup, maybe NSOutlineView) is not
   settled, and the drag design must survive the swap. Hence the layer
   split below: payload, rules, and verbs know no view types; only thin
   per-surface adapters touch construction. Migration cost measured in
   the design: OutlineGroup keeps the adapters unchanged (shims attach
   per-row regardless of how rows are generated); NSOutlineView deletes
   the shims and calls the same three seams from ~4 delegate methods.

3. **The whole cell is the drag handle.** LrC makes you grab the
   thumbnail's center, not its border — a documented users-trip-on-this
   misfeature ([Adobe community](https://community.adobe.com/t5/lightroom-classic-discussions/lightroom-not-allowing-me-to-drag-photos-into-a-collection/td-p/11746570)).
   Native NSCollectionView behavior plus the cell round's hit-test
   transparency already gave this; the ruling pins it as a contract.
   Corollary, same pain family: a drag is ALWAYS allowed to start —
   refusal lives at destinations, visibly, never at the source (LrC's
   "drag just doesn't work" genre).

4. **The payload is assets** — one private pasteboard type carrying asset
   ids, mapped once at drag start; a files-lens cell drags as its OWNING
   asset, so destinations never learn about lenses. This mirrors the
   ratified membership unit (collections ruling 10: the unit is assets)
   and both standing verbs' signatures. Two file cells of one asset are
   ONE dragged asset (first occurrence keeps its place — the
   `reorderMembers` rule). A second type carries a dragged collection's
   id. One pasteboard item per dragged cell (AppKit's multi-item imagery
   contract) with an ordinal, because AppKit does not document multi-item
   pasteboard order. Rejected: lens-faithful SubjectID payloads (every
   destination re-implements the file→asset mapping); Transferable/
   NSItemProvider payloads (see ruling 6's evidence).

5. **Cells that can't name an asset refuse item-level.** The verified
   real case: a formation-pending file (asset_id NULL) in an UNFILTERED
   files view — e.g. Latest Import mid-import — plus the
   milliseconds-wide unresolved-record window. The rest of a selection
   still drags.

6. **The sidebar mechanism is an AppKit shim** — one NSView overlaid per
   collection row, drop target and drag source both — because the spike
   proved SwiftUI's bridging drops undeclared custom types SILENTLY in
   both directions (findings F1/F1b below), while plain
   `NSPasteboard.PasteboardType` strings need no declaration anywhere.
   No UTType and no Info.plist entry exists in this design. Rejected:
   SwiftUI `onDrop`/`dropDestination` (worked for system types, silent
   for ours); declaring the UTType to rescue the SwiftUI path (adds
   project machinery to reach parity with what the shim does without
   it); an NSOutlineView sidebar rebuild just for drag (ruling 2 makes
   it unnecessary — the adapter split means we don't have to decide the
   sidebar's future to ship drag).

7. **One decision table rules the combinatorics**
   (`DragRules.verdict(over:context:)`): payload × semantic target ×
   session context → verdict. Invalid combinations degrade by the target
   NEVER lighting up — no landing no-ops — including drops that would
   add zero new members (counted from a drag-start membership snapshot;
   the count badge shows what a drop would actually add). The snapshot
   is ADVISORY and the verbs stay AUTHORITATIVE: every accept re-checks
   inside its transaction (the cycle fence lives in `moveCollection`,
   not in hover code). Growth is additive, not multiplicative: type
   registration prunes most combos before the table sees them, unlisted
   cells refuse by construction (safe-by-default), and the payload-major
   exhaustive switch makes a new payload a compiler-forced branch.
   Prior-art shape: NetNewsWire's sidebar decomposes validation the same
   dual-axis way; UIKit's UIDragSession is this context object with a
   platform badge.

8. **Grid reorder is gap-only** — the vertical insertion line is the one
   grid drop affordance; dropping a photo ON a photo never means
   anything. AppKit's `.on` proposals (cursor over a cell's middle)
   retarget to the NEARER gap by cursor position, so the whole surface
   maps to insertions and no cell half silently means "insert to my
   left." Rejected: refusing `.on` outright (leaves sliver-thin targets);
   accepting `.on` as-proposed (gives drop-on a meaning, the excluded
   thing).

9. **A judgment is never silently overwritten — ever.** Reorder-dragging
   while sorted by anything other than manual raises an EXPLICIT
   confirmation ("Switch to Manual Order? The order currently shown,
   with your change, becomes this collection's manual order."); only the
   dialog's yes writes, via `setManualOrder` (whole-order re-mint, one
   transaction, exact-member-cover fence), then the arrangement intent —
   verb before switch, so the grid settles once. While already IN manual,
   reorder is live with no ceremony (that's editing your order). The
   cursor-attached hint says what a drop will do before it does it.
   History, recorded so it can't be re-litigated blind: the round first
   proposed a no-dialog fast path for never-hand-ordered collections via
   a derived pristine check — **that check is unimplementable**:
   membership rows carry no timestamp, append-minted keys are the ONLY
   record of add order, and a reorder-to-end mints integer keys
   indistinguishable from appends, so "never hand-ordered" has no
   independent witness. DELIBERATELY UNSETTLED: a stored
   `has_authored_order` bit could restore the no-dialog pristine path;
   it is a schema change and must earn its own ruling.

10. **No reorder in union views, and refusals are visible.** The union
    gate is display-faithful — reorder refuses when any descendant
    contributes members to the displayed union (collections ruling 6's
    "the drag has no honest meaning" case) — and it outranks manual:
    a manual union view refuses too (round review, finding 1). Filtered
    adoption also refuses (the displayed set wouldn't cover the members,
    and the adoption verb's fence would reject it); live-manual reorder
    under a filter stays allowed — anchor-relative placement is
    well-defined over a partial view. Both refusals ride the visible
    channel (ruled: "as annoying as we need to... never silence"):
    a message attached to the drag image, never a dead gap with no
    explanation (LrC's ["custom sort unavailable" mystery](https://asktimgrey.com/2017/09/12/custom-sort-unavailable/)
    is the named anti-pattern). Structural refusals that ARE
    self-explanatory (own subtree, current parent, zero-new-adds) stay
    dark by ruling 7.

## The architecture

```
Alexandria/Drag/
  DragPayload.swift    the two pasteboard types; per-item write, ordered+
                       deduped read — the seam every surface shares
  DragRules.swift      DropTarget (semantic), DropVerdict, DragContext
                       (the one-live-drag session snapshot + the drag-image
                       label channel), the decision table, ReorderPlan
                       (the pure drop-geometry→order math, test-pinned),
                       DragVerbs (verdict→verb, the one door)
  SidebarRowShim.swift the sidebar adapter ONLY: row shim (target+source,
                       threshold drag, spring, highlight callbacks),
                       header shim (root target, hit-test transparent),
                       AutoscrollDriver (timer-driven, owned per adapter)
```

Grid adapters live on the existing `GridRepresentable.Coordinator`
(source + reorder destination + label forwarding via `GridCollectionView`
enter/updated/exited overrides). `GridView` owns the adoption
confirmation. `DragContext` is a per-drag global set by the source and
cleared at session end — honest modeling, not a shortcut: macOS runs one
drag at a time and the drag pasteboard is already exactly this kind of
global. Its `didEnd` notification is the authoritative end-of-drag reset
for drag-scoped UI state (a shim recycled mid-drag can't clear a
highlight it inherited).

Catalog additions (Catalog+Collections.swift): `setManualOrder`
(adoption; refuses anything but an exact member cover rather than
half-scrambling a judgment-class ordering; vacate-then-remint so the
UNIQUE order fence is never transiently violated), `membershipCounts`
(chunked — a select-all drag would otherwise exceed SQLite's
bound-parameter ceiling and silently leave every verdict optimistic),
`descendantsContributeMembers` (the union gate's read),
`collectionSubtreeIds` (the cycle refusal's hover mirror).

## The spike record (2026-09-18, this machine — the evidence base)

- **F1** SwiftUI drop routing silently ignores undeclared custom UTTypes;
  with a SYSTEM type, `onDrop`, an AppKit shim, and even `dropDestination`
  all received drops in a sidebar List. The chunk-4 strike's true killer
  was the type, not (only) the List. **F1b** the reverse bridge (SwiftUI
  `.onDrag` source → AppKit reader): the type identifier crosses, the
  promised data arrives EMPTY — hence the threshold-dance AppKit source
  for sidebar rows too.
- **F2** Drag hover callbacks are movement-driven: content scrolling out
  from under a stationary cursor generates no callback, so autoscroll
  must be TIMER-driven (poll the mouse while the button is down —
  NSOutlineView's own internal shape), scheduled in `.common` mode.
- **F3** Rows created mid-drag receive the in-flight drag — spring-loaded
  disclosure expansion is safe, drops land on rows born after mouse-down.
- **F4** A shim's drop zone is its backing view's frame — row content
  must be full-width or targets are label-sized ("hit just the right
  spot").
- **F5** Native for free: `.stack` gather formation (the dragged
  thumbnails pile under the cursor), the multi-item count badge,
  `numberOfValidItemsForDrop` updating it per target, the inter-item
  insertion line, per-cell drag imagery, and mid-drag drag-image
  mutation. The label channel APPENDS to captured original components
  and restores them on exit — replacement loses the dragged item's image.
- **F6** A background shim coexists with row click-selection.
- **F7** The threshold source (swallow mouse-down, 4pt threshold starts a
  real session, clean click selects via callback) delivers payloads
  intact and preserves selection feel.

## Logging

`Logger(label: "drag")`: begin (payload kind, count, reorder facts) and
end at debug/trace; every verb success at info with ids and counts; verb
refusals audible (beep) + logged; label attachment logged once per attach
(the message otherwise reaches only the drag image); the membership
snapshot's failure names its user-visible consequence (verdicts stay
optimistic).

## PERF ledger

- `// PERF:` at the grid's pasteboard writer: a select-all drag mints one
  pasteboard item + dragging item per cell synchronously (~40k at the
  library target = a visible mouse-down hang). Trigger: select-all drags
  measurably stall. Upgrade: one list-carrying item plus image-only
  dragging items.
- `membershipCounts` chunking is built (500/statement), not deferred —
  the failure mode was silent verdict degradation, not slowness.

## Deliberately unsettled / carried

- Drag-out to Finder and drag-in import (ruling 1's markers).
- The `has_authored_order` bit and the no-dialog pristine adoption path
  (ruling 9's marker; schema, own round).
- Sidebar tree manual ordering (collections ruling 11 — the sidebar has
  no between-rows insertion until then; drop-ON only).
- Smart collections refuse manual adds when the predicate column lands —
  one more refuse row in the table, no reshaping.
- Folder / Library / import rows are NEVER drop targets this round;
  drag-to-folder would mean moving files on disk — its own round if ever.
- Files lens over collections stays empty (collections ruling 10);
  files-lens drags add via the owning asset and never reorder.
- Drag-image styling: first pass is the cell snapshot / titled pill;
  Ari's refinement note lives in ROADMAP (image-only, smaller).
