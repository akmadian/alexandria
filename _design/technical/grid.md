# The grid: image delivery architecture

Grid round, 2026-09-12. This brief prescribes the shape of the stage's
image machinery for an implementer starting from a clean working tree. It
exists because the first build of this round derived the right *behavior*
but hand-rolled the wrong *engine*; the engine was demolished deliberately
(sunk cost cut, Ari's direction) and this document — not the demolished
code — is what future sessions inherit. Register: rulings marked RATIFIED
are Ari's; PROPOSED items await his word at build time.

## The product contract (RATIFIED)

The behavioral invariants are the only product surface; everything below
them is machinery and must trace to one of them:

1. Scrolling never hitches.
2. Anything seen this session comes back instantly.
3. Pixels land softly into fixed geometry — no reflow, no flash, never a
   blank where an image already was.
4. A missing thumbnail is a quiet placeholder; placeholders heal live as
   an import lands (the catalog announces; the renderer picks up).
5. Sharpness is owed to SETTLED viewports — "demand = settled visibility."
   Soft-but-present during motion is correct, not a compromise.
6. Selection, cursor, arrows, click: native machinery, native feel.
7. Every invariant holds while an import runs full-tilt.
8. A delivery never moves the user's viewport, selection, or cursor
   (unless the target is gone).
9. Resize reflows columns (NEVER canvas zoom); the user keeps their place.
10. Mode switches lose nothing — caches outlive renderer swaps.

## The decision: adopt Nuke core (RATIFIED direction, 2026-09-12)

The middle of this system — request coalescing, mutable priorities,
prefetching, decode-at-target-size, caching — is a converged, solved
problem (Nuke, PHCachingImageManager arrive at the same shape
independently). Hand-rolling it produced subtle failures faster than we
could patch them; the pitfalls live exactly in that middle. So the middle
is bought, not built: **Nuke core only** — its local-file fast path and
`ImageRequest.ThumbnailOptions` (target-size decode) make it fit a
local-store DAM despite its network heritage.

- NukeUI: NOT used for the grid now (today's cell is an image and a
  selection ring; hosting SwiftUI per cell buys nothing visible).
  Deliberately unsettled: re-evaluate with measurement at the cell round,
  when cells grow real content.
- NukeVideo: deliberately unsettled; the loupe round's question, where
  AVKit is the native first answer.
- If Alexandria ever grows network-backed sources, Nuke's full surface
  gets re-evaluated then. Deliberately unsettled.

## The stack, top to bottom

**NSCollectionView** (RATIFIED, re-derived): the only enforcement of
invariant 6 at 40k-item scale, plus virtualization and the prefetch
events. macOS `NSCollectionViewDiffableDataSource` is defective (no
`reconfigureItems`, known regressions) — plain data source + stdlib
`difference(from:).inferringMoves()` with a change budget; past budget,
reload.

**The coordinator** owns the bookkeeping no library provides: the one
id↔position table, same-question-diff vs new-question-replace (keyed on
the hub's answered query), viewport anchoring across updates (capture
topmost visible id + offset, restore after apply — invariant 8), the
echo-guarded selection mirror, cursor reveal. This layer is bespoke
because the ecosystem has no answer, not by preference.

**The facade — `StageImagePipeline`** (name PROPOSED): one stage-owned
type (owned by StageView, NOT the hub — the hub is question+position,
never pixels; invariant 10 is why ownership sits above the renderers).
Speaks Alexandria nouns only: cells and coordinator submit
(SubjectID, target size, urgency) and hold a cancellable handle; Nuke
types never leak past this file. If Nuke ever disappoints, the rip-out is
this file's internals.

**Nuke's pipeline inside the facade** owns loading wholesale: coalescing,
priority scheduling, prefetch, decode-at-target-size, caching. Start with
Nuke's OWN cache and scheduling defaults. Do not pre-build compensations
(see watchpoints).

## The Alexandria residue (each item cites what no library can know)

- **Identity resolution**: SubjectID → representative file. Batched ONCE
  per delivery (resolution is a property of the working set, not of
  scrolling); loads await the delivery's query rather than querying
  alone. File-less assets resolve to nil = placeholder.
- **Store layout**: `Thumbnails/<id-suffix shard>/<id>.jpg`, one stored
  1024px JPEG per file (single stored size RATIFIED; a stored pyramid is
  the ladder round's question). The facade maps file id → URL; Nuke
  decodes from that URL directly.
- **The doorbell**: `DatabaseRegionObservation` on the thumbnail-stamp
  column — one event per stamp-touching commit, no values, hopped to the
  main actor. The heal asks the bounded question (of visible placeholder
  cells, which are stamped now — stamp implies bytes, write-before-stamp
  invariant) and re-requests exactly those. Offers are one-shot per
  subject per session, so a corrupt thumbnail can't turn every ring into
  a retry; unstamped subjects stay eligible. The importer never reaches
  into the renderer.
- **Settled visibility** (RATIFIED): during scroll motion, visible cells
  request a small/content size; on a ~150–200ms quiet period of the clip
  view's bounds (covers momentum tails), the true size is requested as an
  upgrade. Trivial to express over an engine with priorities; the rule is
  the product's, the mechanism is a clamp plus a debounce.

## Engine policies (PROPOSED, to validate in the spike)

- Decode sizes on the DCT ladder of the stored JPEG: 128/256/512/1024,
  picked from actual cell size in physical pixels.
- Urgencies: content-for-visible > upgrade-on-settle > preheat. Prefetch
  requests ONLY the content size at the lowest urgency — speculation must
  be structurally unable to decode large or outrank demand.
- A visible cell paints the best cached pixels of ANY size immediately
  (progressive), and an upgrade never cancels or blanks what is shown.

## Watchpoints — failures already paid for; carry the tests, not the machinery

Each of these was observed against the demolished engine. Under the new
engine they are watchpoints and pinned tests, NOT pre-built machinery;
build a compensation only when the failure is observed again.

1. **The storm**: speculative full-size decodes thrashing a bounded cache
   (301 sharp decodes, 50-thread explosion from one folder click). Watch:
   decode counts vs cells actually settled on.
2. **Starvation**: content decodes queued behind cosmetic upgrades in a
   FIFO lane (1.5s latency-to-pixels for a visible cell). Watch:
   latency-to-pixels for cells entering the viewport.
3. **Settle-blanking**: an upgrade cancelling a nearly-finished content
   decode → blank cell at the moment of promised sharpening. Never
   discard pixels, or work about to become pixels, without a replacement.
4. **Cache thrash / eviction-as-steady-state**: watch eviction counts;
   invariant 2 is the tripwire. (The two-retention-policy split from the
   first build is the known compensation IF Nuke's single cache reproduces
   the failure.)
5. **Import contention**: import generation decodes must never delay grid
   loads (invariant 7). Import generation stays on its own bounded queue
   (thumbnails.md invariant 3: ImageIO never on the cooperative pool);
   Nuke's queues are separate by construction — verify, don't assume.
6. **Instrumentation honesty**: measure queue-wait and decode separately;
   latency-to-pixels is the UX headline. Signposts (subsystem
   `akmadian.Alexandria`) on decode, resolve-batch, apply-delivery.

## Platform gotchas already paid for

- A SwiftUI `.task` on a view that renders nothing never fires — gating
  content on state the task creates is a mount deadlock.
- swift-log's default level eats debug; bootstrap sets .debug in DEBUG
  builds. `log stream` needs `--info --debug`.
- NSCache's pressure behavior is opaque; never assume it prunes.
- The clip view posts bounds changes only when asked
  (`postsBoundsChangedNotifications`).

## The spike gate

Before the build: a scratch spike (not in the repo) proving five seams —
file-URL loading from the store layout; target-size decode buckets;
promoting an in-flight request's priority; driving Nuke's prefetcher from
`NSCollectionViewPrefetching`; cache behavior vs invariant 2 under a
grid-shaped workload. Any seam that fights back reopens the
build-vs-adopt question before code is bet on it.

## Deliberately out of this round

Grouping (the grid is one ungrouped run; "section" is not a noun here),
cell content (badges/labels/layout — cell round), manual ordering
(collections round), the stored pyramid (ladder round), delivery
throttling (unbuilt; PERF-marked in the coordinator), UI-test harness.
