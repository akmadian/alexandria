# Error Handling

## Philosophy

Errors should be handled at the right altitude. An error deep in the thumbnail pipeline should not crash the app. An error opening the catalog must. The most common mistake in error handling is treating all errors the same way — either swallowing everything silently or surfacing everything as a fatal alert.

The guiding principle: **the user should always be able to do something useful with the app**, even when things go wrong. Degrade gracefully; never silently corrupt.

---

## Error tiers

### Tier 1: Fatal — app cannot continue

The app cannot reach a usable state. Exit after showing a clear, actionable error dialog.

Examples:
- Catalog cannot be opened (locked, corrupt, permissions)
- Schema migrations fail
- Cannot create catalog backup before migration
- Schema version mismatch (catalog too new or too old for this app version)
- Out of disk space writing the catalog

**What to surface:** A dialog with the specific error, what went wrong, and what the user can do (e.g. "your backup is at X", "check disk space", "update the app").

**What NOT to do:** Log it and silently exit, or try to auto-recover in a way that could make things worse.

### Tier 2: Degraded — feature unavailable, app continues

A background service or non-critical feature has failed. The app is usable but something is not working.

Examples:
- Watcher service fails to start for a source (source path inaccessible, permissions)
- XMP write fails (file locked by Lightroom, source offline)
- Thumbnail generation fails for a specific file
- Update check fails (no network)
- A source goes offline mid-import

**What to surface:** A non-intrusive indicator (toast, status bar warning, or source status badge). Not a blocking modal. The user can continue working.

**What to log:** At `warn` level with enough context to diagnose the issue.

### Tier 3: Expected operational noise — ignore in UI

These are expected during normal operation. The user does not need to know about them.

Examples:
- File scanned, already indexed, unchanged → skip (not even worth logging at info)
- XMP sidecar does not exist for a file → skip
- A file disappears between the scanner finding it and the hasher reading it (race condition with the user deleting it)
- Unsupported file extension encountered

**What to surface:** Nothing to the user.

**What to log:** At `debug` level. Only visible when debugging.

---

## Typed errors

Raw `error` strings are insufficient for callers to make decisions. Alexandria defines typed errors in `internal/domain/errors.go` so callers can use `errors.As()` to branch on error type:

```
NotFoundError { Resource, ID }
ConflictError { Resource, Field, Message }
SourceOfflineError { SourceID, Path }
CatalogLockedError { Path }
ValidationError { Field, Message }
ErrKeybindingConflict { Combo, ConflictAction }
ErrSchemaTooOld { Current, Required }
ErrSchemaTooNew { Current, Known }
```

Example caller:
```
err := locationRepo.FindByAbsPath(ctx, path)

var offline domain.SourceOfflineError
if errors.As(err, &offline) {
    sourceRepo.UpdateStatus(ctx, offline.SourceID, domain.SourceStatusOffline)
    return nil  // expected, non-fatal
}

var notFound domain.NotFoundError
if errors.As(err, &notFound) {
    // file not in catalog yet, treat as new
    return handleNewFile(ctx, path)
}

return err  // unexpected — propagate up
```

---

## Error wrapping

All errors are wrapped with context as they propagate up the call stack. Use `fmt.Errorf` with `%w`:

```
// in a repository method:
return fmt.Errorf("AssetRepository.Get %q: %w", id, err)

// in the ingest pipeline:
return fmt.Errorf("ingest pipeline: hashing %q: %w", path, err)

// by the time it reaches the app layer:
// "ingest pipeline: hashing /path/to/file.arw: AssetRepository.Get "abc": asset not found"
```

A wrapped error carries a full breadcrumb. When this appears in a log, the path to the failure is unambiguous without a debugger or stack trace.

**Always wrap with `%w`, not `%v`.** `%w` preserves the error chain so `errors.As()` can unwrap it. `%v` converts to a string and loses type information.

---

## Pipeline error collection

The ingest pipeline does not use a single error return. Instead, it collects per-file errors and continues processing:

```
ImportResult
  Added    int
  Updated  int
  Skipped  int
  Errors   []ImportError

ImportError
  Path   string
  Stage  string    -- "scanning", "hashing", "extracting", "thumbnailing", "writing"
  Err    error
```

Workers send errors to a shared error channel. The orchestrator drains this channel into `ImportResult.Errors`. At the end of import, errors are surfaced in the summary UI:

```
Import complete
  1,243 added   12 skipped   3 errors
  [View errors] → expands to show:
    /Volumes/SSD/shoots/project/corrupted.arw
      Stage: extracting — EOF reading EXIF block
    /Volumes/NAS/archive/old/missing.psd
      Stage: hashing — no such file or directory
    ...
```

This approach means a single corrupt file does not abort an import of 2,000 files. The user can investigate and re-import failed files individually.

**Catalog write failures are special:** If the catalog writer encounters an error (SQLite error, disk full), this is not just a per-file error — it means the import is silently failing for all subsequent files. This should be surfaced at a higher severity (tier 2 degraded, not buried in the error list).

---

## Logging

### Library

Go's `log/slog` package (stdlib since Go 1.21). No external logging dependency.

```
logger = slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{
    Level: slog.LevelDebug,  // configurable via settings
}))
slog.SetDefault(logger)
```

JSON format for machine-readable logs (useful for searching with tools like `jq`). Text format option for debugging (toggle via settings or launch flag).

### Log levels

| Level | When to use |
|---|---|
| `debug` | Internal state, skipped files, cache hits. High volume. Off by default. |
| `info` | Significant events: import started/completed, source connected/disconnected, migration applied. |
| `warn` | Degraded operation: XMP write failed, thumbnail generation failed, watcher error. Needs attention but app continues. |
| `error` | Unexpected failure: SQLite error, disk I/O error, internal inconsistency. Needs investigation. |

### Structured fields

Always log structured key-value fields rather than interpolating values into the message string:

```
// Good — fields are queryable
slog.Warn("xmp write failed", "path", xmpPath, "asset_id", assetID, "err", err)

// Bad — values are buried in the message string
slog.Warn(fmt.Sprintf("xmp write failed for %s (asset %s): %v", xmpPath, assetID, err))
```

Standard fields used throughout:
- `asset_id`: UUID of the relevant asset
- `source_id`: UUID of the relevant source
- `path`: filesystem path
- `err`: the error value
- `stage`: pipeline stage name
- `duration`: time taken for an operation

### Log file location

`{catalog_dir}/alexandria.log`

Rotated when it exceeds a size limit (default: 50MB). Old log files are renamed with a timestamp suffix. Keep the last 3 rotated logs.

The log file path is surfaced in the settings UI so the user can find it for bug reports.

**Never log to stdout in production.** Wails may capture stdout for its own purposes. All logging goes to the file.

---

## Surfacing errors to the frontend

Two channels depending on whether the error is async or synchronous:

### Async errors (background services)

Background services (watcher, XMP sync, catch-up scan) emit typed events via Wails:

```
// non-fatal source error
runtime.EventsEmit(ctx, "error:degraded", DegradedError{
    Code:    "watcher_failed",
    Message: "File watching unavailable for 'Archive NAS'",
    SourceID: src.ID,
})
```

Frontend shows a non-intrusive toast or status indicator. The user is informed without being blocked.

### Synchronous errors (user-triggered operations)

The `app/` layer returns a typed `AppError` struct rather than a raw Go `error`. Wails serialises this to JSON for the frontend:

```
AppError
  Code    string    -- "rating_failed", "import_failed", "source_offline", etc.
  Message string    -- human-readable, suitable for display
  Detail  string    -- technical detail for logging/debugging (not shown in UI by default)
```

The zero value (`AppError{}`) means no error. The frontend checks `if (result.code)` before acting on results.

```
// in app layer
func (a *App) SetRating(ctx context.Context, assetIDs []string, rating int) AppError {
    cmd := commands.NewSetRatingCommand(...)
    if err := a.stack.Execute(ctx, cmd); err != nil {
        slog.Error("SetRating failed", "err", err, "count", len(assetIDs))
        return AppError{
            Code:    "rating_failed",
            Message: "Could not update rating.",
            Detail:  err.Error(),
        }
    }
    return AppError{}
}
```

---

## The altitude rule

A simple heuristic for deciding where to handle an error:

- **Can the caller do something useful with this error?** → return it
- **Is this error expected in normal operation?** → log at debug, return nil
- **Is this a background service error?** → log at warn, emit degraded event, continue
- **Is this unrecoverable?** → log at error, surface fatal dialog, exit

If you find the same error-handling pattern repeated in multiple places for the same error type, the handler belongs one level higher — propagate and handle once.

---

## Things that must never be silently swallowed

1. Catalog write failures during import
2. Migration failures
3. SQLite integrity check failures
4. Any error during the fatal startup stages

Silent failures in these categories are worse than a crash — they leave the user believing everything is fine when it is not.
