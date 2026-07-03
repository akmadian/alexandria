# Watcher Service

## Overview

The watcher service runs for the lifetime of the app. It monitors active sources for changes and feeds those changes into the ingest pipeline. It also monitors the system for drives being mounted or unmounted.

The watcher service is a background service — it has no direct UI. Its effects are visible through catalog changes that flow to the frontend via events.

---

## Responsibilities

1. **Local source watching:** Use OS filesystem events (FSEvents/inotify) to detect file changes in real time
2. **Network source polling:** Poll network shares on a configurable interval since filesystem events are unreliable over SMB/NFS
3. **Volume monitoring:** Detect when external drives are plugged in or out and reconnect/disconnect sources accordingly
4. **Event routing:** Route file events to the ingest pipeline at the appropriate entry point

---

## Architecture

The watcher service owns:
- One `FileWatcher` instance (injected, OS-specific) that watches all local source paths
- A map of `sourceID → *time.Ticker` for network sources being polled
- A `VolumeMonitor` instance (injected, OS-specific) for drive events
- References to the `Importer` and repository interfaces it needs

All of this is started in `Service.Start(ctx)` and runs until the context is cancelled (app shutdown).

---

## Local source watching

For local sources and external drives, Alexandria uses OS-provided filesystem events:

- **macOS:** FSEvents (via `fsnotify`)
- **Linux:** inotify (via `fsnotify`)
- **Windows:** ReadDirectoryChangesW (via `fsnotify`)

When a local source is attached, the watcher starts watching the source's `BasePath` recursively. Events flow into a per-source goroutine that debounces them before acting.

**Debouncing is critical.** Applications like Photoshop, Illustrator, and most creative tools do not write files atomically. A typical save sequence looks like:

1. Write to a temporary file: `hero.psd.tmp`
2. Flush and close
3. Rename `.tmp` to `hero.psd` (the final atomic step)

This generates multiple filesystem events (create temp, modify temp, rename). Without debouncing, Alexandria would attempt to index the temp file, fail or generate a bad thumbnail, and then index the final file. The debouncer waits for 500ms of quiet on a given path before acting — by which time the rename has completed and only the final file path is active.

**Debouncer implementation:** A map of `path → *time.Timer`. Each new event for a path resets the timer. When the timer fires, the handler runs. A new event that arrives during the debounce window cancels the previous timer and starts a new one.

### Event handling

Events are routed based on type:

**Created / Modified:**
→ `Importer.IngestFile(ctx, source, absPath)`
This enters the ingest pipeline at the hasher stage. The same pipeline logic runs as for manual imports — hash check, dedup check, metadata extraction, thumbnail generation, catalog write. If the file already exists in the catalog (reimport case), it is updated rather than inserted.

**Deleted:**
→ `LocationRepository.UpdateStatus(ctx, locationID, LocationStatusMissing)`
The location record is marked "missing". The asset record is NOT deleted. The user decides what to do with assets whose files are gone. Alexandria surfaces this in the UI (a missing-file indicator on the asset card) but does not make the decision on the user's behalf.

**Renamed (Move within same source):**
→ `LocationRepository.Update(ctx, location)` with new `RelativePath`
The location's path is updated. No new asset record is created. Tags, ratings, collections, and all other metadata are preserved. This is the correct behaviour for a rename or move within a watched source.

**Renamed across sources (file moved from one watched folder to another):**
This is detected when a "deleted" event fires on source A and a "created" event fires on source B within a short window, and the hashes match. This is a complex case. For v1, it is handled as a delete + create (new asset record), not as a move. Cross-source move detection is a future enhancement.

---

## Network source polling

Filesystem events do not work reliably over SMB or NFS on any OS. For network sources, Alexandria uses polling instead.

Each network source has a `poll_interval_secs` configured (e.g. 60, 300). The watcher service starts a `time.Ticker` per network source. On each tick:

1. Check if the source path is currently accessible (network share reachable, credentials valid)
2. If not accessible: mark source offline, skip this tick, try again next tick
3. If accessible: run a full scan via `Importer.Run()` with `Priority: Low`

The importer's idempotency means polling is cheap when nothing has changed — the scanner skips all files where mtime + size haven't changed. The cost is proportional to the number of changed files, not the total library size.

**No progress UI for polling scans.** These are silent background operations. If changes are found, they appear in the catalog and the frontend receives a `catalog:changed` event. The user does not see a progress modal.

**Polling interval considerations:**
- 60 seconds: near-real-time for active work, but generates frequent network traffic on a large library
- 300 seconds (5 min): good balance for most use cases
- User-configurable per source: a NAS used for archival might poll every 30 minutes; a NAS used for active work might poll every minute

---

## Volume monitoring

The `VolumeMonitor` interface emits events when volumes mount or unmount.

### Volume mounted

When a volume mounts:

1. Read the filesystem UUID via `DriveIdentifier.FilesystemUUID(mountPath)`
2. Query `SourceRepository.FindByFilesystemUUID(uuid)`
3. **Found:** This is a known drive.
   - Update `source.BasePath` to the new mount path (mount points can change: `/Volumes/MySSD`, `/Volumes/MySSD 1`, etc.)
   - Update `source.Status` to `active`
   - Call `LocationRepository.MarkOnlineBySource(sourceID)` to mark all locations as potentially online (they'll be verified on the next scan)
   - Start watching the source: `FileWatcher.Watch(source.BasePath)`
   - Trigger a reconciliation scan: `Importer.Run(ctx, ImportJob{SourceID: source.ID, Priority: Low})`
   - Emit `source:connected` event to the frontend
4. **Not found by UUID:** Try disk serial as fallback: `SourceRepository.FindByDiskSerial(serial)`
5. **Found by serial but not UUID:** The drive was likely reformatted. The filesystem UUID changed. Prompt the user: "This looks like your [source name] drive, but it appears to have been reformatted. Would you like to reconnect it?" If confirmed, update `source.FilesystemUUID` and proceed as in step 3.
6. **Not found by either:** Unknown drive. Ignore silently. Do not auto-add it as a new source — the user adds sources intentionally.

### Volume unmounted

When a volume unmounts:

1. Find the source matching this mount path
2. If found:
   - Stop the file watcher for this source: `FileWatcher.Unwatch(source.BasePath)`
   - Update `source.Status` to `offline`
   - Call `LocationRepository.MarkOfflineBySource(sourceID)` — marks all locations as offline
   - Emit `source:disconnected` event to the frontend
3. Assets remain in the catalog, fully browsable with thumbnails. Only "open original" is disabled for offline assets.

---

## Starting and stopping

**Start:** Called during app startup (Stage 6 of the startup sequence, after services are wired up). Queries all active sources and attaches watchers/pollers to each.

```
Service.Start(ctx):
  list all active sources
  for each source:
    if kind is local or external_drive → watchLocal(ctx, source)
    if kind is smb or nfs → pollNetwork(ctx, source)
  start volume monitoring goroutine
```

Failure to start the watcher for a specific source is non-fatal. The app logs a warning and continues. The source will appear as needing manual re-import, but the app remains usable.

**Stop:** When the app context is cancelled (shutdown), all goroutines exit via `ctx.Done()`. The `FileWatcher.Close()` is called, tickers are stopped, and the volume monitor goroutine exits.

---

## Watcher failure handling

The watcher can fail for expected reasons:

- A local source path no longer exists (drive not mounted, folder deleted)
- A network source is unreachable (network down, NAS offline)
- Permission denied on the source path

These are handled gracefully:

- Log at warn level
- Mark the source as offline
- Emit a `source:degraded` or `source:offline` event to the frontend
- Continue running — other sources continue to be watched
- When the source becomes available again (drive mounts, network restores), the volume monitor or next poll tick will trigger reconnection

---

## Interaction with the ingest pipeline

The watcher service does not contain ingest logic — it delegates entirely to the `Importer`. This is the loose coupling principle in action: the watcher knows when things change; the importer knows how to update the catalog. They communicate through the ingest pipeline's `IngestFile()` entry point.

The watcher never writes to the catalog directly. This keeps the write path singular and testable.

---

## Summary of source types and watch strategies

| Source Kind | Watch Strategy | Event Source | Latency |
|---|---|---|---|
| `local` | FSEvents / inotify | OS filesystem events | Near-instant (debounced 500ms) |
| `external_drive` | FSEvents / inotify | OS filesystem events | Near-instant (debounced 500ms) |
| `smb` | Polling | Importer.Run() on ticker | Configurable (60s–1800s) |
| `nfs` | Polling | Importer.Run() on ticker | Configurable (60s–1800s) |
