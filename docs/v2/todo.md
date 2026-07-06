# Resolved (2026-07-03, moved into docs)

- New file types without rewrites → MIME/extension → dispatcher map; external tools as subprocesses (exiftool/ffmpeg). See 01 + 04.
- Markdown files → treated as documents (01). Per-asset notes → `assets.note`, in FTS, synced to XMP dc:description (03 + 07).
- Media backup → explicitly out of scope; catalog backup only. Documented in 01.
- Interface granularity → settled: repository interfaces + two dispatch interfaces (Thumbnailer, MetadataExtractor). Don't split further.

# Open

- DaVinci Resolve / After Effects project support: parse project file, link referenced assets. Needs a `project_references`-style table (new table, cheap migration) — NOT asset_groups. Post-v1.
- Auto-grouping of RAW + JPEG + XMP sidecars (P1, drives asset_groups design): files sharing the same base filename in the same directory are automatically grouped at ingest — e.g. IMG_1234.CR3 + IMG_1234.JPG + IMG_1234.xmp become one group, RAW as cover asset. Common shooting pattern for adventure/event photographers who shoot RAW+JPEG simultaneously. Eagle does not do this. XMP sidecars (both .xmp and .CR3.xmp naming conventions) are always attached to their RAW, never shown as standalone assets.
- Export pipeline (post-v1): LrC-style export with full control — output format, dimensions/resize, quality, color space, filename template, output folder, metadata include/strip options. Should support batch export of selected assets. Underlying engine: ffmpeg for video/audio, ImageMagick for raster.
- In-app asset converter: format conversion without the full export pipeline — png→jpg, heic→jpg, png→ico, mp4→gif, etc. Converts in-place or alongside the original. Shares the same ffmpeg/ImageMagick subprocess machinery as export. Simpler UI: right-click → Convert → pick format.
- Font viewer (post-v1): dedicated inspector view for TTF/OTF/TTC/WOFF/WOFF2 assets. Shows the font rendered at multiple sizes and weights, full glyph map, and a live preview field where the user can type arbitrary text to see it rendered in the font. Multi-font comparison mode: type the same string and see it rendered across multiple selected fonts side by side (Google Fonts-style). Underlying parsing via golang.org/x/image/font/sfnt or similar — no subprocess needed for most of this.
- Lightweight in-app editing (post-v1): simple edits that don't justify opening a specialized app. Heavy work always goes to the appropriate external tool. Candidates:
    - Text/Markdown editor: simple editor for .txt and .md files, syntax highlighting for markdown.
    - Image crop/rotate/flip: non-destructive where possible (store crop rect in catalog, apply on export); lossless JPEG rotate via jpegtran. No raw processing, no layers, no color work — that's LrC/Photoshop's job.
    - Batch rename: rename selected assets with a template (sequence, date, metadata fields).
- LOGGING - logs should be rich but concise and readable. Colors should be used to denote log levels and source components. (Note: file logs are JSON via slog; colored text handler for dev mode.)
- AI auto-tagging?
- AI face detection and grouping?
- AI assisted culling? - probably not, this feels more like a LrC or photo management specific task.
- Perceptual hash (phash) for duplicate/similar image detection: cheap, no model needed, very useful for culling burst shots. Store phash as a column in assets table at ingest. Post-v1 but earlier than full AI search.
- Dominant color/palette extraction: extract 5-8 dominant colors per asset at ingest via k-means or median cut over a downsampled version of the thumbnail — no AI, cheap enough to run on everything unconditionally. Store as hex values in the DB. For video, sample a few keyframes and union the palettes. Enables: filter/search by color, sort by dominant color, color picker in filter bar. Color proximity search should use deltaE in LAB colorspace rather than RGB distance (human perception isn't linear in RGB). Schema should accommodate a palette column from early on.
- On-device semantic AI search (post-v1, significant effort): CLIP-based image+text embeddings so users can search "sunset over water" or "person on a rock face" and get relevant results. Architecture: generate embedding vector per image asset at ingest (optional/configurable, compute cost is real especially on Intel); store vectors via sqlite-vec extension (SQLite-native, no separate vector DB); at search time embed query text and do ANN search merged with existing FTS5 results. Model (~600MB) downloaded on first opt-in, not bundled. Must be fully on-device — no API calls. Schema should accommodate an embeddings table from early on even if the feature ships later.
    - Video AI tagging uses the same CLIP pipeline via frame sampling (ffmpeg extracts keyframes, each frame goes through CLIP, tags aggregated across frames). Scene-level tags work well ("outdoor", "climbing", "rock", "sunset"). No temporal understanding. Sample rate is tunable — 1fps is thorough but expensive on Intel; 1 frame/5-10s is a reasonable default for video. Make video AI tagging independently opt-in from photo tagging given the higher compute cost.
- Maybe allow user to select "LrC is source of truth" or "alexandria is source of truth" for metadata. This will give a clean and well understandable choice as to which program owns raw and rasterized image metadata.
- One-time bootstrap import from LrC catalog (.lrcat): read collections structure, keyword hierarchy, and asset file paths from the SQLite catalog to seed Alexandria's initial state. Collections are the main win — XMP doesn't carry them. One-shot operation only; do NOT attempt ongoing sync against the live catalog (proprietary schema changes between LrC versions, concurrent access risks corruption). Ongoing metadata sync stays XMP-only as designed.
- Write "catalog file" like LrC. This will make it easy to test in a particular catalog environment, manage multiple catalogs, and make backups for general use and before updates.
- Catalog backup improvements over LrC:
    - **Smart retention policy** (not just "keep last N"): keep all backups from last 24h; one per day for the last week; one per week for the last month; one per month beyond that. Graduated density — recent history is dense, older history is compressed automatically. Still user-configurable for those who want a different scheme.
    - **Multiple backup destinations**: configure up to N destinations independently (e.g. local machine + mounted NAS share). Each destination has its own retention policy. If a destination is unavailable at backup time, skip it gracefully with a logged warning — do not block or fail the backup to available destinations.
- Import modal options: when triggering an import, the modal should offer: (1) auto-create a named collection from this import — name pre-filled, checked by default; (2) skip suspected duplicates — checked by default, with a count shown post-import; (3) apply a saved metadata preset to all imported assets (copyright, creator, rights, etc.) — dropdown of saved presets, optional.

- Tags should have an optional customizable color with custom color selection (full color picker, not a fixed palette), with optional color inheritance to child tags.
- Color labels: user-defined, indeterminate number, each with a custom color (full color picker). Not limited to a fixed set like LrC's 6. Each label has a user-defined name and color. Stored per-catalog so label sets can differ across catalogs.
- Thumbnail auto-refresh on external edit: when the watcher service detects an mtime change on a source file, explicitly queue thumbnail regeneration — not just a catalog record update. If a PSD is edited in Photoshop or a RAW is touched by another tool, the grid card should reflect the updated embedded preview. This is digiKam's most-reported complaint.
- Manual stacking: user-initiated "collapse these assets into one card" operation, distinct from auto-detected asset groups. A stack has a user-chosen cover asset; all other members have no semantic role. Uses the same asset_groups schema (role = "member", one role = "cover") but is created manually, not by the ingest pipeline. UX: select assets → right-click → Stack; cover asset is the one on top; collapse/expand in grid.
- Bulk write metadata to XMP: a "Write all metadata to files" bulk operation (like LrC's Metadata → Save Metadata to Files). Writes catalog metadata (ratings, tags, color labels, notes) to XMP sidecars for selected assets or the entire catalog. Available as a one-shot action and as a scheduled/automatic option. The ongoing XMP sync handles incremental updates; this covers migration from LrC and periodic "flush to disk" scenarios.
- Catalog safety — backup must use SQLite backup API or VACUUM INTO, never a raw file copy. Copying an open SQLite database file is the most common real-world catalog corruption cause (captures inconsistent WAL state). All backup paths must go through the proper API.
- Catalog safety — use PRAGMA synchronous=FULL. Slightly slower than NORMAL but eliminates the power-loss window for committed transactions. Correct default for a catalog described as the user's primary organisational work product.
- Catalog safety — detect and prevent two Alexandria instances opening the same catalog simultaneously. Use a lock file at catalog open; refuse to open and surface a clear error if the lock is held. Prevents SQLite locking conflicts from concurrent processes.
- Catalog safety — handle disk-full mid-write gracefully. Detect SQLITE_FULL errors, surface a clear user-facing message, and ensure the catalog is left in a consistent rolled-back state rather than a silent partial write.
- Export ordering: when batch exporting, maintain correct sequence order in output. LrC's multi-threaded export doesn't preserve order; this causes downstream problems. Alexandria's export pipeline should finish files in the sequence the user defined.
- Collection identity: each collection should have a selectable cover image, a free-text description, and an auto-computed date range from its assets. Turns a collection from a query result into something with a sense of place and context. Small schema addition (cover_asset_id, description columns on collections table).
- Grid grouping: a "group by" mode in the grid toolbar that organises assets into collapsible sections — by file type category (image, video, audio, document, font, GPX, etc.), by date (year / month / day), by source, by rating, or by color label. A view mode, not a structural change.
- Configurable ignore list: glob patterns and/or extensions to skip during import (e.g. `.DS_Store`, `Thumbs.db`, `.tmp`, `*.lrprev`). Global defaults ship with sensible system-file exclusions; user can add/remove entries in settings. Checked at the scanner level before any processing happens.
- Configurable "open in" programs: per MIME type / extension, seeded from OS defaults (macOS Launch Services, xdg-open on Linux) on first run, then user-overridable in settings. Stored in machine.json since it's machine-specific (installed apps vary per machine). UI: right-click → Open In → [app list] with a "Set as default" option.
- Waveform thumbnails for audio and video assets: render a visual waveform as the grid card thumbnail via ffmpeg. More useful than a generic file icon and immediately communicates clip length and content shape.
- Clipboard support: copy selected asset(s) to system clipboard so they can be pasted directly into Resolve, InDesign, Finder, etc. Should copy the actual file reference (file URL on clipboard), not a rasterised preview. Feels native, removes friction from the core "find then use" workflow.
- Duplicate source detection: if the same physical file is indexed under two different sources (identical content hash), surface it to the user rather than silently showing it twice. Flag in the UI, let the user decide which catalog entry to keep. Detected via existing content hash column at reconciliation time — no extra ingest cost.
- Metadata: read and edit all commonly used metadata fields as first class. exiftool already handles the breadth of formats so this is mostly a schema and UI question rather than new extraction work. Fields frequently filtered/sorted on get dedicated columns; everything else in the JSON blob. Editing writes back via exiftool for most formats. Exceptions: RAW files write to XMP sidecar only (never modify the raw); proprietary formats (PSD, AI, INDD, Affinity) are read-only for metadata. Commonly used field sets by category:
    - **EXIF (images):** capture datetime, camera make/model, lens, aperture, shutter speed, ISO, focal length, GPS coordinates, flash, metering mode, white balance, color space
    - **IPTC/XMP (images):** title, caption/description, creator/author, copyright notice, usage rights, city, state, country, headline, credit, source
    - **Audio (ID3/MP4):** title, artist, album, track number, year, genre, comment, album art
    - **Video:** title, description, copyright, creation date

## Hotkeys
- L and R arrows move forward and back
- l dims all UI except images and previews, LrC Style
- F with assets selected makes a fullscreen view like LrC

## Building and Deploying
- Should have per piece testing and build scripts, then scripts at root for building and testing
- Packaging can be done in dev environment and by github actions scripts (both github and forgejo runners)
- Releases will be manual and will be done via github
- How will app be updated? The update flow is still completely undecided


## UI
- Logging - important. Want users to be able to upload a log file in case they're having an issue
- Auto-detect removable volumes on insert: when a memory card or external drive is plugged in, detect it and surface a toast notification ("Card detected — import?") rather than a modal. User can dismiss or click to start import. Frictionless first step after every shoot.
- Time offset / timezone correction: quick action on any asset selection to shift capture timestamps by a fixed offset (e.g. +1h for DST, -5:30 for timezone). Covers: wrong timezone set on camera, forgetting to update for daylight savings, multi-camera clock drift. Must happen before GPX correlation — timestamp accuracy is required for correct track-to-photo matching. Writes corrected time to XMP/EXIF via exiftool.
- VFR (variable frame rate) detection: flag clips recorded in VFR mode at ingest. Badge on grid card, warning in inspector. VFR causes audio sync drift in Resolve timelines; editors need to know before bringing footage in.
- DaVinci Resolve codec compatibility: detect codecs incompatible with Resolve Free (H.265 in certain containers, some RAW formats) at ingest. Badge on grid card. Offer in-app transcode to H.264 or ProRes via ffmpeg — same machinery as asset converter. Creates new file alongside original, never overwrites.
- Frame rate mismatch indicator: when a collection contains clips at mixed frame rates (24fps, 30fps, 60fps etc.), surface per-clip badge and a collection-level notice. Offer in-app transcode of selected clips to a target FPS (23.976, 24, 25, 29.97, 30, 60) via ffmpeg. Nice-to-have, not critical path.
- Clip duration on grid card thumbnails: show duration directly on video/audio thumbnails in the grid. Not just in the inspector.
- Status bar: persistent bar at the bottom of the main view with three zones:
    - **Left**: current context — collection or folder name + total asset count ("Yosemite Trip · 347 assets")
    - **Center**: selection scope — count, total file size, total duration if video in selection ("23 selected · 1.4 GB · 14m 32s"). Hidden when nothing is selected.
    - **Right**: background worker status — compact indicator when any background operation is running ("Importing 234/1,330 · ~2m", backup status, integrity check, watcher). Expands to full progress panel on click. Always visible, never intrusive.
- Asset counts in sidebar: show count badge next to every collection, folder, and source in the sidebar. Always visible.
- Copy / paste metadata: select a source asset, copy its metadata (rating, tags, color label, custom fields), select target assets, paste. Invaluable for multi-camera shoots — cull one card then apply the same treatment to matching moments on the second camera's card.
- Auto-advance in loupe/filmstrip: toggleable setting (default off) — when enabled, pressing P/X/rating key automatically advances to the next asset. Default off; arrow key manual advance is the natural rhythm for most users.
- Configurable grid card metadata overlays: user chooses which fields and badges appear on grid cards — rating stars, color label, file type, GPS indicator, VFR flag, audio channel indicator, clip duration, filename, capture date. Stored in settings per view. Similar to LrC's grid view info overlay configuration.
- GPS indicator on grid cards: small location pin badge on assets that have GPS coordinates. Shows which assets will appear on the map and which need GPX correlation. Part of the configurable grid card overlay system.
- Audio channel indicator on video: show stereo / mono / no audio on video assets. Badge on grid card (part of configurable overlays), detail in inspector.
- Sequence number rollover handling: cameras reset from IMG_9999 to IMG_0001. Detect this pattern and sort by capture timestamp rather than filename. Also handle multiple cameras at different shutter counts in the same import — never rely on filename sort when timestamps are available.
- Pick / reject keyboard shortcuts: P for pick, X for reject in loupe and grid. Standard triage shortcuts photographers have muscle memory for.
- Side-by-side comparison mode: select 2–4 assets and view them at equal size for deliberate comparison. Ratings, labels, and flags accessible without leaving the view. For picking hero shots from similar candidates.
- Filmstrip in loupe view: horizontal strip of thumbnails at the bottom of the loupe/detail view (like LrC). Navigate the current selection without returning to the grid. Essential for keyboard-driven triage.
- Adaptive inspector by broad asset type: image, video, audio, document, font, GPX track each get a tailored inspector layout showing relevant fields. Asset groups get a separate inspector design that surfaces the group structure and per-member details. No showing empty camera EXIF panels for audio files.
- "Needs attention" built-in smart collections: Untagged, Unrated, Not in any collection, Import errors. Always present but removable by the user (they can re-add from a system collections list). Act as a continuous to-do list for catalog hygiene.
- Quick preview: press Space over any asset in the grid for a full-screen quick-look preview. Dismiss with Space. No click, no mode change.
- Recently used / recently accessed prioritized everywhere: tag picker, collection picker, open-in app menu, search suggestions. Standard good UX — don't bury the things people use most.
- Sort options: ingest date (when Alexandria imported the asset) vs capture date (when it was shot). Both exposed as sort options. These are different and both useful.
- OS-level notifications: send a desktop notification (macOS UNUserNotificationCenter, Linux libnotify) for background operations the user may have walked away from — import complete (with summary: N added, N skipped, N errors), integrity check found issues, catalog backup failed, a backup destination was unavailable. Non-intrusive, respects system Do Not Disturb.
- Mixed state indicators in batch metadata editing: when a selection has conflicting values for a field (e.g. different ratings across selected assets), show a mixed state indicator rather than blank. Applying a value from mixed state applies to all selected. Matches LrC behaviour.
- Multi-select keyboard behaviour: Cmd+click toggles individual item selection; Shift+click extends selection to a contiguous range; Cmd+Shift+click adds a discontinuous range to the existing selection. Standard macOS selection model, followed exactly.
- Drag and drop into sidebar collections: drag selected assets from the grid onto a collection in the sidebar to add them. Also support dragging between collections. Natural and expected.
- Undo tooltip: show what will be undone on hover over the undo control — "Undo: Set rating on 23 assets." Removes anxiety about what Cmd+Z will do.
- Heirarchical folder sidebar component with Folders nested inside sources.
- Heirarchical collection sidebar component
- Heirarchical tag sidebar component
- Loupe view with full size render of asset
- Map view to see photo geolocations on a map? The whole location mapping thing is interesting - how do we generalize coordinates to a town or area that someone might search for?
- Metadata editing
- User should be able to select UI color scheme beyond just dark and light - users will be using alexandria for color sensitive work, so having neutral grey as an option is important and should likely be the default.
- The UI should be spartan, but nice to look at. Compact, respectful of limited screen space. Clean, generally flat colors with clear text heirarchy, retrofuturistic inspiration and design elements. The UI should be nice to look at, but get out of the way of the assets.
- Typography split: data values (metadata fields, file sizes, dimensions, counts, dates, paths) render in a monospace font; all other UI text uses a clean sans-serif. This creates a clear visual distinction between "data" and "interface."
- Accessibility and multi language support is important.

### UI Refresh
- System pieces
    - i18n
    - Logging and observability
    - Testing system - unit tests are absolute minimum. Also want test coverage data.
    - Scss or sass type thing - i want to have better styling features than just basic css
    - Do we need or want some frontend state management? I would try to lean away from this where possible. State management is a headache.
    - Linting - eslint probably
    - Any others you can think of?
- Styling and Page Structure
    - I want to focus on building out a unique style and the page structure, I don't want to focus on responsiveness, scaling, etc. We will not support mobile or tablet. Only desktop for now.
    - How can we use a system to give us responsiveness and scaling, as well as grids and such, without a massive headache of handling it ourselves?
- Components (Non exhaustive, just an example of structure and such)
    - Want to have components for domain concepts and bits of decoration.
    - Asset - something that populates onto the grid, represents the asset domain model
    - Tree - Resuable component for displaying heirarchichally structured data. Folder structures, collections, tags, etc. Selector at top. See reference image.
    - Modal - Wrapper around some inner content - could be for user settings, could be for smart collection definitions.
    - Button
    - Tag
    - InputField
    - Views
        - GridView - The actual grid view. Shows assets and asset groups
        - InspectorView - The inspector panel - all information about a single asset or asset group.
            - "Contained in" section: shows the asset's folder path and every collection it belongs to. Each is clickable to navigate there.
            - Collection membership affordance: ability to add/remove the asset from collections directly in the inspector.
        - BrowserView - The tree component, the left sidebar thing.


### Landing Page?
- Import
- Bookmarked Collections

## Customization and User Settings
- Hotkeys, of course
- Worker pool size controls in settings: user-configurable counts for hash workers, metadata extraction workers, and thumbnail workers. Two presets — "performance" (more workers, faster import, higher resource use) and "lightweight" (fewer workers, slower import, stays out of the way of LrC/Resolve) — plus manual sliders for power users. Stored in machine.json since these are hardware-dependent. Default to conservative values.
- Custom background image/ color
- Custom glassmorphism opacity
- Select which metadata fields are shown/ not shown
- Bookmark collections and sources
- Persist sizing and scaling of elements between sessions. Save sidebar sizing and collapsed/expanded, grid zoom, current view and selection