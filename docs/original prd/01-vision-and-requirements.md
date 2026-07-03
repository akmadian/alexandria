# Vision & Requirements

## The problem

Creative professionals accumulate vast archives of files across many locations: local SSDs, external drives, NAS devices, old hard drives in a drawer. These files span many formats — RAW photos, PSDs, videos, Illustrator files, Affinity documents, exported JPEGs, InDesign layouts. Over time, the archive becomes opaque. Files are hard to find, hard to know about, hard to organise without opening each one individually.

Existing tools fail this user in specific ways:

- **Photo-only tools** (digiKam, Darktable, Damselfly) don't understand creative files like PSD, AI, INDD, AFPHOTO
- **Enterprise DAMs** (ResourceSpace, Pimcore, Phraseanet) are designed for teams, have dated web UIs, and are painful to self-host
- **Lightroom** comes closest for photos but is subscription-locked to Adobe, has no support for non-Adobe creative formats, and its catalog is a black box
- **Nothing** in the open source space has a UI a designer would actually enjoy using

The gap: a **local-first, cross-platform, modern-UI DAM** that handles the full creative file stack. It does not exist.

## Target user

Solo creative professional. Photographer, designer, videographer, or generalist. Has:

- Thousands to hundreds of thousands of files
- Files spread across multiple drives and locations
- A mix of file types including proprietary creative formats
- Strong opinions about folder structure (does not want an app reorganising their files)
- Likely already uses Lightroom Classic for photo work
- Works on Mac primarily, possibly Linux, occasionally Windows

The core need is: **find, see, and open**. Not edit, not publish, not collaborate.

## What makes Alexandria different

| Capability | digiKam | ResourceSpace | Lightroom | Alexandria |
|---|---|---|---|---|
| Local-first, no subscription | ✓ | ✓ | ✗ | ✓ |
| Works offline (drive disconnected) | Partial | ✗ | Partial | ✓ |
| Modern UI | ✗ | ✗ | ✓ | ✓ |
| Creative file support (PSD, AI, AFPHOTO) | ✗ | Partial | Partial | ✓ |
| Video support | Partial | ✓ | Partial | ✓ |
| Cross-platform (Mac + Linux) | ✓ | ✓ | ✓ | ✓ |
| GPL open source | ✓ | ✓ | ✗ | ✓ |
| Reference model (doesn't move files) | ✓ | ✓ | ✓ | ✓ |
| Lightroom XMP interop | ✗ | ✗ | N/A | ✓ |

## Reference vs managed

Alexandria is a **reference DAM**. It indexes files where they live. It does not copy, move, or reorganise them. The user remains in full control of their folder structure.

This is the right model for a power user with an existing opinionated archive. The alternative — a managed library like Apple Photos that owns the files — would require the user to surrender control of their archive to the app. That is a non-starter.

## Supported file types

Support means: thumbnail generation, metadata extraction, display in grid, open-in-app handoff. Alexandria does not edit files.

| Category | Formats | Approach |
|---|---|---|
| JPEG / PNG / TIFF / GIF / BMP / WebP | Standard raster | Native Go image libraries |
| RAW | ARW, CR3, CR2, NEF, DNG, RAF, ORF, RW2, and 700+ others | Extract embedded preview via `exiftool`; fall back to `dcraw_emu`/libraw CLI for full decode |
| PSD (Photoshop) | .psd | Extract embedded composite (ImageMagick CLI or Go psd library) |
| Illustrator | .ai | Treat as PDF (modern .ai files are PDF-compatible), Ghostscript CLI for preview |
| InDesign | .indd | Extract embedded preview from file header |
| Affinity (.afphoto, .afdesign, .afpub) | Proprietary, no SDK | Extract embedded preview thumbnail from file header |
| Video | MP4, MOV, AVI, MKV, ProRes, and common containers | `ffmpeg`/`ffprobe` CLI for thumbnail frame and stream metadata |
| Audio | MP3, WAV, AIFF, FLAC | `ffmpeg` CLI for metadata, waveform thumbnail |
| Vector | SVG | Rasterise for thumbnail |
| PDF | .pdf | First page preview via Ghostscript CLI or PDF library |
| Markdown | .md | Treated as a document: text-preview thumbnail, content in full-text search |

**External tools are invoked as subprocesses, not cgo bindings.** `exiftool` (batch `-stay_open` mode) and `ffmpeg`/`ffprobe` are bundled with (or resolved by) the app. Subprocesses sidestep cgo cross-compilation pain, and a tool crashing on a corrupt file kills the subprocess, not Alexandria — which is exactly the per-file-error-and-continue behaviour the ingest pipeline wants. Adding support for a new file type means adding a MIME/extension → dispatcher mapping and, at most, a new extractor/thumbnailer implementation — never a pipeline change.

**On Affinity formats specifically:** Serif has not published an SDK for .afphoto/.afdesign/.afpub. These files contain an embedded preview thumbnail in the file header. Alexandria extracts this embedded preview — the same approach used by NeoFinder and XnView. This is the ceiling for any third-party tool without an SDK. It is good enough for a DAM: the user can see what the file is and open it in Affinity.

**On InDesign specifically:** .indd files embed a preview rendered at last-save time. Alexandria extracts this. It does not parse the InDesign format.

## Platform support

- **macOS:** First-class. Primary development target.
- **Linux:** First-class. Full feature parity intended.
- **Windows:** Supported. Third priority. Some platform-specific behaviours (drive identity, file watching) will have Windows implementations but may lag Mac/Linux.

## Functional requirements

### Core catalog

- Catalog-first architecture: the database is the source of truth, not the filesystem
- Reference model: files stay where the user put them, Alexandria never moves or copies them
- Assets remain browsable and searchable when their source is offline (drive disconnected, NAS unreachable)
- Only "open original" is disabled when a source is offline — all other operations (browse, search, tag, rate, add to collection) work normally
- Soft delete: removing an asset from the catalog requires explicit user action and shows a confirmation modal with two options — "remove from catalog" vs "delete from disk". The latter is protected by an additional warning.
- Physical file deletion from disk cannot be undone by Alexandria. The user is clearly warned.

### Sources

- A source is a watched root: a local folder, an external drive mount, or a network share (SMB/NFS)
- Multiple sources are supported
- Each source has a configurable scan behaviour (recursive or not)
- Network sources use polling rather than filesystem events (see Watcher Service docs)
- External drives are identified by filesystem UUID, not volume label or mount path, so they reconnect automatically when plugged in regardless of what mount point the OS assigns
- Network shares are identified by host + share name. The user is responsible for keeping these stable (static IP or stable hostname on the NAS). This is an intentional simplicity tradeoff.
- Sources can be marked inactive without being removed from the catalog
- When a source goes offline, its assets remain in the catalog. When it comes back online, Alexandria runs a reconciliation scan to catch changes.

### Ingest

- Manual trigger only (v1). User clicks Import, selects a source, Alexandria scans and indexes it. No background auto-import without explicit user action.
- The app stays fully usable during import. Progress is shown in a persistent, non-blocking progress panel (LrC-style corner indicator with counts and current stage). WAL mode means the grid can be browsed and queried while the pipeline writes.
- Imported assets appear in the grid incrementally as batches commit. Because thumbnails and metadata are generated *before* the catalog write (pipeline ordering), an asset is always fully rendered the moment it appears — no placeholder/blank-card state like LrC shows.
- A "Previous Import" collection (like LrC's) collects the assets from the most recent import. It is revealed/updated when the import completes, so the user gets a clean, complete set to review rather than watching it trickle in.
- Import is cancellable at any point.
- Import is idempotent: re-running import on the same source is safe. Files that haven't changed (same mtime and size) are skipped. No duplicate assets are created.
- At the end of import, a summary is shown: N added, N updated, N skipped, N errors. Errors include file path and reason.
- Import extracts and stores thumbnails and metadata eagerly — not lazily — so assets are fully browsable immediately after import, including when offline.

### Thumbnails

- Generated at ingest time and stored in the Alexandria application data directory, keyed by asset ID
- Stored separately from source files so they survive the source going offline, and survive file moves (asset ID doesn't change when a file moves)
- The thumbnail cache is considered rebuildable — it is not a critical backup target. This should be documented clearly to users.
- Thumbnail generation is dispatched per MIME type to the appropriate generator

### Metadata

- Metadata is extracted at ingest time and stored denormalized in the assets table for fast querying
- Fields that are commonly filtered or sorted on (file type, rating, color label, capture date, camera model, etc.) are stored as dedicated columns
- Extended or format-specific metadata that is not commonly queried is stored as a JSON blob
- EXIF, IPTC, and XMP metadata are all extracted where present
- Metadata is not written back to source files unless XMP sync is explicitly enabled (see XMP Sync docs)

### Organisation

- **Tags:** Hierarchical. A tag can have a parent tag, enabling structures like `Photography > Portrait > Headshot`. Tags can be applied to any asset regardless of file type.
- **Color labels:** Six labels: Red, Orange, Yellow, Green, Blue, Purple. Stored on each asset. Synced to/from XMP where applicable.
- **Star ratings:** 0–5. Stored on each asset. Synced to/from XMP where applicable.
- **Flags:** Pick or Reject. Stored on each asset. For triage workflows.
- **Notes:** A free-text note per asset. Included in full-text search. Synced to/from XMP `dc:description` (Lightroom's Caption field) when XMP sync is enabled.
- **Collections:** Manual collections (user curates membership). Collections can be nested. Smart collections (stored query, membership computed dynamically) are schema-ready but the query builder UI is **deferred to P1** — see Deferred Features.
- **Filesystem view:** The sidebar also exposes a filesystem tree view mirroring the actual folder structure of indexed sources. Assets can be browsed by their physical location.
- **Asset groups (P1, deferred):** Related assets — a RAW file and its exported JPEG, a PSD and its exported PNG — can be grouped. In the grid, the group renders as a single card (showing the cover asset). Group members have roles (raw, jpeg_sidecar, source, export, member). This is a P1 feature, deferred from v1, but the schema accommodates it from day one.

### Search

- Search on: filename, file type, tag, rating, color label, flag, capture date range, source, dimensions, duration, camera make/model
- Full-text search via SQLite FTS5 covers filename, camera/lens fields, **tag names**, and per-asset notes — typing a tag name into the search box finds tagged assets
- Search results should return in under 200ms on a 500k asset catalog with proper indexing

### Lightroom Classic interop

- Alexandria reads XMP sidecar files written by Lightroom Classic to sync ratings, color labels, and keywords into the catalog
- Alexandria can optionally write XMP sidecar files so changes made in Alexandria appear in Lightroom Classic
- The XMP interchange layer is the handshake — neither app is "special", the file is the source of truth
- Conflict resolution is user-configurable: "XMP wins" (Lightroom/XMP is authoritative) or "catalog wins" (Alexandria is authoritative)
- Lightroom's develop settings, local adjustments, and crop data are stored in its own proprietary catalog format and are NOT read by Alexandria. Only the portable XMP subset (rating, label, keywords) is exchanged.
- Lightroom color label strings are locale-dependent ("Red", "Rojo", etc.). Alexandria normalises these on read.

### Keyboard-driven workflow

- All common triage operations are keyboard-accessible: ratings (1–5), color labels, flags, navigation (arrows), full-screen preview (space), open in app
- All keybindings are user-configurable
- Keybindings are context-scoped (grid, detail, import, global) so the same key can mean different things in different contexts
- Default bindings follow platform conventions (Cmd on Mac, Ctrl on Windows/Linux)

### Undo/redo

- All catalog editing operations (rating, tagging, labelling, collection membership) are undoable
- Undo/redo is implemented via a command pattern with a history stack (default depth: 50, configurable)
- Import, source management, and disk deletion are not undoable
- The undo stack is in-memory only and does not persist across app restarts

### Settings

- Comprehensive user preferences system covering: XMP conflict resolution, catalog backup retention, undo stack depth, default sort orders, and more
- Catalog-scoped settings are stored in the catalog database in a key-value table with JSON values for complex types — they travel with the catalog
- Machine-scoped settings (worker pool sizes, memory limit) live in a local `machine.json` file so a catalog restored on different hardware doesn't import another machine's tuning

### Updates

- Alexandria checks the GitHub Releases API for new versions on launch (at most once per 24h)
- The user is notified via a non-intrusive indicator linking to the release page — **no in-app auto-install in v1**. Wails has no built-in updater; a self-update mechanism (download, verify, platform-specific replace, code-signing implications) is deferred.
- Release binaries are distributed as: .dmg (macOS), .AppImage/.deb (Linux), .exe installer (Windows), hosted on GitHub Releases
- Catalog compatibility across versions is protected independently of the update mechanism: the schema version check at startup (see Migrations doc) refuses to open a catalog newer than the app understands, and migrates older catalogs forward with an automatic backup

## Non-functional requirements

### Performance

- Thumbnail grid must scroll at 60fps with 10,000+ assets visible (requires virtualised rendering)
- Search results must return in under 200ms on a 500k asset catalog
- App must reach an interactive state in under 3 seconds on a 100k asset catalog (progressive loading — show UI immediately, load data in background)
- Import throughput target: 500 JPEGs/minute on average hardware
- Library size ceiling: 500,000 assets (design target; may exceed in practice with good indexing)

### Resource usage

Alexandria will frequently run alongside heavy creative applications: DaVinci Resolve, Adobe Lightroom Classic, Adobe Photoshop, Adobe InDesign. Resource usage must be respectful of this reality.

- Worker pool sizes for import are configurable and default to conservative values (4 hash workers, 2 extraction workers, 2 thumbnail workers)
- Go's memory limit (`GOMEMLIMIT`) is set at startup based on available system RAM to prevent memory ballooning during bulk imports
- GC aggressiveness is tunable via settings
- The app never saturates CPU or disk I/O to the point of degrading co-running applications without explicit user configuration to allow it
- Background operations (watcher polling, catch-up scans) run at low priority and yield to user-triggered operations

### Reliability

- The catalog is the user's primary organisational work product. Losing it is catastrophic.
- Automatic catalog backup before every migration (see Schema Migrations docs)
- Configurable rolling backups (default: keep last 10)
- SQLite WAL mode for crash safety — an app crash cannot corrupt the catalog mid-write
- SQLite integrity check (`PRAGMA integrity_check`) runs on startup and warns the user if the catalog is damaged
- Import operations are transactional — a crash mid-import leaves the catalog in a consistent state (partial, not corrupted)
- Deletion from disk requires double confirmation and cannot be undone by Alexandria

### Security

- The catalog contains file paths to potentially sensitive creative work
- The catalog file is created with permissions 0700 (owner read/write only)
- No telemetry is collected without explicit opt-in (telemetry feature is deferred but must be opt-in, privacy-respecting, and transparent when implemented)
- Alexandria does not make network requests except for update checks, and update checks can be disabled

### Idempotency

All batch operations and background jobs are designed to be idempotent. Re-running an import, a reconciliation scan, or an XMP sync on the same data produces the same result as running it once. This is enforced at the scanner level by checking mtime + size against the catalog before processing a file.

## Explicitly out of scope (v1)

- Plugin or extension system. Contributors add features via code contributions or feature requests. A plugin system is a maintenance and support burden that is not justified at this stage.
- Team or multi-user features. Alexandria is a single-user application. The catalog is not designed for concurrent access from multiple users.
- Cloud storage integration (S3, Google Drive, Dropbox, etc.). Network shares (SMB/NFS) are supported; cloud provider SDKs are not.
- Media file backup. Alexandria backs up its own catalog only. Backing up the user's source files is the job of dedicated tools (restic, borg, Time Machine) that do it better; taking it on would be a liability. This is stated prominently in user documentation alongside the "back up catalog.db" guidance.
- AI/ML tagging (face recognition, object detection, scene classification). This is a P2 feature. The schema does not preclude it but no infrastructure is built for it in v1.
- Built-in image/video editing. Alexandria opens files in external apps.
- Export/publishing workflows (web galleries, zip exports, FTP upload). Out of scope for v1.
- Mobile companion app.
- Localisation. Strings are not hardcoded in ways that preclude i18n, but no translations are provided in v1. This will be painful to retrofit if strings are concatenated in code — they should not be.
- Accessibility. Foundational keyboard navigation is in scope. Full screen reader support and WCAG compliance are deferred. The structure should not preclude adding them later.
- Onboarding tour. A help guide will be hosted online. In-app tours are deferred.
