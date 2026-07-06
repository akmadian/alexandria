# Deferred Features

This document tracks features that were explicitly discussed and deliberately deferred from v1. Each entry includes the rationale for deferral and notes on what would be required to implement it. The schema and architecture accommodate these features — they are not blocked by existing decisions.

---

## P1 — Deferred from v1, planned for early follow-up

### Asset grouping

**What it is:** Related assets — a RAW file and its exported JPEG, a PSD and its exported PNG, multiple crops of the same shot — are presented as a single card in the grid. Expanding the card reveals the group members. Each member has a role (raw, jpeg_sidecar, source, export, member).

**Primary use case:** Photographers shooting RAW+JPEG simultaneously (common for adventure/event work where fast-shareable JPEGs are needed alongside archival RAWs). Files sharing the same base filename in the same directory are automatically grouped at ingest: `IMG_1234.CR3` + `IMG_1234.JPG` + `IMG_1234.xmp` become one group. The RAW is the archival master. XMP sidecars (both `.xmp` and `.CR3.xmp` naming conventions) are always attached to their RAW and never shown as standalone assets. Eagle does not do this.

**Preview priority chain:** The grid thumbnail should reflect the most-edited version available on disk. Alexandria can only work with what is present on disk — it does not run a RAW developer. Priority order:
1. **LrC-exported JPEG** in the same directory with matching base filename — reflects the actual LrC edit, highest fidelity
2. **Camera-original companion JPEG** (RAW+JPEG shoot mode) — camera-processed, fast to decode
3. **DNG with updated embedded preview** — LrC writes an updated embedded preview back into DNG files on metadata save; extract via exiftool
4. **Embedded RAW preview** (all other formats) — camera-original, no LrC edits reflected; this is the ceiling for non-DNG RAWs unless a JPEG is exported

When XMP sync detects that a RAW's sidecar has changed, Alexandria should re-evaluate whether a fresher preview source has appeared on disk and regenerate the thumbnail if so.

**Why deferred:** The grouping logic (detecting which files should be grouped automatically) is non-trivial. Heuristics based on filename similarity and partial hash comparison are needed. The UX for managing groups (creating, breaking apart, reassigning roles) needs careful design. The schema is ready (`asset_groups`, `asset_group_members`).

**What's needed to implement:**
1. Grouping heuristics: filename-based (strip extension, compare base names; same base name with different extensions = potential group), hash-based (partial hash match with different MIME types). Must handle both `.xmp` and `.<raw_ext>.xmp` sidecar naming conventions.
2. UI: group card in grid (stacked card appearance), group detail view showing all members with their roles
3. Commands: `GroupAssetsCommand`, `UngroupAssetsCommand`, `SetGroupCoverCommand`
4. Integration with ingest pipeline: detect grouping candidates at ingest time or as a post-ingest pass

**Schema impact:** None — `asset_groups` and `asset_group_members` tables are present from migration 0001.

---

### Smart collections

**What it is:** A collection whose membership is computed dynamically from a stored query. Always live — membership updates as the catalog changes.

**Design goal: significantly more powerful than Lightroom Classic.** LrC's smart collections only support a flat list of criteria with a single "match ALL" or "match ANY" switch. Alexandria should support **fully nested boolean conditionals** — an expression tree of AND/OR/NOT groups, each containing criteria or further nested groups. This enables queries like:

```
(rating >= 4 AND file_type = RAW)
AND (tag = "climbing" OR tag = "mountains")
AND NOT (in_collection = "Published")
```

Criteria should support string and numeric comparisons against any metadata field — not just equality but `>=`, `<=`, `contains`, `starts with`, `is empty`, `is not empty`, date ranges, etc.

**Built-in system smart collections** (always present, removable by user, re-addable from a system list): Untagged, Unrated, Not in any collection, Import errors.

**Why deferred:** The query builder UI for nested conditionals is non-trivial — needs a recursive condition group editor. The JSON query schema must be designed carefully to support arbitrary nesting without becoming unmaintainable.

**What's needed to implement:**
1. Query definition schema (JSON): recursive structure supporting `{ "and": [...] }`, `{ "or": [...] }`, `{ "not": {...} }`, and leaf criteria nodes `{ "field": "rating", "op": "gte", "value": 4 }`
2. Query compiler: translates the expression tree to parameterised SQL, handling all operators and field types
3. UI: recursive condition group editor — add criterion, add group, nest groups, toggle AND/OR/NOT per group
4. `CollectionRepository.GetAssets()` already accepts `AssetFilter` — the compiler populates it from the JSON
5. System smart collections: seeded at catalog creation, removable but re-addable

**Schema impact:** None — `collections.query` column is present from migration 0001. The JSON schema just becomes richer.

---

## P2 — Post-v1, requires research

### Perceptual hash / similar image detection

**What it is:** A perceptual hash (phash) stored per image asset at ingest. Enables fast duplicate and near-duplicate detection — useful for surfacing burst shot clusters for culling. Cheaper and earlier than full CLIP-based AI search.

**Why deferred:** Not core to v1 browse/search/organise. Adds ingest cost per asset.

**What's needed to implement:**
1. Compute phash at ingest (fast, no subprocess — pure Go implementation available)
2. Store as a column in the `assets` table
3. Similarity query: find assets within a hamming distance threshold
4. UI: "similar images" cluster view in the inspector or a dedicated dedupe screen

**Schema impact:** New `phash` column on `assets` (cheap migration).

---

### Dominant color / palette extraction

**What it is:** Extract 5–8 dominant colors per asset at ingest via k-means or median cut over a downsampled thumbnail. Store as hex values. Enables filter/search by color and sort by dominant color. For video, sample a few keyframes and union the palettes.

**Why deferred:** Not blocking v1. Adds minor ingest cost.

**What's needed to implement:**
1. Color extraction at ingest, run against the thumbnail (already generated) — no extra file read
2. `asset_colors` table or JSON column on `assets`
3. Color picker in the filter bar; proximity search uses deltaE in LAB colorspace (not naive RGB — human color perception is not linear in RGB)

**Schema impact:** New `dominant_colors` JSON column or `asset_colors` table.

---

### AI/ML tagging and semantic search

**What it is:** Two related capabilities sharing the same CLIP infrastructure:
- **Auto-tagging:** suggest tags based on image content (scene classification, object recognition)
- **Semantic search:** natural language search — "sunset over water", "person on a rock face" — returning visually relevant results

**How it works:** CLIP encodes images and text into the same vector embedding space. At ingest, each image gets an embedding vector stored via the `sqlite-vec` extension (SQLite-native vector similarity — no separate vector DB). At search time, the query text is embedded and nearest-neighbor search merges with existing FTS5 text results.

**Video:** uses frame sampling via ffmpeg (1 frame per 5–10 seconds is a reasonable default). Scene-level tags work well. No temporal understanding. Video AI tagging should be independently opt-in from photo tagging given higher compute cost.

**Why deferred:** Requires either on-device ML inference (large model files, ~600MB for CLIP vision encoder) or an external ML service. On-device is the only acceptable approach — user files must never leave the machine without explicit opt-in. Model downloaded on first opt-in, not bundled. Apple Silicon handles inference well; older Intel hardware is slower and may need sparser sampling.

**What's needed to implement:**
- Decision: ONNX runtime vs Core ML (Mac) vs clip.cpp (cross-platform)
- `sqlite-vec` extension for vector storage and ANN search
- Tag source "ai" added to `asset_tags.source` CHECK constraint
- ML pipeline stage at ingest or as a separate background pass, independently configurable for photos vs video
- Face regions: `face_regions` and `persons` tables (schema left as an example in the migrations doc)
- UI: confidence scores, correction workflow, opt-in settings, model download/management

**Schema impact:** New `asset_embeddings` table (vec column via sqlite-vec). `asset_tags.source` CHECK constraint needs "ai" value. New tables needed for face detection.

---

### Duplicate resolution UI

**What it is:** Duplicates (same content at two paths) are already detected at import, ingested as normal assets, and logged persistently to the `duplicates` table (see schema doc) — nothing is dropped or lost between sessions. What's deferred is the resolution UI: side-by-side comparison where the user decides keep both, remove one, or link as a group.

**Why deferred:** The review screen is a distinct piece of interaction design. Because detections are persisted, deferring the UI costs nothing — pending rows wait in the table.

**What's needed to implement:**
1. UI: duplicate review screen showing both assets side-by-side with metadata comparison, driven by `duplicates` rows with `status = 'pending'`
2. Commands: `ResolveDuplicateCommand` (options: keep_both, remove_one, link_as_group), setting `status = 'resolved'`/`'ignored'`

---

### Integrity check service

**What it is:** A periodic or on-demand check that verifies source files still exist at their expected locations, and that their content (hash) matches what was recorded at ingest. Surfaces missing, moved, and changed files.

**Why deferred:** The UX for presenting integrity check results is non-trivial — you need to show the user what changed and what they should do about it. The background check implementation is straightforward.

**What's needed to implement:**
1. `IntegrityChecker` service: walks each online source, compares mtime/size (fast) or partial hash (slow, optional) against catalog records
2. Results surfaced as a report: N assets verified, N missing, N changed, N moved (detected via hash match at different path)
3. UI: integrity report screen with actions (re-link moved files, remove missing from catalog, re-ingest changed)
4. Scheduled runs: configurable background integrity check (nightly, weekly)

---

### GPX track correlation

**What it is:** Import a GPX file and Alexandria correlates it with photos and video clips by timestamp, writing GPS coordinates to assets that lack them. Essential for outdoor/adventure photographers whose camera bodies don't have GPS but whose watch or action camera was recording a track the entire time.

**Core feature:** GPX file import — works with any GPS device (Garmin, Suunto, Wahoo, phone exports). The user drops a `.gpx` file onto a source or collection, Alexandria matches assets by timestamp within a configurable tolerance window, and writes coordinates via exiftool.

**Optional extension:** Garmin Connect API integration — auto-pull recent tracks when the user connects a source, avoiding the manual GPX export step. Opt-in only, requires OAuth. Clearly secondary to plain GPX import; should not block the core feature.

**Why deferred:** Core GPX correlation is P1. Garmin API is P2.

**What's needed to implement:**
1. GPX parser (standard XML format, well-supported in Go)
2. Timestamp matching: correlate asset capture time to nearest GPX trackpoint within tolerance (configurable, default ±30s). Handle timezone offset between camera clock and GPS device.
3. Exiftool write path for GPS fields (GPSLatitude, GPSLongitude, GPSAltitude, GPSTrack)
4. UI: import GPX dialog, preview of how many assets would be geotagged, confirm
5. (P2) Garmin Connect OAuth flow, track sync settings

---

### Smart bracket / burst / panorama detection

**What it is:** Auto-detect and group HDR bracket sets, focus stacks, panorama sequences, and burst sequences based on EXIF metadata and timestamp patterns. Cameras write these in predictable ways:
- **Bursts:** sub-second timestamp intervals between frames
- **HDR brackets:** sequential exposure compensation values (EXIF `ExposureBiasValue`) at near-identical timestamps
- **Focus stacks:** sequential focus distance changes at identical exposure settings
- **Panoramas:** overlapping GPS headings or identical focal length sequences with timestamp proximity

Groups these automatically as an extension of the asset grouping system.

**Why deferred:** Depends on asset grouping (P1). Detection heuristics need careful tuning to avoid false positives.

**What's needed to implement:**
1. Post-ingest detection pass examining EXIF patterns across adjacent assets (by timestamp and source)
2. Confidence scoring per detection type — only auto-group above a threshold, flag uncertain cases for user review
3. Group roles for bracket members (base, +1EV, -1EV, etc.)
4. UI: review detected groups before confirming, with option to always-auto-group or always-ask per type

---

### Technical quality scoring

**What it is:** At ingest, compute a sharpness/focus score per image using Laplacian variance — no AI, pure signal processing, fast. Store as a column. Enables filtering "show only sharp photos" and ranking within burst groups to surface the sharpest frame. Combined with burst detection, makes culling significantly faster.

**Why deferred:** Experimental — quality of the signal varies by subject matter (intentional blur, low contrast subjects). Should be surfaced as a filter/sort option, not as an automatic hide/show decision.

**What's needed to implement:**
1. Laplacian variance computation at ingest (operates on thumbnail, not source file — fast)
2. `sharpness_score` column on `assets`
3. Sort by sharpness, filter above/below threshold in the grid
4. Within burst groups: surface the highest-scoring frame as the default selection

---

### Timestamped video clip annotations

**What it is:** Leave notes on a video clip tied to a timecode without opening an editor — "good b-roll at 0:23", "bad focus at 0:45", "use this line". Makes the find-the-good-bits step faster before taking clips into DaVinci Resolve.

**Interop situation:** No universal cross-app standard exists. Options:
- `xmpDM:markers` (XMP Dynamic Media namespace) — used by Premiere Pro, stored in XMP sidecar. Resolve ignores it.
- FCPXML markers — Final Cut Pro only.
- Resolve markers — stored inside the `.drp` project file, not accessible via sidecar.

**Approach:** Store annotations in Alexandria's catalog (fast, searchable, full-featured). Export to `xmpDM:markers` for Premiere interop. When the deferred Resolve project integration lands, write markers into the `.drp`. The ceiling for Resolve interop without a sidecar standard is the project integration feature.

**Why deferred:** Depends on video playback in the loupe view. Interop is limited until Resolve project integration lands.

**What's needed to implement:**
1. `clip_annotations` table: asset_id, timecode_ms, body, created_at
2. Annotation UI in the video loupe view: timeline scrubber with annotation markers, add/edit/delete
3. Export to `xmpDM:markers` XMP sidecar for Premiere interop

---

### On-device Whisper transcript search

**What it is:** Run OpenAI's Whisper (small model, fully local — no API calls, no data leaves the machine) on video and audio assets containing speech. Index the transcript in FTS5 alongside all other metadata. Enables searching "find the clip where I'm explaining the crux" or "find interviews where the word 'sponsors' was mentioned."

**Why deferred:** Model download (~150MB for small, ~500MB for medium). Significant compute at ingest for long clips. Must be explicitly opt-in with clear user communication about processing time.

**What's needed to implement:**
1. Whisper inference: whisper.cpp (cross-platform, no Python dependency) via subprocess
2. `asset_transcripts` table: asset_id, timecode_ms, text (one row per segment)
3. FTS5 integration: transcript segments indexed and searchable alongside filename/tags/notes
4. UI: transcript panel in video inspector showing segments with timecodes; search results highlight matching segment and jump to timecode
5. Settings: opt-in toggle, model size selection, background processing queue

---

### Map view

**What it is:** A map view showing geotagged photos and video clips as clustered pins, with GPX track overlays and a time scrubber. Explicitly designed to be better than Lightroom Classic's map — which is clunky, requires internet, uses Google Maps (increasingly restricted and privacy-hostile), and has no GPX track integration.

**Technical approach:**
- **MapLibre GL JS** — open source Mapbox GL fork, runs natively in the Wails webview, uses OpenStreetMap tiles, has built-in clustering, and supports custom line layers for GPX tracks. No API key required, no Google dependency.
- **Offline tile caching** — download and cache map tiles for areas where the user has geotagged assets. Works without internet after initial area load. Consistent with Alexandria's offline-first philosophy.
- **GPX track overlay** — when a GPX file is associated with a collection or source, draw the route as a line layer alongside photo/video pins. Creates a visual narrative of movement through a location.
- **Time scrubber** — a timeline at the bottom of the map. Dragging it animates assets appearing in sequence along the route as they were captured. Makes reviewing a trip feel like reliving it. Pairs especially well with GPX correlation.
- **Smart clustering** — pins cluster at wide zoom and expand as you zoom in. Click a cluster to see its assets inline without leaving the map.

**Why better than LrC's map:**
- Offline-capable after first tile load
- GPX track overlay showing the actual route
- Time scrubber for temporal navigation
- No Google Maps dependency — privacy-respecting, no API key, no cost
- Works on Linux
- Fast (MapLibre GL is GPU-accelerated via WebGL)

**Why deferred:** Depends on GPS data being populated — either from EXIF or via GPX correlation. Most useful after those features exist. MapLibre GL adds frontend dependency weight.

**What's needed to implement:**
1. MapLibre GL JS integration in the frontend
2. Tile caching layer: download and store OSM tiles for bounding boxes of geotagged asset clusters; serve cached tiles locally
3. Asset pin layer: query assets with GPS coordinates, render as clustered pins via MapLibre's built-in clustering
4. GPX track layer: render associated GPX tracks as a polyline
5. Time scrubber component: filters the asset layer by capture timestamp, animates along the track
6. Click interactions: click pin → select asset and open in inspector; click cluster → zoom or show asset strip

---

### Mood / palette board per collection

**What it is:** An aggregate visual palette for a collection — the dominant colors across all assets displayed as a colour distribution strip or swatch grid. Shows at a glance whether a trip or project has a consistent aesthetic. Built directly on top of the per-asset dominant color extraction feature. Useful for content creators maintaining a visual brand and designers reviewing a body of work.

**Why deferred:** Depends on dominant color extraction being in place first. The aggregate computation is trivial once per-asset palettes exist.

**What's needed to implement:**
1. Aggregate query: union/cluster dominant colors across all assets in a collection, weighted by asset count
2. Palette board UI component in the collection inspector/detail view
3. Optional: comparison mode — show two collections' palettes side by side

---

### Shooting statistics

**What it is:** A statistics view derived from EXIF data already in the catalog — which lenses you actually use, focal length distribution, aperture distribution, time-of-day patterns, shutter count per body, shots per camera. Fun, genuinely interesting to photographers, useful for gear decisions.

**Why deferred:** Side feature, not core DAM value. Implement after the catalog is stable and well-used.

**What's needed to implement:**
1. Aggregation queries over the assets table (EXIF columns already present)
2. Stats view UI: charts/histograms for focal length, aperture, ISO, time of day, shots per camera body

---

### Analog photography support

**What it is:** Two related features for film photographers scanning their own negatives:

**1. Scanned negative / positive grouping:** Auto-group a raw scan (TIFF from scanner) with its developed/colour-corrected counterpart using filename heuristics. Similar to RAW+JPEG grouping but patterns are less standardised — scanning software varies. Grouping logic needs to handle patterns like `roll01_frame05.tif` + `roll01_frame05_edit.tif` or `roll01_frame05_positive.jpg`. The base-name matching approach in asset grouping is the right foundation; the analog case just needs broader suffix/variant pattern recognition.

**2. Analog camera/lens EXIF override:** Film scans carry the scanner's EXIF (Epson V850, Nikon Coolscan, etc.) rather than the actual capture device. A dedicated "analog metadata" section in the inspector lets the user specify the actual camera body and lens, mapping to the standard EXIF CameraMake, CameraModel, LensModel, LensMake fields, with optional FocalLength, FNumber, ISO (pushed film), and ExposureTime. Writes back via exiftool. Critically: supports **saveable analog camera presets** ("Nikon F3 + 50mm f/1.4 Nikkor") that can be batch-applied to an entire roll of scans at once. This is genuinely underserved — no mainstream DAM handles this well.

**Why deferred:** Niche but passionate audience. Depends on asset grouping (P1) being in place first for the negative/positive grouping. The EXIF override is largely a metadata editing UI problem on top of existing exiftool write support.

**What's needed to implement:**
1. Extended grouping heuristics for scan filename patterns (builds on asset grouping)
2. Analog metadata panel in InspectorView with camera/lens fields
3. Analog camera preset system: save, name, apply to selection
4. Exiftool write path for camera/lens EXIF fields (shares machinery with general metadata editing)

---

### Waveform thumbnails

**What it is:** For audio and video assets, render a visual waveform as the grid card thumbnail via ffmpeg. More useful than a generic file icon — communicates clip length and content shape at a glance.

**Why deferred:** Adds per-asset ffmpeg work at ingest for audio/video. Not blocking v1 browse/search.

**What's needed to implement:**
1. Waveform rendering subprocess via ffmpeg (`showwavespic` filter)
2. Output stored alongside other thumbnails, keyed by asset ID
3. Thumbnailer dispatcher routes audio/video MIME types to the waveform generator

---

### Clipboard support

**What it is:** Copy selected asset(s) to the system clipboard as a file reference so they can be pasted directly into DaVinci Resolve, InDesign, Finder, etc. Copies the file URL, not a rasterised preview.

**Why deferred:** Platform clipboard APIs vary (macOS NSPasteboard, Linux XClipboard/Wayland). Needs per-platform implementation behind a common interface.

**What's needed to implement:**
1. Platform clipboard abstraction (macOS: NSPasteboard file URL type; Linux: xclip/wl-clipboard)
2. "Copy" command bound to ⌘C / Ctrl+C in grid context
3. Multi-asset copy: copies all selected file URLs as a file list

---

### Duplicate source detection

**What it is:** If the same physical file is indexed under two different sources (identical content hash, different paths), surface it to the user rather than silently showing it twice. Let the user decide which catalog entry to keep.

**Why deferred:** Relies on content hash already being computed at ingest (it is). Just needs the detection query and a resolution UI. Different from the existing duplicate resolution UI (same file, two paths within one source — this is same file across two separate sources).

**What's needed to implement:**
1. Detection query: find assets with matching content hash across different source IDs
2. UI: surfaces cross-source duplicates in a review screen similar to the duplicate resolution queue
3. Resolution options: keep both, remove one, merge metadata

---

### LrC catalog bootstrap import

**What it is:** Point Alexandria at a Lightroom Classic `.lrcat` file and seed the initial catalog state from it — collections structure, keyword hierarchy, and asset file paths. The main win is collections, which XMP does not carry. One-shot operation only.

**Why deferred:** Not needed if starting fresh. Useful for photographers migrating from an established LrC workflow.

**Important constraint:** This is a one-time bootstrap only. Do NOT implement ongoing sync against the live `.lrcat` file — the schema is proprietary and undocumented (Adobe changes it between major versions without notice), and concurrent access to a live catalog risks corruption. Ongoing metadata sync uses XMP sidecars only, as designed.

**What's needed to implement:**
1. `.lrcat` reader: open as SQLite, extract collections tree, keyword hierarchy, and asset file path → collection membership mapping
2. Import wizard UI: select `.lrcat` file, preview what will be imported, confirm
3. Map LrC collection structure to Alexandria collections; map LrC keywords to Alexandria tags
4. Mark imported assets as "needs ingest" — paths are known but thumbnails/metadata still need the normal ingest pipeline

---

### DaVinci Resolve / After Effects project support

**What it is:** Parse Resolve/AE project files to extract referenced asset paths and link them to catalog assets. Enables "which clips are used in which projects" queries and surfacing project context in the inspector.

**Why deferred:** Project file formats are complex and partially undocumented. High value for video-heavy workflows but not blocking v1.

**What's needed to implement:**
1. Resolve `.drp` / AE `.aep` project file parser (extract referenced media paths)
2. `project_references` table linking project file assets to referenced catalog assets
3. UI: inspector shows "used in projects: X, Y, Z" for a clip; project asset shows its referenced clips

**Schema impact:** New `project_references` table (cheap migration, does not touch existing schema).

---

### Batch rename

**What it is:** Rename multiple selected files on disk according to a template (e.g. `{date}_{camera}_{sequence}` → `2024-07-01_Sony_A7IV_001.arw`).

**Why deferred:** Writes to source files on disk. Requires careful UX (preview before apply, confirmation, undo). The rename is a path update on the asset record. The rename command must update the filesystem and the catalog atomically (or handle partial failures gracefully).

---

### Export pipeline

**What it is:** LrC-style export with full control over the output — format (JPEG, PNG, TIFF, WebP), dimensions/resize, quality, color space, filename template, output folder, metadata include/strip options. Batch export of selected assets. Underlying engine: ffmpeg for video/audio, ImageMagick for raster.

**Why deferred:** Wide scope that can spiral. The core DAM value (find, browse, organise) must exist first. Export is a second-order concern.

**What's needed to implement:**
1. Export panel UI: format picker, resize options (fit width, fit height, exact dimensions, percentage), quality slider, filename template editor, output folder picker, metadata options (strip all, keep IPTC/XMP, keep GPS, etc.)
2. Export engine: dispatches to ffmpeg or ImageMagick subprocess per asset type
3. Batch progress UI: non-blocking, same pattern as import progress panel
4. Filename template system: variables like `{date}`, `{camera}`, `{sequence}`, `{original_name}`

---

### In-app asset converter

**What it is:** Lightweight format conversion without the full export pipeline — PNG→JPEG, HEIC→JPEG, PNG→ICO, MP4→GIF, etc. Converts in-place or alongside the original. Simpler UI than export: right-click → Convert → pick target format.

**Why deferred:** Shares the ffmpeg/ImageMagick subprocess machinery with the export pipeline. Should follow export rather than precede it.

**What's needed to implement:**
1. Conversion format matrix per MIME type (what can be converted to what)
2. Right-click context menu entry
3. Simple format picker modal with output location option (replace / alongside original / choose folder)
4. Subprocess dispatch to ffmpeg or ImageMagick

---

### Telemetry / crash reporting

**What it is:** Opt-in, privacy-respecting feature usage analytics and crash reporting. Goal: understand which features are used and which aren't, to inform prioritisation. Not advertising, not profiling, not data resale.

**Why deferred:** Privacy is a hard constraint and the implementation needs careful design before shipping.

**What to collect:** Anonymous feature usage events — "map view opened", "smart collection created", "GPX correlation run", "semantic search used". Aggregate counts. No file paths, no filenames, no metadata values, no asset content, no personal information of any kind.

**Requirements when implemented:**
- Explicit opt-in prompt, not opt-out. Never enabled silently.
- Before enabling, show the user a live preview of exactly what would be sent — no surprises.
- The event schema is documented publicly so users can verify the claims.
- Self-hosted analytics backend (Plausible or Posthog self-hosted) — no third-party analytics companies, no Google, no data brokers.
- Easy to disable at any time in settings; disabling takes effect immediately.
- Telemetry code is in the open source codebase and auditable by anyone.

**What never to collect:** File paths, filenames, metadata values, tag names, collection names, asset counts, GPS coordinates, camera models, any value that could identify the user's content or location.

---

### Localisation (i18n)

**What it is:** Support for non-English languages in the UI.

**Why deferred:** Significant ongoing maintenance burden (translation updates with every release). Third-party translation management tooling needed. Design must accommodate variable string lengths in UI layouts.

**Constraint:** String literals in application code must not be concatenated (e.g. `"File " + filename + " not found"` would require the translator to work around an awkward structure). Even before localisation is implemented, strings should be structured so they can be extracted to resource files. This is a low-cost discipline to establish early.

---

### Accessibility

**What it is:** Screen reader support (VoiceOver on Mac, Orca on Linux, NVDA/Narrator on Windows), keyboard-only navigation, sufficient colour contrast for WCAG AA compliance.

**Why deferred:** Foundational keyboard navigation (the keyboard-driven workflow already defined) provides some accessibility. Full screen reader support in a Wails/webview app requires ARIA attribute work throughout the UI component tree. This is a significant effort that is easier to address once the UI component structure is established.

**Constraint:** The color label system uses colour alone to convey meaning (Red label, Yellow label, etc.). A shape or pattern alternative must be provided for users who cannot distinguish colours. This should be implemented early as it affects the core labelling UI.

---

### Plugin / extension system

**What it is:** A public API for third-party extensions to add new file format support, custom actions, or integrations.

**Why this is permanently deferred (not just P2):** Explicitly decided against. A plugin system is a significant maintenance, security, and support burden. The API surface must be versioned and maintained forever once published. Contributors add features via code contributions or feature requests. This is a deliberate scope boundary.

---

### In-app auto-updater

**What it is:** Download-and-install updates from within the app, rather than v1's notify-and-link-to-release-page.

**Why deferred:** Wails has no built-in updater. A self-update mechanism means per-platform download/verify/replace logic, code-signing and notarisation interactions on macOS, and elevation handling on Windows. Notify-and-link delivers most of the value at a fraction of the risk. Catalog compatibility is protected independently by the schema version check at startup.

---

### Onboarding tour

**What it is:** An in-app guided tour for new users.

**Why deferred:** Online documentation is sufficient for v1. An in-app tour requires significant UI work and becomes outdated as the app changes. The empty state on first launch (with a prominent "Add Source" call to action) provides enough guidance to get started.

---

### Font viewer

**What it is:** Dedicated inspector view for TTF/OTF/TTC/WOFF/WOFF2 assets. Shows the font rendered at multiple sizes and weights, full glyph map, and a live preview field where the user can type arbitrary text rendered in the font. Multi-font comparison mode: render the same string across multiple selected fonts side by side (Google Fonts-style).

**Why deferred:** Niche but high-value for designers. No subprocess needed — `golang.org/x/image/font/sfnt` handles TTF/OTF natively in-process.

---

### Lightweight in-app editing

**What it is:** Simple edits that don't justify opening a specialized app. Heavy work always defers to external tools.

**Candidates:**
- **Text/Markdown editor:** textarea with Markdown syntax highlighting for `.txt` and `.md` files
- **Image crop/rotate/flip:** non-destructive where possible (store crop rect in catalog, apply on export); lossless JPEG rotate via `jpegtran`

**Explicit non-scope:** No RAW processing, no layer editing, no color grading. Those belong in LrC, Photoshop, and Resolve respectively.

**Why deferred:** Each editing surface is its own design and implementation effort. The reference DAM core must exist first.

---

### Catalog health dashboard

**What it is:** A two-layer system for giving users confidence in their catalog — passive background monitoring that surfaces urgent issues as they arise, and an on-demand health panel that gives the full picture across all dimensions.

**Layer 1 — passive monitoring:** Background checks surface urgent issues via the status bar right zone (a subtle warning indicator) and toast notifications. Examples: a source goes offline with a high missing-file count, a backup destination hasn't been reachable for N days, a backup is overdue, SQLite integrity check finds a problem at startup. These are things the user needs to know about without going looking.

**Layer 2 — catalog health panel:** An on-demand view showing the full health picture with traffic-light indicators (green / amber / red) per category and "fix" or "review" actions inline. Opened by clicking the status bar health indicator, or from the menu. Categories:

| Category | What's checked |
|---|---|
| **Database** | SQLite `PRAGMA integrity_check` result, WAL state, schema version current |
| **Files** | Missing files (source online but file gone), files changed on disk since ingest (hash mismatch), orphaned thumbnails with no catalog entry |
| **Metadata** | Assets with no XMP sidecar written yet, XMP sidecars that disagree with catalog values (unresolved conflicts) |
| **Organisation** | Untagged assets, unrated assets, assets not in any collection — surfaced here as informational, not errors |
| **Duplicates** | Count of pending duplicate detections awaiting user resolution |
| **Backups** | Last backup timestamp, age relative to configured schedule, backup destinations reachable, available disk space on each destination |
| **Groups** | Broken asset groups (cover asset deleted or missing) |
| **Sources** | Sources currently offline, sources with a high proportion of missing-file assets |

**Design principles:**
- The health panel never auto-fixes anything without user confirmation. It surfaces and explains; the user decides.
- Organisation checks (untagged, unrated) are informational — amber at most, never red. Not everyone tags everything and that's fine.
- Database and backup checks can be red — these are things that could result in data loss.
- The panel should be fast to open. It does not run all checks on open; it shows the last-known state from background passes and lets the user trigger a fresh check manually.

**Why deferred:** Requires background check infrastructure and a dedicated UI surface. The individual checks (integrity_check, file verification, backup status) are simpler to build than the unified dashboard. Build the checks first; the dashboard is the layer that ties them together.

---

### Catalog server mode (multi-machine access)

**What it is:** Run Alexandria as a lightweight server process on a NAS or always-on machine. Desktop clients connect to it over the local network. The catalog lives on the server, accessed through a proper server process rather than direct file access. Enables the same catalog from multiple machines (desktop + laptop) without duplication or sync conflicts.

**Why not catalog-on-NAS via SMB/NFS:** SQLite's WAL mode — which Alexandria uses for crash safety — is explicitly incompatible with network filesystems. SQLite's own documentation states that WAL does not work on NFS or SMB/CIFS due to unreliable POSIX file locking and the mmap shared memory requirement. Running the catalog directly on a NAS share is a documented corruption risk, not a theoretical one. This is exactly why Lightroom Classic has never supported it despite years of requests.

**The right answer:** A proper server mode where the DB stays on one machine and is accessed via a network API — the same pattern Plex uses for media libraries. Sources (the actual asset files) still live wherever they live; only the catalog server moves to the NAS.

**Why deferred:** Requires a client/server architectural split — a significant departure from the current single-process model. The Go backend and Wails frontend would need to support a "remote catalog" connection mode. This is a v3+ consideration, not something to design for early. The architecture should not be constrained by this today, but should not preclude it either.

**Near-term substitute:** Catalog backup-to-NAS (already planned) covers the "catalog survives machine failure" use case. The multi-machine use case requires the server mode.

---

## Maybe someday

Ideas worth preserving but with no committed timeline. Not blocked by architecture — just far enough out that detailed design would be premature. Revisit when the core product is stable and well-used.

- **Preview LUT for log footage:** Associate a viewing LUT per camera source (S-Log, LOG-C, V-Log). Alexandria applies it in the loupe for preview only — stored footage stays untouched. No DAM does this. Every video creator shooting log has this pain.
- **Focus peaking + highlight/shadow clipping in loupe:** Highlight sharp areas in a configurable color (focus peaking) and overlay blown highlights / crushed shadows as colored warnings. Makes Alexandria competitive with LrC for the photo review step.
- **Virtual copies:** Same physical file, multiple independent catalog entries with different metadata, ratings, and collection membership. LrC's concept. "This photo as color and as B&W without duplicating the file."
- **Usage rights / licensing tracking:** Per-asset license records — licensed to, usage terms, territory, expiry date. Expiry alerts. Smart collection: "expiring within 30 days." Real professional pain point currently managed in spreadsheets.
- **Sensor dust spot detection:** A bright speck recurring at the same pixel position across dozens of frames from the same camera = sensor dust. Detect the pattern across a collection and alert the user.
- **Audio library with BPM / key / mood:** For content creators licensing music. Extract BPM and key via ffmpeg/aubio. Tag mood and genre. Track which projects each track has been used in and license expiry dates.
- **Content planning / shot list:** Create a shot list attached to a collection before a trip ("need: establishing wide, golden hour, action, talking head"). After ingest, Alexandria shows coverage — which shots were captured vs. planned. Pre-trip intent meets post-trip reality.
- **Before/after comparison view:** Side-by-side loupe of a RAW and its edited counterpart with a split-drag handle. Portfolio review, client approval, teaching.
- **Client delivery workflow:** Mark a collection as a client project, select picks, generate a watermarked proof package or a full-res delivery package. Track delivery status. Lightweight replacement for Pixieset / ShootProof for working photographers.
- **Watermarking at export:** Text or image watermark applied at export time, configurable position/opacity/size. For client proofing.
- **Garmin Connect API integration:** Auto-pull recent GPX tracks from Garmin Connect as a convenience layer on top of plain GPX import. Opt-in, OAuth. Secondary to the core GPX correlation feature.
- **Subtitle / caption management:** Store and manage SRT/VTT files alongside video assets. Preview video with subtitles in the loupe. Export video+subtitles together.

---

## Implementation order recommendation

When these features are prioritised, suggested order based on user value and implementation dependency:

1. **Asset grouping (RAW+JPEG+XMP auto-grouping)** — highest user value for creative professionals with RAW+JPEG workflows
2. **Smart collections** — enables powerful filtering workflows; backend query builder unblocks this
3. **Perceptual hash / similar detection** — cheap ingest addition, high culling value
4. **Dominant color extraction** — cheap ingest addition, enables color-based search
5. **Waveform thumbnails** — low effort, high value for video/audio workflows
6. **Clipboard support** — removes friction from the core find-then-use workflow
7. **Duplicate source detection** — content hash already computed; mostly a UI problem
8. **LrC catalog bootstrap import** — high value for migrating photographers
9. **DaVinci Resolve project support** — high value for video workflows
10. **Integrity check** — important for catalog reliability as libraries grow
11. **Duplicate review queue** — backend detection is done; just needs UI
12. **Export pipeline** — broad scope; implement after core catalog is stable
13. **In-app asset converter** — shares export machinery; implement alongside or after export
14. **Font viewer** — niche but low effort given sfnt library
15. **Lightweight in-app editing** — each surface is independent effort; batch rename first
16. **AI/ML tagging and semantic search** — high user value but significant infrastructure; requires sqlite-vec and CLIP decisions
17. **Localisation** — lower priority until user base has clear non-English demand
18. **Accessibility** — should be addressed before any v2 release
19. **Telemetry** — if revenue/sustainability requires product analytics
