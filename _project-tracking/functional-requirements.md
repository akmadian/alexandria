# Functional Requirements

**The single source of truth for the feature roadmap: what will be built, and when.** Consolidated from the original PRD, the retired todo.md, the backend design round (2026-07-06), and the frontend/UX design round (2026-07-07) — where an older source disagrees with the 2026-07-07 design round, the design round wins and this file reflects it. Prioritized by both technical build order (what must exist first) and user value (what makes the app worth using). Video-specific features are P2+ per project decision.

**Priority definitions:**

- **P0** — The app does not function without these. Core infrastructure and MVP.
- **P1** — Without these, no one uses the app over alternatives. Essential for daily workflow.
- **P2** — High-value features that differentiate Alexandria. Early post-launch.
- **P3** — Important but can wait until the core is stable and well-used.
- **P4** — Far future / maybe someday.

---

## P0 — Core Infrastructure and MVP

These are the foundation. Everything else depends on them.

### Catalog and Database

- SQLite catalog in WAL mode as the single source of truth
- Schema migration system with versioned migrations, automatic backup before migration, and schema version check at startup
- UUID primary keys, ISO 8601 timestamps throughout
- `PRAGMA synchronous=FULL` for crash safety
- `PRAGMA integrity_check` at startup (background, non-blocking, warn on failure)
- Instance lock — detect and prevent two Alexandria instances opening the same catalog simultaneously (advisory lock file, clear error if held)
- Disk-full mid-write handling — detect `SQLITE_FULL`, surface clear message, ensure consistent rolled-back state
- Catalog-scoped settings in a key-value table with JSON values
- Machine-local settings in `machine.json` (worker pool sizes, memory limit)
- Catalog file layout: `catalog.db`, `thumbnails/`, `backups/`, `alexandria.log` in platform-appropriate app data directory

### Source Management

- Source = watched root (local folder, external drive mount, SMB/NFS share)
- Multiple sources supported
- Configurable scan behaviour (recursive or not)
- External drives identified by filesystem UUID (stable across mount point changes), with disk serial as fallback
- Network shares identified by host + share name
- Sources can be marked inactive without removal
- When a source goes offline, its assets remain fully browsable in the catalog

### Ingest Pipeline

- Six-stage pipeline: SCAN -> HASH -> MATCH -> EXTRACT -> THUMB -> WRITE (ENRICH joins between THUMB and WRITE with the signals milestone, P3)
- Stages connected by buffered Go channels, decoupled
- Scanner skip check: files with unchanged mtime + size are skipped (idempotency gate). 2-second mtime comparison tolerance for FAT/exFAT/SMB
- xxHash of first 64KB + file size for fast change detection and dedup
- MATCH per D20 (detect-and-flag, never auto-mutate): any content match at a new path — move, rename, or copy — mints the new path as a distinct asset and records a pending pair in the `duplicates` ledger for Review; nothing is ever auto-relinked or auto-merged
- Per-file error handling — individual file failures do not abort the pipeline
- Mid-scan source disconnect (drive unplugged, share dropped) must never mass-mark unvisited assets missing — the walk-completeness guard is an open design task (`backend/04-open-questions.md` #15)
- Cancellable at any point via context
- Batched catalog writes (default 50 per transaction) for performance
- Import is idempotent — re-running on unchanged source is essentially free
- Manual trigger (user clicks Import, selects source)
- Non-blocking — app stays fully usable during import
- Import progress shown in persistent, non-blocking progress panel with counts and current stage
- Assets appear in grid incrementally as batches commit, fully rendered (no placeholder state)
- Import summary at completion: N added, N updated, N skipped, N errors with file path and reason

### Thumbnail Generation

- Generated at ingest time, stored in app data directory keyed by asset ID
- Stored separately from source files — survives source going offline and file moves
- Thumbnail cache is rebuildable (documented to users)
- Dispatched per MIME type to appropriate generator
- Thumbnail directory uses two-character UUID prefix subdirectories to avoid filesystem limits
- Supported approaches by type:
  - Standard raster (JPEG/PNG/TIFF/GIF/BMP/WebP): native Go image libraries
  - RAW: extract embedded JPEG preview via exiftool; fall back to dcraw_emu/libraw CLI
  - PSD: extract embedded composite via ImageMagick CLI or Go psd library
  - AI (Illustrator): treat as PDF, Ghostscript CLI for preview
  - InDesign: extract embedded preview from file header
  - Affinity (.afphoto/.afdesign/.afpub): extract embedded preview thumbnail from file header
  - Video: ffmpeg/ffprobe CLI for thumbnail frame
  - Audio: ffmpeg CLI for metadata (waveform thumbnails deferred)
  - SVG: rasterize for thumbnail
  - PDF: first page preview via Ghostscript CLI
  - Markdown: text-preview thumbnail
- External tools invoked as subprocesses, never cgo bindings — subprocess crash kills the subprocess, not Alexandria
- Extension floor (from `_scratch/file-types-to-support.md`), beyond the list above: HEIC (raster path via platform decoder or libheif), plain text/CSS (text-preview like Markdown), fonts (woff/woff2/ttf — specimen-render thumbnail; full viewer is P3), zip (classify + generic card, no preview). Adding a type = one registry row per side (backend `assettype`, frontend presentation registry)

### External Tool Acquisition

*The subprocess fleet (exiftool, ffmpeg, dcraw_emu, ImageMagick, Ghostscript) gates RAW/video/PSD/PDF thumbnails and metadata — the acquisition UX is part of making P0 formats work. From `_scratch/sysde.md`.*

- Auto-detect tools on PATH at startup, plus common install locations
- In-app UX offering to fetch/download missing tools where licensing permits; otherwise clear guidance to install
- Downloaded binaries are a security boundary: HTTPS only, checksum verification, signature verification where the upstream provides one
- Missing tool degrades gracefully per the registry's nil-capability rule (generic card, no crash) — the app is fully usable with zero external tools, just with fewer formats enriched

### Metadata Extraction

- Extracted at ingest time, stored denormalized in assets table for fast querying
- Commonly filtered/sorted fields as dedicated indexed columns: file type, rating, color label, capture date, camera model, etc.
- Extended/format-specific metadata in JSON blob column
- EXIF, IPTC, and XMP all extracted where present
- Extraction failure is non-fatal — corrupt EXIF does not prevent indexing

### Asset Grid View

- Virtualized rendering — must scroll at 60fps with 10,000+ assets visible
- Renders only visible window plus padding buffer
- Grid-resolution thumbnails; higher-resolution previews load after scroll settles (debounced)
- Selection model: Cmd+click toggles individual, Shift+click extends contiguous range, Cmd+Shift+click adds discontinuous range
- Select all / deselect all
- Asset card shows: thumbnail, filename, rating stars, color label, flag indicator, file type badge
- Density control (adjustable tile size)

### Basic Organization

- **Star ratings:** 0-5, stored per asset
- **Color labels:** Red, Orange, Yellow, Green, Blue, Purple (v1 fixed set; custom labels are P2)
- **Flags:** Pick or Reject, stored per asset
- **Tags:** Hierarchical (parent-child). Tags can be applied to any asset regardless of file type
- **Per-asset notes:** Free-text, included in full-text search
- **Collections:** Manual collections with user-curated membership. Nestable.
- **Soft delete:** Requires explicit user action, confirmation modal with "remove from catalog" vs "delete from disk". Disk deletion gets additional warning and cannot be undone.

### Basic Search and Filtering

- Search on: filename, file type, tag, rating, color label, flag, capture date range, source, dimensions, camera make/model
- Full-text search via SQLite FTS5 covering filename, camera/lens fields, tag names, and per-asset notes
- Target: results in under 200ms on 500k asset catalog

### Frontend Shell and Navigation

- Single-window desktop app (Wails v2), no router — view state, not URLs
- CSS Grid shell: FilterBar (top), Browser (left), Main region (center, grid or loupe), Inspector (right), StatusBar (bottom)
- Pane resizing via drag handles, widths persisted to localStorage
- Pane collapse (hide browser, hide inspector)
- Desktop only — no mobile, no tablet, no breakpoints
- Three themes: graphite (neutral grey default for color-critical work), dark, light
- Theme persisted to localStorage, applied before first paint (no flash)
- Design tokens system (CSS custom properties) with semantic token layer
- CSS Modules for component styling, no Tailwind
- Typography split: monospace for data values, sans-serif for UI text

### Browser / Sidebar

- Segmented selector at top: Sources | Collections | Tags
- Hierarchical tree component (reusable across all three modes)
- Folder tree nested inside sources
- Selecting a node filters the main grid to that scope

### Inspector Panel

- Displays all metadata for selected asset
- Triage controls: rating, flag, label, note, tags
- "Contained in" section: folder path and every collection the asset belongs to (clickable)
- Collection membership management (add/remove from collections directly)

### View Mode System

*Locked 2026-07-07 (CONSTANTS C2): `view state = viewMode(query + arrangement, selection + cursor)`. A view mode is a pure renderer over shared catalog state — all modes present the same working set, arrangement, selection, and cursor; only rendering and input mapping change. Full spec: `frontend/02-state-model.md`.*

- Four modes: **Grid** (G, P0), **Loupe** (E/Enter, P0), **Compare** (C, P2), **Cull** (D, P1)
- Switches are single keys, instant (<100ms), stateless — no navigation stack; Escape always steps down toward Grid
- Space in Grid = quick preview (Finder Quick Look convention) — a peek, not a mode change
- Per-type presentation lives inside modes via the frontend type registry (zoomable image, video player, waveform, font specimen, PDF pages)

### Loupe / Detail View

- Cursor asset rendered large + filmstrip (LrC-style horizontal thumbnail strip — navigate without returning to grid)
- Navigate next/previous within current working set
- Per-type body via the type registry
- Close/return to grid via Escape

### Open in External App

- Open selected asset in OS-default application
- Reference model: Alexandria never edits files

### Startup Sequence

- Ordered stages: resolve catalog dir -> acquire instance lock -> open SQLite -> run migrations -> integrity check (background) -> wire dependencies -> seed defaults (first launch) -> start watcher -> check for updates (background) -> emit app:ready -> background catch-up scan
- First launch: empty state with prominent "Add Source" call to action
- Two hard exits: can't open database, can't migrate safely. Everything else degrades gracefully.

### Performance Targets

- Grid: 60fps scroll with 10,000+ assets
- Search: <200ms on 500k catalog
- Startup: interactive in <3s on 100k catalog (progressive loading)
- Import throughput: 500 JPEGs/minute on average hardware
- Library size ceiling: 500k assets (design target)

### Resource Management

- Worker pool sizes configurable, default conservative (4 hash, 2 extraction, 2 thumbnail)
- `GOMEMLIMIT` set at startup based on available RAM
- Never saturate CPU/disk I/O to degrade co-running creative apps
- Background operations run at low priority, yield to user-triggered operations

### Platform Support

- macOS: first-class, primary development target
- Linux: first-class, full feature parity
- Windows: supported, third priority

### Security and Privacy

- Catalog file created with permissions 0700
- No telemetry without explicit opt-in
- No network requests except update checks (disableable)
- No files or metadata sent to any server
- External tools as subprocesses, never cgo — subprocess crash isolation

---

## P1 — Essential for Daily Workflow

Without these, the app isn't competitive with existing tools for the target user's daily work.

### XMP Sync / Lightroom Classic Interop

*Key differentiator. Depends on: metadata extraction, tag system, watcher service.*

- Read XMP sidecar files written by Lightroom Classic: sync ratings, color labels, keywords, notes (dc:description) into catalog
- Optionally write XMP sidecars so Alexandria changes appear in LrC
- Conflict resolution: user-configurable "XMP wins" (default) or "catalog wins"
- Tags always merge (never silently delete tags from either side)
- XMP hash tracking to detect external changes and prevent self-triggered sync loops
- Inbound sync triggers: at ingest, on .xmp file change (via watcher), manual sync
- Outbound sync triggers: on catalog field change (if catalog_wins), manual sync
- Color label locale normalization (LrC writes locale-dependent strings)
- Never write into proprietary files (Affinity, InDesign) — sidecar only
- Sidecar write merges into existing content (preserve LrC develop settings, other fields)

### Keyboard-Driven Workflow

*Core value proposition for creative professional triage. Depends on: basic organization, grid view.*

- Action registry: every user-invokable operation is an entry `{id, title, aliases, context predicate, handler, default binding}`; grows incrementally as features land (spec: `frontend/04-keyboard-and-actions.md`)
- Verb grammar (locked 2026-07-07): **universal verbs** identical everywhere — navigate (arrows/J/K), rate (1–5, 0 clears), label (6–9, − clears), flag (P/X/U), view modes (G/E/C/D), Escape steps down, open-in (O), quick preview (Space in Grid); **media verbs** type-interpreted via the type registry — Space = "engage the asset" (photo: 100% zoom; video/audio: play/pause). Asset type is never a dispatch dimension
- All keybindings user-configurable
- Context-scoped dispatch `(context, key) → action`: global, grid, loupe, compare, cull, import, review, palette
- Platform-normalized modifier: `primary` = Cmd on Mac, Ctrl on Win/Linux
- **Keybinding preset sets**: named defaults in code — Alexandria (= LrC grammar), darktable, (Capture One candidate); first-run picker; user overrides layer on top of the chosen preset
- Conflict detection when reassigning keys
- Reset to defaults (per-binding or all)
- Defaults live in code as data; user overrides in `keybindings.json` (impl/11) — new actions auto-appear on update

### Command Palette

*Pulled forward from P2 (2026-07-07 design round): ships WITH the keyboard system because it is the action registry's face — every entry shows its current binding, so it teaches the keys. It also absorbs chrome: rare actions (Open Settings, Export Logs, Rebuild Thumbnails) live here instead of toolbar real estate. Spec: `frontend/04-keyboard-and-actions.md`.*

- Fuzzy subsequence matcher (VS Code/fzf scoring) over titles + aliases, filtered by context predicate; frecency ranking
- Prefix modes: bare text = search (parses via the search tiers, emits pills), `>` = actions, `#` = go-to (collection/tag/source)
- **Cmd+K** opens in action mode; **Cmd+F** (or `/`) opens the same palette in search mode — the global retrieval entry point
- Healthy-palette test: everything reachable in the palette has a home elsewhere too (except deliberately-buried rare actions)

### Cull View Mode

*The ingest-day weapon; benchmark is Photo Mechanic speed. Absorbs the former standalone Lights Out Mode, Fullscreen View, and Auto-Advance items. Spec: `frontend/05-culling-and-signals.md`.*

- Fullscreen, lights-out chrome by default; filmstrip; inherits the current query/arrangement/cursor (a view mode, not a separate place)
- Auto-advance on P/X/rating (toggleable, default off)
- Key-feedback overlay: big transient confirmation ("★3", "REJECT") — one of the few sanctioned color/motion moments
- Mixed-type sessions respect the current arrangement; type-batched order is one group-by away; per-type engagement via media verbs
- Signal-driven force multipliers (threshold filters, suggested rejects, burst collapse) arrive with the signals milestone (P3)

### Undo/Redo

*Depends on: command pattern, repository layer.*

- All catalog editing operations undoable: rating, tagging, labelling, collection membership, soft delete
- Command pattern with history stack (default depth 50, configurable)
- Previous state captured per-asset before bulk operations — undo restores each asset's individual prior state
- Not undoable: import, disk deletion, source management, settings changes, XMP sync
- In-memory only, does not persist across restarts
- Undo tooltip: shows what will be undone on hover ("Undo: Set rating on 23 assets")

### File Watcher Service

*Depends on: ingest pipeline, source management.*

- Local sources: OS filesystem events (FSEvents on macOS — not kqueue; inotify on Linux with per-directory watches; ReadDirectoryChangesW on Windows)
- Network sources: configurable polling interval (60s-1800s)
- Volume monitoring: detect external drive mount/unmount, auto-reconnect known drives by filesystem UUID
- Event routing (post-D20): created/modified -> ingest pipeline; deleted -> mark asset missing (never auto-remove); renamed/moved -> pending Review pair, never relinked in place. Same-path in-place edits and a missing file reappearing at its original path are the only automatic restorations
- 500ms debounce for creative app save patterns (write temp, rename)
- Cross-source content matches are always duplicates, never moves (a file cannot move between roots and keep one identity) — flagged for Review
- Graceful degradation: watcher failure is non-fatal, source degrades to polling or manual import
- Background reconciliation scan on app startup (after 2s delay) to catch changes while app was closed

### Missing File Detection and Relocate Flow

*Depends on: watcher service, ingest pipeline.*

- Assets marked "missing" remain fully browsable (thumbnails and metadata are catalog-resident) but badged (LrC-style "?")
- Missing files surface as a Review category (see Review, above), alongside the probable-move pairs the matcher records
- Relocate flow: user points to new folder, Alexandria matches by hash+size+filename, relinks in bulk
- Per D20 (2026-07-07): moves are never auto-relinked — a content match at a new path becomes a pending Review pair (original stays missing, new path minted) and the user confirms the relink

### Filter Bar and Query Pills

*Depends on: the query layer (AST→SQL). Locked 2026-07-07 (C6/C12): the query AST is the spine; a pill is the rendered form of one AST leaf. Spec: `frontend/03-search-and-filter-ux.md` + `seam/01-queries-and-commands.md`.*

- Pill row = the query, visibly (one thing on screen = one AST node); click to edit operator/value, × to remove; flat row = implicit AND
- Filterable: file type, rating (minimum), color label, flag, date range, source, dimensions, camera — any registered token type
- Advanced path: recursive AND/OR/NOT group editor, shared with the smart collection editor
- Save as Smart Collection = one click from any query state
- Arrangement controls: sort by capture date, ingest date, filename, rating, file type, file size; direction toggle; grouping is a sibling of sort (C4: arrangement never changes membership)
- **Search tiers** (every tier's output is visible pills — a misparse is correctable, never a mystery):
  1. Deterministic parser (always on, zero latency): closed vocabulary — token names/aliases, date grammar, small phrase table, the user's own tag/collection/place names
  2. Local LLM fallback — P3, off by default (see P3 entry)
  3. FTS fallback (always): unresolved words become full-text terms — NL-off degrades to baseline search, never a different system

### Status Bar and Transparency Chrome

*Part of the base UI shell. "Showing the work" is the positioning made visible — layers from ambient to nerdy, per `frontend/01-flows-and-views.md`.*

- Left zone: the current query narrated in plain words ("Sources ▸ 2024 ▸ Iceland · RAW · ★≥3 · 412 assets") — the app never shows a mystery subset
- Center zone: selection scope — count, total file size, total duration if video selected. Hidden when nothing selected.
- Right zone: compact live job/health indicator — glyph-based telegraphy in the mono face (`▁▃▆`, `◐`, watcher heartbeat dot), character-swap animation, no SVG weight
- **Activity drawer** (right zone expands): the Jobs envelope stream rendered generically — per-job progress, plain-language history ("Relinked 34 moved files · 2m ago"), detail lines
- **Dev corner** (deepest drawer tab; discoverable, not advertised): live queue depths, watcher event feed, per-stage pipeline timings — the `_scratch/sysde.md` observability wishlist
- In-grid corner ticks on assets with pending Review items

### Review

*New first-class surface from the 2026-07-07 design round — the frontend face of backend D20 (detect-and-flag, never auto-mutate). Unifies what was scattered: the Missing Files view (this tier), the Duplicate Resolution UI (formerly P2), XMP conflicts, import errors. Spec: `frontend/06-review.md`; backend projection specced in `backend/impl/DEFERRED.md` §5.*

- One task view collecting everything the engine noticed but refused to decide: probable moves/renames, duplicates, missing files, XMP conflicts, import errors, suggested rejects (when signals land)
- Sidebar item with count badge — ambient, never modal; opening it is a full-window task view (C3)
- Categorized list processed top-to-bottom, keyboard-forward, zero-when-done; bulk resolution is the norm ("confirm all 34 relinks")
- Per-category row UX: moves show old path → new path with confirm-relink; duplicates get side-by-side metadata comparison (keep both / remove one / link as group); missing files get the relocate flow; XMP conflicts show both sides + the policy that would apply
- Resolution actions are the user-granted versions of what D20 removed from the engine; catalog-editing resolutions ride the command pattern (undoable) where possible
- **Deferred: automation rules** ("always relink moves when hash+size+name match") — wait for Review v1 usage to show which repetitions hurt. Reserved now: automation is a user grant, never an engine default; rule conditions reuse the query token vocabulary

### "Previous Import" Collection

*Depends on: import pipeline, collections.*

- Auto-created/updated at import completion
- Shows the assets from the most recent import as a clean, complete set for review

### Catalog Backup

*Depends on: catalog infrastructure. The backup-before-migration floor is owned by the app-host milestone (`backend/impl/12-app-host.md`); the backup system proper is an unscheduled design task (`backend/04-open-questions.md` #16).*

- Must use SQLite backup API or `VACUUM INTO`, never raw file copy (raw copy of open SQLite = corruption)
- Automatic backup before every migration
- Configurable rolling backups (default: keep last 10)

### Logging and Observability

- Backend: structured JSON logs via slog, rotated at size limit
- Dev mode: colored text handler with component source indicators
- Frontend: logger module that batches entries to backend, so one log file tells the whole interleaved story
- Global capture: window.onerror, unhandled rejections, error boundary catches, API errors
- User-facing: Help -> "Export logs" action (backend zips log directory to user-chosen location)
- Rich but concise and readable

### i18n Infrastructure

*Not translations — just the mechanism so strings don't accumulate as literals.*

- react-i18next with flat JSON locale files
- English ships; mechanism is day-one so retrofitting isn't needed
- Stable key identifiers namespaced by feature
- Dates/numbers/file sizes via Intl API, not locale catalogs
- No string concatenation in code (must be extractable to resource files)

### Error Boundaries

- App-level crash screen with restart hint, copy error details, export logs
- Per-pane error boundaries: browser, main region, inspector crash independently without taking down the whole app
- "Reload panel" recovery via key bump

### Persisted Layout State

- Sidebar widths, collapsed/expanded state, grid zoom/density, current view — persist between sessions via localStorage

---

## P2 — Differentiation and Power Features

These elevate Alexandria above competitors. Build after P0/P1 is solid.

### Asset Grouping (RAW+JPEG+XMP)

*Highest user value for creative professionals with RAW+JPEG workflows. Schema is ready from day one.*

- Files sharing same base filename in same directory auto-grouped at ingest (e.g. `IMG_1234.CR3` + `IMG_1234.JPG` + `IMG_1234.xmp`)
- Group renders as single card in grid, expandable to show members
- Each member has a role: raw, jpeg_sidecar, source, export, member
- XMP sidecars always attached to their RAW, never shown as standalone
- Preview priority chain: LrC-exported JPEG > camera companion JPEG > DNG updated preview > embedded RAW preview
- Thumbnail re-evaluation when XMP sidecar changes
- Commands: group, ungroup, set cover asset
- Manual stacking: user-initiated "collapse these into one card" operation, distinct from auto-grouping

### Smart Collections

*Enables powerful filtering workflows. Schema is ready.*

- Stored query (the versioned AST, C6), membership computed dynamically — a saved query promoted into the scope tree, the deliberate bridge between filter and scope
- Fully nested boolean conditionals (AND/OR/NOT groups) — significantly more powerful than LrC's flat match-all/match-any
- Criteria support: string/numeric comparisons, contains, starts with, is empty, date ranges, etc. — any registered token type
- Save as Smart Collection = one click from any filter-bar query state; the recursive condition group editor (shared with the filter bar's advanced path) is for refinement
- Built-in system smart collections: Untagged, Unrated, Not in any collection, Import errors (later: Suggested rejects, from signals)

### Custom Color Labels

*From todo.md — extends the fixed 6-color set.*

- User-defined labels, indeterminate number, each with custom color (full color picker, not fixed palette)
- Each label has user-defined name and color
- Stored per-catalog (label sets can differ across catalogs)

### Tag Colors

*From todo.md.*

- Optional customizable color per tag with full color picker
- Optional color inheritance to child tags

### Thumbnail Auto-Refresh on External Edit

*From todo.md — addresses digiKam's most-reported complaint.*

- When watcher detects mtime change on source file, queue thumbnail regeneration (not just catalog record update)
- Edited PSD or touched RAW reflects updated embedded preview in grid

### Configurable Grid Card Overlays

*From todo.md.*

- User chooses which fields/badges appear on grid cards: rating stars, color label, file type, GPS indicator, clip duration, filename, capture date
- Stored in settings per view

### Compare View Mode

*Formerly "Side-by-Side Comparison Mode"; now the C view mode per the state equation (see View Mode System, P0). The filmstrip item that used to sit here was absorbed into Loupe's P0 definition.*

- Select 2-4 assets, view at equal size
- Ratings, labels, flags accessible without leaving the view

### Collection Identity

*From todo.md.*

- Selectable cover image per collection
- Free-text description
- Auto-computed date range from assets

### Grid Grouping

*From todo.md.*

- "Group by" mode: collapsible sections by file type, date (year/month/day), source, rating, color label
- View mode, not structural change

### Asset/Group Pinning

*From todo.md, 2026-07-07.*

- Lets certain high-demand assets surface first at the top of the grid regardless of active sort
- Scope: collection-relative ("important in this album"), not global — pin state lives on the asset<->collection relationship, not the asset itself
- Pinning a single asset vs. an asset group is a real UX fork worth deferring rather than resolving speculatively (does the whole group move as a unit, or does pinning one representative just surface the group without reordering members)
- Needs its own explicit order among multiple pinned items (e.g. drag-to-reorder) — otherwise "pinned" degrades into an unordered clump and loses the point
- Likely only meaningful in a dedicated "custom order" view mode, silently suppressed the moment a real sort criterion is picked, rather than fighting every sort mode

### Copy/Paste Metadata

*From todo.md — high value for multi-camera shoots.*

- Copy metadata from source asset (rating, tags, color label, custom fields)
- Paste onto target assets

### Configurable "Open In" Programs / External Program Registry

*From todo.md; expanded 2026-07-07 during export-flow design.*

- Per MIME type / extension, seeded from OS defaults on first run (macOS Launch Services, `xdg-mime`/desktop files on Linux, similar-but-distinct Windows APIs)
- User-overridable in settings, stored in machine.json
- Right-click -> Open In -> [app list] with "Set as default"
- **Registry extension** (same "one explicit table documents what's supported" shape as the `TypeHandler` registry): per-program row holds display name, icon, matched file types, and capability flags — batch-launch-safe (accepts multiple file args in one launch) and has-scripting-automation (AppleScript/Shortcuts available, confirmed per-program, never assumed)
- Name/icon/default-app are OS-queryable; capabilities are not — capabilities live in a small hand-curated overlay list on top of the OS-discovered base data, safe default (single-file-only, no automation) for anything not curated
- Design intent: generic OS-seeded "open in" is the floor for every program; a curated few (the programs people actually use most) get genuinely bespoke, deep integration on top — the hand-off should feel tight and intentional for those, not uniformly lowest-common-denominator
- Batch dispatch: multi-select N assets -> consult registry's batch-launch-safe flag -> one process launch with all N paths if safe, otherwise fall back to opening just the first / warning the user

### Import Task View Options

*From todo.md; reframed 2026-07-07 — Import is a full-window task view (source pick, options, live pipeline progress, completion summary), not a modal. Task views never touch catalog view state (C3).*

- Auto-create named collection from import (pre-filled name, checked by default)
- Skip suspected duplicates (checked by default, count shown post-import)
- Apply saved metadata preset to all imported assets (copyright, creator, rights)

### Metadata Presets

*From todo.md; general feature, import modal above is one consumer of it.*

- A preset is a named, saved partial set of metadata fields+values (e.g. copyright, creator, rights), applicable later to a single asset or a batch selection, not just at import time
- Conceptually "a saved partial patch with a name" — reuses the batch metadata-edit path, no separate update machinery
- Open design decision, needs to be made deliberately before building: conflict behavior when a target asset already has a value in a field the preset touches — overwrite unconditionally (simple, matches most other apps' default, occasionally surprising), skip fields with an existing value, or prompt on conflict

### Bulk Write Metadata to XMP

*From todo.md — LrC's "Metadata -> Save Metadata to Files" equivalent.*

- One-shot action and scheduled/automatic option
- Writes catalog metadata (ratings, tags, color labels, notes) to XMP sidecars
- For selected assets or entire catalog

### Catalog Backup Improvements

*From todo.md.*

- Smart retention policy: all from last 24h, one/day for last week, one/week for last month, one/month beyond
- Multiple backup destinations (up to N, each with own retention)
- Graceful skip if destination unavailable

### Configurable Ignore List

*From todo.md.*

- Glob patterns and/or extensions to skip during import (.DS_Store, Thumbs.db, .tmp, *.lrprev)
- Global defaults ship with sensible exclusions, user can add/remove
- Checked at scanner level before any processing

### Drag and Drop

- Assets from grid onto collection/tag in sidebar to add membership
- Drop files onto app window to trigger import (with source selection UX)

### Auto-Detect Removable Volumes

*From todo.md.*

- Toast notification on card/drive insert ("Card detected — import?")
- User can dismiss or click to start import

### OS-Level Notifications

*From todo.md.*

- Desktop notifications for background operations: import complete, integrity check found issues, backup failed
- Respects system Do Not Disturb

### Mixed State Indicators in Batch Editing

*From todo.md.*

- When selection has conflicting field values, show mixed state indicator (not blank)
- Applying a value from mixed state applies to all selected

### Metadata Editing

*From todo.md — read and edit commonly used metadata fields as first class.*

- Writing back via exiftool for most formats
- RAW files: write to XMP sidecar only (never modify the raw)
- Proprietary formats (PSD, AI, INDD, Affinity): read-only for metadata
- Field sets: EXIF (capture datetime, camera, lens, aperture, shutter, ISO, focal length, GPS, flash, metering, WB, color space), IPTC/XMP (title, caption, creator, copyright, usage rights, location), Audio (title, artist, album, track, year, genre), Video (title, description, copyright, creation date)

### Time Offset / Timezone Correction

*From todo.md — must happen before GPX correlation for timestamp accuracy.*

- Quick action on asset selection to shift capture timestamps by fixed offset
- Covers: wrong timezone, DST, multi-camera clock drift
- Writes corrected time to XMP/EXIF via exiftool

### Adaptive Inspector by Asset Type

*From todo.md.*

- Image, video, audio, document, font, GPX track each get tailored inspector layout
- No showing empty camera EXIF panels for audio files
- Asset groups get separate inspector showing group structure and per-member details

### Worker Pool Size Controls

*From todo.md.*

- User-configurable counts for hash, metadata, thumbnail workers
- Two presets: "performance" and "lightweight" plus manual sliders
- Stored in machine.json

### Recently Used Prioritized

*From todo.md.*

- Tag picker, collection picker, open-in app menu, search suggestions — all prioritize recently used items

### Asset Counts in Sidebar

*From todo.md.*

- Count badge next to every collection, folder, and source

### Home View and Bookmarks

*From todo.md; shaped 2026-07-07 as the optional landing view.*

- Bookmark collections and sources for quick access
- Home view: minimal — recently used collections, saved queries, calls to action (import, find), Review count. Not a dashboard, no greeting
- User-disableable — Alexandria works for them, not the other way around

### Sort Options

*From todo.md.*

- Both ingest date (when Alexandria imported) and capture date (when shot) exposed as sort options

### Sequence Number Rollover Handling

*From todo.md.*

- Detect camera rollover (IMG_9999 -> IMG_0001)
- Sort by capture timestamp rather than filename when timestamps available

### UI Color Scheme

*From todo.md.*

- Selection beyond just dark/light — neutral grey is important for color-sensitive work and should be the default
- Custom background image/color
- Custom glassmorphism opacity

### Clipboard Support

- Copy selected asset(s) to system clipboard as file reference (file URL, not rasterized preview)
- Paste into Resolve, InDesign, Finder, etc.
- Per-platform implementation (NSPasteboard on macOS, xclip/wl-clipboard on Linux)

### Duplicate Source Detection

- Same content hash across different sources surfaced to user
- Detection via existing content hash column — no extra ingest cost
- Resolution: keep both, remove one, merge metadata

### LrC Catalog Bootstrap Import

- One-time import from .lrcat file: collections structure, keyword hierarchy, asset file paths
- Main win is collections (XMP doesn't carry them)
- One-shot only — no ongoing sync against live .lrcat
- Import wizard: select file, preview what will be imported, confirm

---

## P3 — Post-Stabilization

Build after the core product is stable and well-used.

### Signals Milestone (ENRICH Stage + Enrichment Jobs)

*Architecture locked 2026-07-07 (C11: models propose as data, the user disposes via the query). Every signal is a metadata column exposed as a query token type — no verdicts, no opaque scores driving hidden behavior. Engine spec: `backend/06-signals-and-enrichment.md`; UX: `frontend/05-culling-and-signals.md`. Subsumes the former standalone "Perceptual Hash" and "Technical Quality Scoring" entries.*

- **Cheap signals ride ingest** (the ENRICH pipeline stage, on the in-memory thumbnail): sharpness (Laplacian variance), highlight/shadow clipping % (histogram), phash — there when the user sits down to cull, no waiting
- **Heavy signals are background enrichment jobs**, never pipeline stages: blink/eyes-closed probability, face count/quality, embeddings (P4). Priority is work-follows-attention: opening a working set bumps its pending enrichment; within a set, sharpest first
- Every signal column is deletable + recomputable via a registered rebuild (derived-state rule); shipping a new signal or model upgrade = backfill job over the existing catalog
- Filtering on a still-computing signal annotates the pill honestly ("sharpness > 0.5 · 214 not yet scored"), results streaming in
- phash: duplicate and near-duplicate detection via hamming distance

### Cull Force Multipliers (built on signals)

- **Burst/stack collapse** — phash clusters render as stacks, pre-sorted within by sharpness + eyes-open, best frame as cover; cull representatives, expand only when contested. Rides the asset-group machinery (P2)
- **Suggested rejects — never auto-rejects**: below-threshold frames get a suggested state (dimmed in filmstrip, collected in a "Suggested rejects" system smart collection), confirmed in bulk. The model drafts; the user signs
- **Threshold filters**: "when culling, only show sharpness > 0.5" is a pill on the cull session's query; saveable
- Auto-grouping opt-out: ask once, respect forever

### Natural-Language Query Fallback (local LLM)

*Tier 2 of the search system (`frontend/03-search-and-filter-ux.md`); the deterministic parser (tier 1) ships with the query layer and covers most real input. Off by default until built; parked until the deterministic parser has usage.*

- Only for the unresolved remainder: paraphrase/synonyms, fuzzy time ("a couple summers ago")
- Schema-constrained to emit only valid AST against the token vocabulary; candidate tag/place names injected as a shortlist
- Latency budget 0.5–2s — NL is a deliberate act, never the hot path; typed pills stay instant and model-free
- Output is visible pills (C12); disabling NL degrades to FTS baseline, not a different system

### Dominant Color / Palette Extraction

- 5-8 dominant colors per asset at ingest via k-means/median cut on thumbnail
- Stored as hex values
- Color picker in filter bar; proximity search using deltaE in LAB colorspace (not RGB)
- Video: sample keyframes, union palettes

### GPX Track Correlation

- Import GPX file, correlate with photos/video by timestamp
- Configurable tolerance window (default +/-30s)
- Timezone offset handling between camera clock and GPS device
- Write GPS coordinates to assets via exiftool
- Preview dialog showing how many assets would be geotagged

### Integrity Check Service

- Periodic or on-demand: verify source files exist and content hash matches
- Report: N verified, N missing, N changed, N moved
- Actions: re-link moved files, remove missing from catalog, re-ingest changed
- Scheduled runs: configurable (nightly, weekly)

### Catalog Health Dashboard

- Passive monitoring: status bar warning indicator + toasts for urgent issues
- On-demand health panel with traffic-light indicators per category:
  - Database: integrity check, WAL state, schema version
  - Files: missing files, changed files, orphaned thumbnails
  - Metadata: assets without XMP, unresolved XMP conflicts
  - Organization: untagged, unrated, not in any collection (informational, not errors)
  - Duplicates: pending detections count
  - Backups: last timestamp, schedule adherence, destinations reachable
  - Sources: offline sources, high missing-file proportion
- Never auto-fixes without user confirmation

### Video-Specific Features

- **Waveform thumbnails:** visual waveform as grid card thumbnail for audio/video via ffmpeg
- **Clip duration on grid cards:** show duration directly on video/audio thumbnails
- **VFR detection:** flag variable frame rate clips at ingest, badge on grid card, warning in inspector
- **DaVinci Resolve codec compatibility:** detect incompatible codecs at ingest, badge, offer transcode
- **Frame rate mismatch indicator:** per-clip badge and collection-level notice for mixed frame rates
- **Audio channel indicator:** stereo/mono/no audio on video assets
- **Timestamped video clip annotations:** notes tied to timecodes ("good b-roll at 0:23"), export to xmpDM:markers for Premiere interop

### Export Pipeline

*Scope note (2026-07-07): this pipeline handles flat/already-rendered images (format conversion, resize, metadata strip) via ffmpeg/ImageMagick — it is explicitly NOT a Lightroom-Develop-module replacement. RAW development/export is a different problem entirely; see "RAW Export via External Tool Dispatch" below.*

- Format selection: JPEG, PNG, TIFF, WebP
- Resize options: fit width, fit height, exact dimensions, percentage
- Quality slider, color space selection
- Filename template system ({date}, {camera}, {sequence}, {original_name})
- Output folder picker, metadata strip options
- Batch export with non-blocking progress UI
- Export ordering: maintain sequence order (LrC's multi-threaded export doesn't preserve order)

### RAW Export via External Tool Dispatch

*From 2026-07-07 design session; full detail in `_project-tracking/design/raw-export-dispatch.md`, summarized here.*

- Non-destructive RAW edits (masks, AI denoise, adjustments) are instructions meaningful only to the engine that wrote them — Alexandria cannot build its own RAW rendering engine at parity with dedicated tools, and isn't going to try
- `Export` becomes a third `TypeHandler` capability (alongside `Metadata`/`Thumb`): raster formats get direct ffmpeg/ImageMagick dispatch (Alexandria's own pipeline above); RAW formats resolve via a lookup into the external-program registry for whichever tool is associated with that RAW extension, gated on that tool's has-scripting-automation flag
- No automation surface available -> graceful degradation (same principle as any nil `TypeHandler` capability): surface a clear message telling the user to export from that program directly, never a crash or silent no-op
- Mixed batches (assets developed across multiple different tools) must group dispatch by owning tool, not treat the selection as homogeneous
- Job modeling reuses the Jobs-registry/progress-bar UX, but deliberately does NOT reuse the `dependency` fleet's subprocess supervision (kill-on-timeout, semaphores) — a GUI app the user is actively using has a different risk profile than a headless tool Alexandria fully owns
- Still open: exactly how "which tool rendered this asset's edits" gets tracked (distinct from the "open in" default-app mapping)

### In-App Asset Converter

- Lightweight format conversion: PNG->JPEG, HEIC->JPEG, PNG->ICO, MP4->GIF, etc.
- Right-click -> Convert -> pick format
- Output: replace, alongside original, or choose folder
- Shares ffmpeg/ImageMagick subprocess machinery with export

### Batch Rename

- Rename selected files on disk with template ({date}_{camera}_{sequence})
- Preview before apply, confirmation, undo
- Updates filesystem and catalog atomically

### Font Viewer

- Dedicated inspector for TTF/OTF/TTC/WOFF/WOFF2
- Font rendered at multiple sizes/weights, full glyph map
- Live preview field for arbitrary text
- Multi-font comparison (Google Fonts-style)
- Pure Go via golang.org/x/image/font/sfnt — no subprocess needed

### DaVinci Resolve / After Effects Project Support

- Parse project files to extract referenced asset paths
- Link to catalog assets
- Inspector shows "used in projects: X, Y, Z"
- New `project_references` table

### Smart Bracket / Burst / Panorama Detection

*Depends on: asset grouping.*

- Auto-detect HDR brackets, focus stacks, panorama sequences, burst sequences from EXIF patterns
- Confidence scoring, only auto-group above threshold
- UI for reviewing detected groups before confirming

### On-Device Whisper Transcript Search

- Whisper (small model, fully local) on video/audio with speech
- Transcript indexed in FTS5
- Search "find the clip where I'm explaining the crux"
- Transcript panel in video inspector with timecodes
- Opt-in with clear processing time communication

### Map View

- MapLibre GL JS (open source, OpenStreetMap tiles, no Google dependency)
- Clustered pins for geotagged assets
- GPX track overlay as polyline
- Time scrubber for temporal navigation
- Offline tile caching for areas with geotagged assets
- Click pin -> select asset; click cluster -> zoom or show strip
- Reverse geocoding via offline gazetteer (GeoNames) at ingest for searchable place names

### Analog Photography Support

*Depends on: asset grouping.*

- Scanned negative/positive grouping via filename heuristics
- Analog camera/lens EXIF override (film scans carry scanner's EXIF, not actual camera)
- Saveable analog camera presets ("Nikon F3 + 50mm f/1.4 Nikkor") batch-applicable to entire rolls

### Accessibility (Full)

*Foundational keyboard navigation is P0. Full accessibility is here.*

- Screen reader support (VoiceOver, Orca, NVDA/Narrator)
- ARIA attributes throughout UI component tree
- WCAG AA color contrast in all themes
- Color labels must have shape/pattern alternative for color-blind users
- `prefers-reduced-motion` support

### Localisation

- Non-English language support
- Translation management tooling
- Variable string length accommodation in UI

### Telemetry / Crash Reporting

- Explicit opt-in, never opt-out. Never enabled silently.
- Live preview of what would be sent before enabling
- Self-hosted backend (Plausible or Posthog)
- Publicly documented event schema
- Collects: anonymous feature usage events only
- Never collects: file paths, filenames, metadata values, tag names, GPS coordinates, anything identifying

---

## P4 — Far Future / Maybe Someday

Ideas worth preserving but with no committed timeline.

### AI/ML Tagging and Semantic Search

- CLIP-based image+text embeddings for auto-tagging and natural language search
- Fully on-device (no API calls) — model (~600MB) downloaded on first opt-in
- sqlite-vec extension for vector storage and ANN search
- Video: frame sampling via ffmpeg, scene-level tags, independently opt-in from photo tagging
- Face regions: face_regions and persons tables
- UI: confidence scores, correction workflow, model management

### Catalog Server Mode (Multi-Machine Access)

- Run Alexandria as server process on NAS, desktop clients connect over LAN
- Same pattern as Plex
- Required because SQLite WAL mode is incompatible with network filesystems (documented corruption risk)
- Significant architectural departure — v3+ consideration

### Lightweight In-App Editing

- Text/Markdown editor with syntax highlighting
- Image crop/rotate/flip (non-destructive where possible, lossless JPEG rotate via jpegtran)
- Explicit non-scope: no RAW processing, no layer editing, no color grading

### In-App Auto-Updater

- Download and install updates within the app
- Per-platform download/verify/replace, code signing, elevation handling

### Mood / Palette Board Per Collection

*Depends on: dominant color extraction.*

- Aggregate palette across all assets in a collection
- Color distribution strip or swatch grid
- Comparison mode: two collections side by side

### Shooting Statistics

- Stats derived from EXIF data: lens usage, focal length distribution, aperture distribution, time-of-day patterns, shutter count per body
- Charts/histograms UI

### Preview LUT for Log Footage

- Associate viewing LUT per camera source (S-Log, LOG-C, V-Log)
- Applied in loupe for preview only, stored footage stays untouched

### Focus Peaking + Highlight/Shadow Clipping in Loupe

- Highlight sharp areas in configurable color
- Overlay blown highlights / crushed shadows as colored warnings

### Virtual Copies

- Same physical file, multiple independent catalog entries with different metadata/ratings/collection membership

### Usage Rights / Licensing Tracking

- Per-asset license records: licensed to, usage terms, territory, expiry
- Expiry alerts, smart collection for "expiring within 30 days"

### Content Planning / Shot List

- Create shot list attached to collection before a trip
- Post-ingest coverage tracking: which shots captured vs planned

### Before/After Comparison View

- Side-by-side loupe of RAW and edited counterpart with split-drag handle

### Client Delivery Workflow

- Mark collection as client project, select picks, generate watermarked proof or full-res delivery
- Track delivery status

### Watermarking at Export

- Text or image watermark at export time, configurable position/opacity/size

### Subtitle / Caption Management

- SRT/VTT files alongside video assets
- Preview video with subtitles in loupe

### Sensor Dust Spot Detection

- Detect recurring bright speck at same pixel position across frames from same camera

### Audio Library with BPM / Key / Mood

- BPM and key extraction via ffmpeg/aubio
- Mood/genre tagging, project usage tracking, license expiry

### Garmin Connect API Integration

- Auto-pull recent GPX tracks from Garmin Connect (OAuth, opt-in)

### Plugin / Extension System

- **Permanently deferred.** Explicitly decided against. Contributors add features via code contributions or feature requests. The API surface maintenance burden is not justified.

---

## Open Product/Architecture Questions

*Carried over from todo.md — genuine unresolved questions, not settled features. Distinct from the implementation-deferred ledger (`_project-tracking/backend/impl/DEFERRED.md`) and the architectural decision log (`_project-tracking/backend/02-decision-log.md`); these haven't reached a decision yet.*

- ~~Frontend state management~~ — **RESOLVED (2026-07-07 frontend design round):** no state
  library; one reducer holding the state equation (`view state = viewMode(query + arrangement,
  selection + cursor)`, CONSTANTS C2), server state in TanStack Query, everything else derived.
  See `_project-tracking/frontend/02-state-model.md`.
- Map view / location search: how do we generalize raw GPS coordinates to a town or area name someone would actually search for? (Reverse geocoding via offline gazetteer is the current lead — see Map View, P3 — but the UX of "searchable place" from a lat/lng pair isn't fully worked out.)
- Responsiveness/scaling system: desktop-only, no mobile/tablet target, but still want a system that gives grid/layout scaling without hand-rolling it — which existing tool/approach fits that constraint without the overhead of a full responsive framework aimed at screen sizes that will never be supported here?

---

## Technical Build Order

Within each priority tier, features should be built roughly in this order based on dependencies:

### P0 Build Order
1. Domain model and types (`internal/domain/`)
2. SQLite catalog, schema, migrations, settings
3. Source management (add, scan config, drive identity)
4. Ingest pipeline stages (scanner -> hasher -> dedup -> extractor -> thumbnailer -> writer)
5. Asset repository and query layer
6. Frontend shell (CSS Grid, tokens, themes)
7. Browser/sidebar with tree component
8. Grid view with virtualization and selection
9. Inspector panel (read-only first)
10. Filter bar and search (FTS5)
11. Loupe/detail view
12. Open in external app
13. Basic organization (rating, labels, flags, tags, collections, notes)
14. Startup sequence

### P1 Build Order
1. Undo/redo (command pattern) — enables safe editing
2. Keyboard system + command palette (action registry, dispatcher, configurable bindings, preset sets — the palette is the registry's face, same increment)
3. Filter bar + query pills (rides the backend query layer; deterministic parser tier)
4. XMP sync (read first, write second, conflict resolution third)
5. Watcher service (local watching first, then volume monitoring, then network polling)
6. Review v1 + missing-file/relocate flow (the projection + resolution actions per DEFERRED §5)
7. Cull view mode (fullscreen triage; signal force-multipliers arrive later with P3 signals)
8. Status bar and import progress UI (transparency chrome layers)
9. Logging infrastructure (backend + frontend bridge)
10. i18n mechanism
11. Catalog backup system
12. Error boundaries
13. Previous Import collection
14. Layout persistence

### P2 Build Order
1. Asset grouping — highest standalone value, unblocks several P3 features
2. Smart collections — high workflow value, backend query builder
3. Custom color labels + tag colors — small schema, high daily-use value
4. Thumbnail auto-refresh — builds on watcher, high annoyance-removal value
5. Metadata editing — builds on exiftool write path, needed for several later features
6. Import modal options, configurable ignore list — small but high-value workflow improvements
7. Grid overlays, Compare view mode, adaptive inspector — UI polish that compounds
8. Clipboard, drag-and-drop, copy/paste metadata — interaction refinements
9. Duplicate source detection — leverages existing detection; resolution rides Review (P1)
10. Catalog backup improvements — reliability
11. LrC catalog bootstrap — migration onramp for new users
