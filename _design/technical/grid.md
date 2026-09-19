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
5. Visible images are sharp and scrolling never hitches. (This is a UX
   promise. It earlier prescribed a mechanism — "small during motion, upgrade
   on settle" — which was struck 2026-09-12: no battle-tested grid does it
   and it caused a whole-viewport flash. Scroll cost is carried by request
   priority + prefetch, not by resolution; sizing is one tier per cell.)
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

**The layout** (RATIFIED 2026-09-19): a custom `NSCollectionViewLayout`
(`GridLayout`) over pure geometry (`GridGeometry`), with the column
count as the ONE input — cells absorb the width, gaps are exactly the
spacing, edges land on the device-pixel grid. Flow layout was removed,
not patched: its model is the inversion (item size in, count out), and
its bounds-change invalidation never re-asks the size delegate, so a
resize reflowed stale sizes (the stretching-gaps bug). Invariant 9's
"the user keeps their place" is the layout's job: on any geometry
change (width, columns, backing scale) it mints the scroll origin that
holds the topmost visible ITEM — a point offset dies with the old row
heights — and the collection view applies it BEFORE the pass builds
cells, so the pass materializes the anchored viewport, never one jump
behind it (at depth the jump exceeds the whole viewport). This
index-keyed reflow anchor is deliberately separate from the
coordinator's id-keyed delivery anchor: a reflow moves no items, a
delivery moves ids.

**The coordinator** owns the bookkeeping no library provides AND drives
image loading (it is effects + tables; the pure decisions live in
`GridImaging`, the Nuke calls in `StageImaging`): the one id↔position
table, same-question-diff vs new-question-replace (keyed on the hub's
answered query), viewport anchoring across DELIVERIES (capture topmost
visible id + offset, restore after apply — invariant 8; geometry-change
anchoring lives in the layout, above), the echo-guarded
selection mirror, cursor reveal, plus the load lifecycle — resolution,
per-cell request on display, cancel on exit, prefetch, the stamp-watch
heal, and the atomic paint routed through the id↔position table. This
layer is bespoke because the ecosystem has no answer, not by preference.

**`StageImaging`** (RATIFIED 2026-09-12, built): the stage-owned owner of
the Nuke pipeline + prefetcher + memory cache, and the ONE place a Nuke
request is built (so every caller keys the cache identically). Owned by
StageView, NOT the hub — the hub is question+position, never pixels;
invariant 10 (cache outlives a renderer swap) is why ownership sits above
the renderers. It is NOT a facade: the coordinator sees Nuke types
directly. There is no rip-out-behind-an-interface goal — that was
speculation struck this round; the boundary exists for ownership and
consistent request construction, not concealment. Cells submit NOTHING:
they are pure display sinks (`show`/`showPlaceholder`), and loading is the
coordinator's job entirely.

**Nuke, owned by `StageImaging`,** owns loading wholesale: coalescing,
priority scheduling, prefetch, decode-at-target-size, caching. Start with
Nuke's OWN cache and scheduling defaults. Do not pre-build compensations
(see watchpoints).

## The Alexandria residue (each item cites what no library can know)

- **Identity resolution**: SubjectID → representative file. Batched in
  visible/prefetch windows and cached for the session (RATIFIED 2026-09-12,
  revised from "once per delivery": at the 1M scale target an eager whole-set
  read is the hitch we're avoiding — resolution is windowed, keyed to the
  delivered working set so results stay consistent). File-less assets resolve
  to nil = placeholder.
- **Store layout**: `Thumbnails/<id-suffix shard>/<id>.jpg`, one stored
  1024px JPEG per file (single stored size RATIFIED; a stored pyramid is
  the ladder round's question). `ThumbnailStore` maps file id → URL and owns
  the decode ladder (`maxPixelSize >> k`); `StageImaging` hands the URL to
  Nuke, which decodes at the target bucket.
- **The stamp watch**: `DatabaseRegionObservation` on the thumbnail-stamp
  column — one event per stamp-touching commit, no values, hopped to the
  main actor. The heal asks the bounded question (of visible placeholder
  cells, which are stamped now — stamp implies bytes, write-before-stamp
  invariant) and re-requests exactly those. Offers are one-shot per
  subject per session, so a corrupt thumbnail can't turn every ring into
  a retry; unstamped subjects stay eligible. The importer never reaches
  into the renderer.
- **Sizing** (RATIFIED 2026-09-12, replacing the struck "settled visibility"
  mechanism): ONE tier per cell — the smallest rung on the store's DCT ladder
  covering the cell in physical pixels (cell points × backing scale), clamped
  to the stored ceiling. Decoded once, cached, reused; re-requested only on a
  cell-size change (zoom/resize) or heal, never on scroll. This is the
  cross-system pattern (Apple Photos, Nuke, Kingfisher, SDWebImage, Lightroom,
  Capture One). The ladder is a store fact (`ThumbnailStore.decodeLadder`,
  `maxPixelSize >> k`): each rung is a clean inverse-DCT scale of the stored
  JPEG, no resampling. Scroll cost lives in priority + prefetch (below), not
  in resolution.

## Engine policies (RATIFIED 2026-09-12; built)

- Decode sizes on the DCT ladder of the stored JPEG: 128/256/512/1024,
  picked from actual cell size in physical pixels. One size per cell.
- Two urgencies: content-for-visible (high) > preheat (lowest). Prefetch
  requests the SAME bucket a cell will request, at the lowest urgency — so a
  prefetched image is a real cache hit and speculation cannot outrank demand.
- A visible cell paints the best cached pixels of ANY size immediately
  (progressive), then the one target decode swaps in atomically; a paint
  never downgrades what is shown.

## Watchpoints — failures already paid for; carry the tests, not the machinery

Each of these was observed against the demolished engine. Under the new
engine they are watchpoints and pinned tests, NOT pre-built machinery;
build a compensation only when the failure is observed again.

1. **The storm**: speculative full-size decodes thrashing a bounded cache
   (301 sharp decodes, 50-thread explosion from one folder click). Watch:
   decode counts vs cells actually settled on.
2. **Starvation**: content decodes queued behind lower-value work
   (1.5s latency-to-pixels for a visible cell). Priority carries this now
   (content high, preheat lowest). Watch: latency-to-pixels for cells
   entering the viewport.
3. **Swap-flash / ground exposure**: replacing a cell's image must never
   reveal the placeholder ground between old and new pixels. Fixed
   structurally 2026-09-12 — the cell is a CALayer whose `contents` swap
   inside an actions-disabled CATransaction, so there is no drawRect erase
   and no implicit fade. (Supersedes the old "settle-blanking" watchpoint:
   with one size per cell there is no upgrade to cancel content.) Watch:
   any blank frame on scroll-stop, zoom, or scope change.
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
