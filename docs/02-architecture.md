# Architecture Overview

## Core architectural principles

### 1. Catalog-first

The SQLite catalog is the source of truth. The filesystem is a source of data, not the authority.

This means:
- Assets exist in the catalog independent of whether their source file is currently accessible
- Moving a file from one drive to a NAS is a path update operation, not a re-ingest
- Thumbnails and metadata are stored in the catalog/app data directory, not derived on demand from the source file
- The catalog can be queried, searched, and browsed regardless of which sources are online

The alternative — filesystem-first, where the catalog is just an index you can throw away — would mean assets disappear when drives are disconnected, and organisation work (tags, ratings, collections) could be lost in a re-index. For a DAM used by a creative professional with multiple drives, this is unacceptable.

**Analogy:** Lightroom Classic uses this model. The catalog is primary; "?" icons appear on assets when the source file is missing, but the catalog record, metadata, and develop settings are all intact.

### 2. Reference model

Alexandria indexes files where they live. It never moves, copies, or reorganises them.

The user's folder structure is theirs. Alexandria observes it; it does not manage it. This is essential for a power user who has spent years building a deliberate archive structure.

**Contrast with:** Apple Photos, which copies imported files into its own library bundle and manages storage. This is the managed model. It is not appropriate here.

### 3. Loose coupling / producer-consumer architecture

Components communicate through well-defined interfaces and channels, not direct calls. The ingest pipeline is a series of stages connected by buffered channels. The watcher service feeds events into the ingest pipeline without knowing what the pipeline does with them. The UI subscribes to catalog change events without knowing what triggered them.

This makes components independently testable, replaceable, and understandable in isolation.

### 4. Dependency injection for platform APIs

Every place where the code must interact with the operating system differently on Mac vs Linux vs Windows is behind an interface. File watchers, drive identifiers, volume monitors, and "open in app" launchers are all injected at startup via platform-specific implementations.

Benefits:
- Platform code is isolated to `internal/platform/darwin`, `internal/platform/linux`, `internal/platform/windows`
- Tests inject mock implementations — no real filesystem events needed
- Adding a new platform means implementing the interfaces, not refactoring the core

### 5. Nothing in internal packages imports Wails

The Wails framework is confined to `app/` and `cmd/alexandria/`. All business logic lives in `internal/` packages that have no knowledge of Wails. This means the entire backend is testable with standard `go test` — no Wails runtime required.

The `app/` layer is thin: it translates between Wails IPC calls and internal service calls, and emits Wails events when catalog state changes.

---

## Technology decisions

### Go

**Why:** Go's concurrency model (goroutines + channels) maps directly to the producer-consumer ingest pipeline. The language is learnable quickly, has strong stdlib support for the operations Alexandria needs (file I/O, hashing, SQLite via `database/sql`), and is increasingly common in the job market for the kind of systems work this project represents.

**Trade-off vs Rust:** Rust would give better memory efficiency and deterministic deallocation — relevant when running alongside Photoshop and Resolve. Go's garbage collector requires tuning (`GOMEMLIMIT`, worker pool throttling) to achieve acceptable resource behaviour. This is a manageable trade-off given Go's significantly lower learning curve and better job market alignment for the primary developer.

**Trade-off vs Flutter/Avalonia:** Those frameworks avoid NPM by using a single language for UI and backend. Go + Wails still requires a web frontend (and therefore NPM). The trade-off was accepted because Go's concurrency model and ecosystem for system-level work (FFmpeg, SQLite, file I/O) is stronger than Dart or C#.

### Wails v2

**Why:** Wails gives a cross-platform desktop app using Go as the backend and a web frontend for UI. Unlike Electron, it uses the OS's native webview (WebKit on Mac, WebKitGTK on Linux, WebView2 on Windows) — no bundled Chromium. Binary sizes are 3–10MB vs Electron's 150MB+. Memory usage is substantially lower.

**The webview split:** The IPC boundary between Go backend and web frontend is typed and fast (sub-millisecond). The frontend never does business logic — it calls Go functions and subscribes to Go events. This keeps the backend testable independently of the UI.

**Trade-off:** The web frontend still requires NPM and a JavaScript framework. This is accepted. NPM complexity is manageable with a disciplined dependency approach (prefer fewer, well-maintained packages).

### SQLite

**Why:** The catalog is a single-user, single-process database on the local machine. SQLite is the right tool: embedded, zero-configuration, reliable, fast for read-heavy workloads, and well-supported in Go. A client-server database (Postgres, MySQL) would add operational complexity with no benefit.

**Configuration:** WAL (Write-Ahead Logging) mode is essential. WAL allows concurrent reads while a write is in progress, which means the UI can query the catalog while the ingest pipeline is writing — no locking the grid while importing.

**Ceiling:** With proper indexing, SQLite handles 500k+ rows in the assets table comfortably. The performance target of sub-200ms search on a 500k catalog is achievable.

### GPL v3

**Why:** Alexandria is a desktop application with no server component. GPL v3 ensures that anyone distributing a modified version must open source their changes. The "network use" loophole that AGPL closes is not relevant for a desktop app, so GPL v3 is the simpler and less controversial choice. It does not restrict users from using Alexandria with proprietary files or in a commercial context.

### GitHub Releases

**Why:** Standard for open source desktop apps. Familiar to contributors and users. Tauri's built-in updater plugin supports it natively. Avoids the complexity of running release infrastructure.

---

## Directory structure

```
alexandria/
  cmd/
    alexandria/       -- Wails entry point. Wires together all services and starts the app.
  internal/
    domain/           -- Pure Go types. No external dependencies. Everything imports this.
    catalog/          -- All SQLite operations. Repository implementations.
    ingest/           -- Import pipeline. Scanner, hasher, extractor, thumbnailer, writer.
    watcher/          -- File system watching and network polling service.
    thumbnailer/      -- Thumbnail generation. Dispatcher routes by MIME type.
    metadata/         -- Metadata extraction. Dispatcher routes by MIME type.
    xmp/              -- XMP read/write. Lightroom interop.
    commands/         -- Command pattern. Undo/redo stack and command implementations.
    sources/          -- Source and drive management.
    migrations/       -- Schema migration system and SQL migration files.
    platform/         -- Platform interface definitions.
      darwin/         -- macOS implementations.
      linux/          -- Linux implementations.
      windows/        -- Windows implementations.
    testutil/         -- Shared test helpers. In-memory DB, temp sources, stub implementations.
  app/                -- Wails command handlers. Thin translation layer.
  testdata/           -- Fixture files for tests. Sample images, RAW files, XMP files, etc.
    images/
    video/
    raw/
    xmp/
    documents/
  docs/               -- This documentation.
```

**Key rule:** Nothing in `internal/` imports `app/` or anything Wails-related. The dependency graph flows one way: `cmd` → `app` → `internal` → `domain`.

---

## Data flow overview

### Import (user-triggered)

```
User clicks Import
  → app layer creates ImportJob
  → Importer.Run() starts pipeline
      Scanner walks source directory
        → emits ScannedFile per discovered file
      Hasher pool (N workers) computes partial hash per file
        → emits HashedFile
      Dedup checker (single goroutine) checks against catalog
        → routes: skip (already indexed, unchanged) | pass through | duplicate queue
      Metadata extractor pool (N workers) reads EXIF/IPTC/XMP/video streams
        → emits ExtractedFile with populated Asset struct
      Thumbnailer pool (N workers) generates thumbnail, writes to app data dir
        → emits ThumbedFile
      Catalog writer (single goroutine) writes to SQLite in batches of 50
        → emits progress events to frontend via Wails events
  → Import completes, summary shown in modal
```

### File change (watcher-triggered)

```
Watcher detects file modified (FSEvents / inotify / polling)
  → debounce (500ms) to handle temp-file-rename patterns
  → handleEvent routes by event type:
      Created/Modified → Importer.IngestFile() (enters pipeline at hasher stage)
      Deleted → mark location as "missing" in catalog (never auto-remove asset)
      Renamed → update location's relative_path (no new asset created)
  → catalog change event emitted → frontend re-queries current view
```

### Drive mount

```
VolumeMonitor detects new volume
  → DriveIdentifier reads filesystem UUID
  → SourceRepository.FindByFilesystemUUID()
  → if found: update base_path, mark source active, start watcher, trigger reconciliation scan
  → if not found: ignore (unknown drive)
```

### UI query

```
User changes view / filter / sort
  → frontend calls Go command (e.g. GetAssets(filter))
  → AssetRepository queries SQLite with filter
  → returns page of results
  → frontend renders virtualised grid
```

The frontend never holds a local copy of the asset library. It queries on demand. SQLite with proper indexes returns pages of results in under 10ms.

---

## State management philosophy

State lives in exactly one place per concern:

| Concern | Owner | Access pattern |
|---|---|---|
| Asset metadata, tags, collections | SQLite catalog | Query on demand |
| Import progress, source status | Go service layer | Push via Wails events |
| Current view, selected assets, filter state | Frontend | Local reactive state |
| User preferences | SQLite settings table | Loaded at startup, updated via settings commands |
| Undo/redo history | In-memory command stack | Queried by frontend for menu state |

**The rule:** The frontend never owns catalog state. It mirrors it by querying Go. When catalog state changes (import, file watch event, undo/redo), Go emits a `catalog:changed` event and the frontend re-queries the current view. No fine-grained cache invalidation. No optimistic updates.

This is intentionally simple. The round-trip from frontend to Go for a catalog query is sub-millisecond over Wails IPC. There is no perceptible latency to avoid.

---

## Concurrency model

Go's goroutine model is used deliberately throughout:

- **Import pipeline:** Each stage is a pool of goroutines consuming from an input channel and producing to an output channel. Stages are decoupled — a slow thumbnailer doesn't block the hasher.
- **Watcher service:** One goroutine per watched source, plus a goroutine monitoring volume events. All feed into shared channels.
- **Catalog writer:** Single goroutine. One write path to SQLite avoids lock contention.
- **UI thread (frontend):** Never blocked by Go operations. All Go calls are async from the frontend's perspective; results come back via callbacks or events.

Worker pool sizes are bounded and configurable. They default to conservative values appropriate for a machine running other heavy applications. See requirements doc for defaults.

---

## Catalog file layout

```
~/Library/Application Support/alexandria/  (macOS)
~/.local/share/alexandria/                 (Linux)
%APPDATA%\alexandria\                      (Windows)
  catalog.db          -- The catalog. Primary work product. Back this up.
  thumbnails/         -- Thumbnail cache. Keyed by asset UUID. Rebuildable.
    ab/
      ab1234cd-...jpg
  xmp-cache/          -- Local copies of XMP sidecars read/written by Alexandria.
  alexandria.log      -- Application log. Rotated at size limit.
  backups/
    catalog-2024-07-01T10-30-00.db
    catalog-2024-07-02T10-30-00.db
    ...
```

The thumbnail directory uses a two-character prefix subdirectory (`ab/` for UUIDs starting with `ab`) to avoid filesystem limits on files per directory at large library sizes.

**What to back up:** Only `catalog.db` and `backups/` are essential. Thumbnails are rebuildable from source files. If the catalog is lost, the source files are intact but all organisation work (tags, ratings, collections) is gone.

This trade-off — and its backup implications — should be surfaced prominently in user documentation.
