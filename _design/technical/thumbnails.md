# Thumbnail System

Ratified 2026-09-11 (thumbnailing round). Records the design, the rationale, the
alternatives rejected, and the invariants. Built surface: registry thumbnailer
column, `Core/Thumbnails/`, worklist methods in `Catalog+Files`,
`ImportRun.generateThumbnails` / `generateThumbnailBatch`.

## What it is

Thumbnailing is a **phase of the one import pipeline** — never a named engine
(learnings.md ruling; the old repo promoted it to an engine with its own
governance and paid for it). Files are what get thumbnailed, not assets: an
asset's thumbnail is its representative file's thumbnail, a display-time
lookup, no machinery. One size for p0: **1024px long edge, JPEG q0.8**, stored
in the visible `Thumbnails/` directory beside the `.alxcat` package, sharded
`Thumbnails/<LAST-2-of-file-id>/<file-id>.jpg` — the id's random tail, never
its prefix: a UUIDv7's leading characters are the timestamp's top bits,
constant until 2039, so prefix sharding is one directory in a costume
(review finding, fixed 2026-09-11). A flat directory dies before the ~1M-item
scale target. Multiple sizes may come later. The store is REQUIRED equipment
(ruled 2026-09-11): the grid is the product and thumbnails are its face, so
ImportService refuses to start an import without a store — never a run that
silently skips the phase.

## Generation: registry capability, functions by domain

The format registry gains a capability column — a function value, nil meaning
"this format never thumbnails, the generic card is its face":

- `generateRawThumbnail` — raw rows. ImageIO `IfAbsent`: extracts the embedded
  JPEG preview, never decoding sensor data. Guard: a result under 512px long
  edge (tiny-preview vendors) falls back to full decode. 512 is a tunable
  default, not a researched constant.
- `generateRasterThumbnail` — plain image rows + genericImage. ImageIO
  `Always` (`IfAbsent` on a JPEG returns its 160×120 EXIF thumb — MaxPixelSize
  is a ceiling, never a floor; it will not regenerate a too-small embedded
  thumbnail).
- `generateVideoThumbnail` — video rows + genericVideo. AVAssetImageGenerator,
  keyframe near zero (default infinite tolerance = nearest-keyframe fast
  path), `appliesPreferredTrackTransform` for rotation.
- `generateQuickLookThumbnail` — vector/document/project floor. QLThumbnail-
  Generator: whatever Finder can draw, zero code per format.

Audio and sidecar rows: nil. `unrecognized`: nil for now — DELIBERATELY
UNSETTLED whether strangers should get a QuickLook try.

Functions live in `Core/Thumbnails/ThumbnailGeneration.swift` (peer of
`Core/Metadata/` — format mechanics consumed via a registry column), named by
**domain, not mechanism**: mechanism (which ImageIO flag, the size guard) is
each function's private business; the registry speaks domains. Edge cases get
a branch inside the domain function or a new function plus a row edit.

Alternatives rejected:
- **A Thumbnailer enum with strategy cases** — one type doing four jobs behind
  a shared switch; adding a strategy edits the shared type.
- **A `Thumbnailing` protocol with one struct per strategy** — ceremony buying
  nothing for stateless single-method strategies; a function value has equal
  power. (Mild idiom split with `metadataExtractor`'s protocol, accepted.)
- **Mechanism-named strategies** (EmbeddedPreview/Decode/…) — leaked
  implementation vocabulary into the registry.

ImageIO options: `ThumbnailMaxPixelSize` + `CreateThumbnailWithTransform`
(EXIF orientation — default is false; omitting it is a bug) +
`ShouldCacheImmediately: true` (decode on the worker thread, not lazily at
first render). JPEG/HEIF decode at reduced scale natively (DCT-domain), so
memory and most CPU track the 1024px output, not the source. PNG/TIFF cannot
reduced-scale decode; `kCGImageSourceSubsampleFactor` (bounds memory, not
work; source dimensions are already in the metadata column) is the named
upgrade if giant-PNG outliers ever show up in measurement. This implementation
is a starting point for optimization, not the end state.

### Measured evidence (2026-09-11, M-series/local SSD, repo TestData fixtures)

- RAF `IfAbsent`: 683×1024 in ~200ms cold, **9.3MB read of an 85MB file**
  (preview bytes only). RAF `Always`: same output, 5.5s cold, whole file read.
  This ~27x time / ~9x IO gap is what makes the NAS import contract
  (requirements: first thumbnails in seconds, ~40k RAWs) reachable at all.
  Fuji embeds full-size previews (~9MB — worst vendor); CR3/NEF typically
  embed 1–3MB.
- JPEG decode necessarily reads the whole file; no IO lever exists there.
- Encode at 1024: JPEG ~4–6ms / 133–349KB; HEIC ~28–77ms / half the bytes.
  JPEG chosen: 7–10x cheaper inside the import the user is watching.
- hev1-tagged HEVC (ffmpeg default tag) fails in both AVFoundation and
  QuickLook — Apple requires hvc1. Camera-written files are hvc1/ProRes and
  fine. The 422 fixture is the ffmpeg failure class, landing as error residue
  by design. qlmanage hung 2+ minutes on that 11KB file: decode machinery can
  wedge, not just fail (hence the timeout invariant below).

## Scheduling: the database is the queue

`files.thumbnail_at` NULL = pending; **the missing artifact IS the queue**
(schema idiom, shared with formation). Recording a file batch *is* the
enqueue — no queue objects, no in-memory handoff. One worker loop per import
(`ImportRun.generateThumbnails`), a structured `async let` child beside the record
loop:

- **Wake-up**: `AsyncStream<Void>`, `bufferingNewest(1)`. The record loop
  yields after each batch commit; conflation makes wake-ups level-triggered
  ("something changed, go look"). `finish()` on every record-loop exit path —
  a missed finish deadlocks the bracket. The stream carries the *signal*,
  never the work: the worklist query is the single source of truth, because
  retry/recovery must survive the process and an in-memory queue's contents
  don't. Handing rows through the stream would mint a second source of truth
  that disagrees with the first.
- **Worklist query** (`Catalog+Files` — table-oriented convention; there is no
  thumbnails table): files in this import where `thumbnail_at IS NULL`, kind
  has a thumbnailer, `missing = 0`, and no `file_errors` row for task
  'thumbnail'. `ORDER BY id` (UUIDv7 = record order) — import-ordered, the
  ratified p0 ordering. No visible-first queue-jumping. Served by the partial
  index `idx_files_thumbnail_pending (import_id, id) WHERE thumbnail_at IS
  NULL` (ratified 2026-09-11 after review): pending rows only, pre-ordered,
  empties as stamps land — each pull O(log n + batch) instead of re-walking
  the whole import per 8-file pull.
- **Batch = width**: pull 8, run all 8 concurrently in a TaskGroup, await all,
  stamp all in **one transaction** — which is what makes the grid un-shimmer
  in waves (batches + shimmer, ratified UI behavior). Known ceiling: the batch
  waits for its slowest member; the upgrade is a sliding window if measurement
  ever cares. Width is one constant in one place; volume-aware policy
  (ObservedVolume knows residence) is the named refinement.
- **Not all-N parallel**: throughput caps early (1GbE caps Fuji previews at
  ~12 files/s; decode caps near core count) — past saturation, parallelism
  buys only memory spikes and SMB thundering herd.
- **No unstructured Tasks**: a free `Task {}` per batch/file gives unbounded
  pileup (recording outpaces thumbnailing), orphaned cancellation, vanishing
  errors, and hand-rolled completion bookkeeping. Structured child + stream =
  backpressure, cancellation, one error path, free completion.

## Import-critical, non-blocking — two different edges

- **Pacing**: thumbnailing never gates the record loop. Recording sprints,
  thumbs trickle behind (requirements: grid populates in seconds, no import
  wall).
- **Completion**: the bracket awaits the worker before
  `recordImportFinished(.completed)` — a thumbnail is part of ready-to-use.
- **Failure split** (mirrors formation): a per-file failure is `file_errors`
  residue and a generic card, never an import failure. The worker itself dying
  (catalog write error, store unwritable) fails the import.

## Failure handling: file_errors is the DLQ

One attempt per file per import: first failure writes
`file_errors(task: 'thumbnail', …)` and the worklist exclusion keeps it
excluded. **No retry policy for p0** — no attempts counting, no
retryable/terminal distinction in the query. A future rescan clearing error
rows is the natural retry lever. (Without the exclusion the drain loop would
re-hammer failures on every nudge.)

## Critical invariants

1. **Idempotent passes**: re-running the pass is always safe; the worklist
   query defines the population. Crash/cancel recovery is "run it again."
2. **Per-file timeout, abandon not cancel**: every generation races a
   deadline (~30s) → `timed_out` residue. ImageIO is synchronous and
   uninterruptible; the timed-out thread runs on, the pass moves on. Without
   this, one wedged decode = an import that never completes.
3. **No synchronous decodes on the cooperative pool**: ImageIO calls bridge
   through a dispatch queue + continuation. Blocked cooperative-pool threads
   starve every async task in the app — the mystery-freeze the requirements
   forbid. (QuickLook/AVFoundation paths are natively callback-async.)
4. **Store write before stamp, atomically**: thumb file written (atomic
   write), then `thumbnail_at` stamped. A crash between leaves an orphan file
   that the retry overwrites — never a stamp without bytes.
5. **Stamp granularity = batch transaction**: the UI's shimmer waves are the
   write granularity, not separate machinery.
6. **Zero format names in the pass**: the pass is generic code reading
   registry rows. A format conditional in pipeline code is the registry
   split-brain defect reappearing.

## Deferred / unsettled

- ~~Killed imports leave un-thumbed files until a future feature~~ SUPERSEDED
  by the resume ruling (2026-09-11, same day): a run is identified by its
  source. ImportService resolves source → root folder → the unfinished job
  (latest import whose outcome isn't 'completed'; interrupted/canceled/failed
  all count) and the new run adopts that id — bracket reopened, walk re-runs
  with skip-existing, and every import-scoped worklist (formation AND
  thumbnails) completes under the original identity. The worklist stays
  import-scoped — an import's thumbnailing never widens to arbitrary pending
  rows; a global "generate missing thumbnails" repair remains a future
  feature. Known edge, accepted: two simultaneous runs over the same root
  would adopt the same id (NULL outcome can't distinguish running from
  interrupted); guard when the import UI grows.
- Shimmer vs "thumbnail failed" card for nil-capability and errored files:
  future decision; either way never-shimmer-forever (the state is derivable:
  nil capability or error row = not pending).
- Store economics at the ~1M ceiling (~hundreds of GB at current settings) —
  1M is a crazy-high upper ceiling; pruning features come later. The store is
  regenerable by design.
- Rescan invalidation: `modified_at` change nulls `thumbnail_at` and clears
  error rows — same idiom, no second mechanism, when rescan lands.
- Shared read window across hash/metadata/thumbnail (one open per file on
  NAS) — real future IO optimization, not p0.
- Multi-size ladder, per-vendor tuning, HEIC, quality knob, sliding-window
  drain, volume-aware width: all named upgrade lanes, none built.
