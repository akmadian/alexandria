# Startup Sequence

## Overview

App startup is ordered deliberately. Some stages must complete before others begin. Some stages can fail gracefully (app starts in a degraded state); others are fatal (app cannot proceed). Understanding the sequence is important for reasoning about initialisation dependencies and first-launch behaviour.

---

## Startup stages

### Stage 1: Resolve catalog directory (blocking, fatal on failure)

Determine where the catalog lives. This is platform-specific:

| Platform | Path |
|---|---|
| macOS | `~/Library/Application Support/alexandria/` |
| Linux | `~/.local/share/alexandria/` |
| Windows | `%APPDATA%\alexandria\` |

Create the directory if it does not exist (first launch). Fail with a clear error dialog if the directory cannot be created (permissions issue, disk full).

### Stage 2: Open SQLite connection (blocking, fatal on failure)

Open the catalog database at `{catalog_dir}/catalog.db`.

SQLite connection string includes:
- `_journal=WAL` — enable Write-Ahead Logging for concurrent read/write
- `_timeout=100` — 100ms timeout on lock acquisition (used to detect another instance running)
- `_foreign_keys=on` — enforce referential integrity

**Lock detection:** Attempt a `PRAGMA user_version` read immediately after opening. If this fails with a lock error, another Alexandria instance has the catalog open. Surface a clear error: "Alexandria is already running. Only one instance can open the catalog at a time." Do not attempt recovery — exit cleanly.

If the open fails for any other reason, surface the error with the catalog path. Exit.

### Stage 3: Run migrations (blocking, fatal on failure)

Run pending schema migrations against the opened database. See [Schema Migrations](12-migrations.md) for the full migration system description.

**Before running migrations:** If any migrations are pending, take an automatic backup of `catalog.db` to `backups/catalog-{timestamp}.db`. If the backup fails, abort startup — do not run migrations without a backup. Surface: "Could not create a safety backup before updating the catalog. Please check disk space."

**Migration failure:** If a migration fails, the transaction rolls back (the catalog is in the state before that migration). Surface a clear error with the migration number and error message. Tell the user where their backup is. Exit.

**Schema version check:** After migrations, verify that the catalog's schema version matches what this build expects. If the catalog is newer than the app knows about (user opened a catalog created by a newer Alexandria version), surface: "This catalog was created with a newer version of Alexandria. Please update the app." Exit.

### Stage 4: SQLite integrity check (non-blocking, warn on failure)

Run `PRAGMA integrity_check` in a background goroutine. This takes seconds on a large catalog and should not block the UI.

If the check finds problems, emit a `catalog:integrity_warning` event to the frontend. The frontend shows a non-intrusive banner: "The catalog may be damaged. Consider restoring from a backup." Do not block the user — they may want to export what they can before restoring.

### Stage 5: Wire dependencies (blocking, fast)

Construct all repositories and services. This is pure in-memory work — no I/O. It must complete before anything else starts.

Order of construction:
1. Repositories (AssetRepo, LocationRepo, SourceRepo, TagRepo, CollectionRepo, KeybindingRepo, SettingsRepo)
2. Settings (load from DB immediately — needed by all services)
3. Platform implementations (FileWatcher, VolumeMonitor, DriveIdentifier, Opener) — selected based on `runtime.GOOS`
4. Pipeline components (Hasher, DispatchMetadataExtractor, DispatchThumbnailer)
5. Importer (depends on pipeline components + repos)
6. Undo stack (depends on Settings for stack size)
7. Services (WatcherService, XMPService) — constructed but not started yet
8. App layer (depends on everything above)

**Apply GOMEMLIMIT:** After loading settings, set `debug.SetMemoryLimit(settings.MemoryLimitMB * 1024 * 1024)`. This caps Go's memory ceiling before any heavy work begins.

### Stage 6: Seed defaults (blocking, fast, first-launch only)

On first launch (detected by checking if any rows exist in `settings` or `keybindings`), seed:

- Default settings values (see schema doc for full list)
- Default keybindings for the current platform

This is idempotent — safe to call every launch. It checks before inserting.

### Stage 7: Start watcher service (non-blocking)

Call `WatcherService.Start(ctx)` which launches background goroutines for all active sources. Returns immediately.

Failure to start the watcher is **non-fatal**. If the watcher fails to start (permissions error, source path inaccessible), log at warn level and emit a `watcher:error` event to the frontend. The app continues — the user can still browse, search, and manually import. They just won't get automatic change detection.

### Stage 8: Check for updates (non-blocking, if enabled)

If `settings.UpdateCheckEnabled` is true, spawn a goroutine that queries the GitHub Releases API for newer versions. If a newer version is found, emit an `update:available` event. The frontend shows a non-intrusive indicator. This should not block startup or the UI.

Do not check for updates on every launch if the last check was within 24 hours — cache the last-check timestamp to avoid rate-limiting the GitHub API.

### Stage 9: Signal ready to frontend (blocking)

Emit `app:ready` with the initial payload the frontend needs to render its first frame:

```
AppReadyPayload
  CatalogPath   string
  AssetCount    int           -- total non-deleted assets in catalog
  SourceCount   int           -- total active sources
  UndoState     StackState    -- { CanUndo: false, CanRedo: false, ... } on first launch
  Settings      Settings      -- full settings object
  Keybindings   []Keybinding  -- all keybindings for the current context
```

The frontend transitions from its loading screen to the main UI on receipt of this event. Everything needed for the initial render is in this one payload — no additional round-trips before the UI is interactive.

### Stage 10: Background catch-up scan (non-blocking, low priority)

After a short delay (2 seconds — let the user see the UI first), run a low-priority reconciliation scan against all active sources. This catches changes that happened while the app was closed: files modified overnight on a NAS, photos added to an external drive before connecting it.

The delay is intentional: the user should see the app become responsive before any background I/O begins. Starting heavy I/O before the UI is interactive feels like the app is "hanging" even if the UI loads correctly.

---

## Startup decision tree

```
Resolve catalog dir
  ↓ fail → error dialog, exit
Open SQLite
  ↓ fail (locked) → "already running" dialog, exit
  ↓ fail (other) → error dialog with path, exit
Backup if migrations pending
  ↓ fail → "can't backup" dialog, exit
Run migrations
  ↓ fail → error dialog with backup location, exit
Schema version check
  ↓ too old → "update app" dialog, exit
  ↓ too new → "update app" dialog, exit
Integrity check ──────────────────────────→ (background goroutine)
Wire dependencies
Seed defaults
Start watcher ────────────────────────────→ (background, non-fatal if fails)
Check for updates ────────────────────────→ (background, if enabled)
Emit app:ready → frontend shows UI
Background catch-up scan (after 2s) ─────→ (background, low priority)
```

Two hard exits: can't open the database, can't migrate safely. Everything else degrades gracefully and the app reaches a usable state.

---

## First launch experience

On first launch:
- The catalog directory is created
- An empty catalog.db is created and migrated to the current schema
- Default settings and keybindings are seeded
- The watcher service starts but has no sources to watch
- `app:ready` fires with `AssetCount: 0, SourceCount: 0`
- The frontend shows an empty state with a prominent "Add Source" call to action

There is no onboarding wizard or tour (deferred). The help guide is online. The empty state should make the next step obvious.

---

## Shutdown

When the user quits the app, Wails calls the app's `shutdown` callback:

1. Cancel the root context — all goroutines exit via `ctx.Done()`
2. Stop the file watcher: `FileWatcher.Close()`
3. Drain any in-progress import gracefully (the pipeline's cancellation path handles this)
4. Close the SQLite connection
5. Exit

Shutdown should complete within a few seconds. If an import is in progress, cancellation is triggered and the partially-completed import is left in a consistent state (the catalog writer only commits complete batches).

---

## Crash recovery

If Alexandria crashes mid-operation:

- **Mid-import:** The catalog writer only commits batches inside SQLite transactions. A crash rolls back the uncommitted batch. The catalog is consistent. Some files from the current batch were not indexed, but they will be picked up on the next import (idempotency).
- **Mid-settings write:** Settings are written in individual SQLite statements. A crash might leave a setting at its old value. This is acceptable.
- **Mid-thumbnail write:** Thumbnail files are written to disk independently of the catalog. A crash might leave a thumbnail file without a corresponding catalog entry. On next launch, orphaned thumbnails can be detected and cleaned up (a future maintenance task).
- **Mid-migration:** Migration transactions roll back on crash. The next launch re-runs the migration from scratch.

SQLite's WAL mode and transaction semantics protect the catalog from corruption in crash scenarios. The catalog is safe; in-flight work may be partially lost but can be redone.
