# State & Commands

## Overview

This document covers two related concerns:

1. **Backend state management:** How the Go backend handles state, particularly for user-triggered catalog edits
2. **Command pattern:** How undo/redo is implemented

---

## Backend state categories

The backend has four kinds of state with different lifetimes and management strategies:

### 1. Catalog state (SQLite)

Assets, tags, collections, sources, locations. This is the primary work product of the application. Managed by the repository layer. The source of truth. Never held in memory longer than needed — always query fresh from SQLite.

### 2. Service state (in-memory, process lifetime)

The watcher service knows which sources are being watched. The importer tracks active import jobs. The undo stack holds the command history. These live in memory and are rebuilt at startup. They do not need to survive crashes (the catalog does; service state is reconstructible).

### 3. Settings (SQLite, cached in memory)

Loaded from the settings table at startup and held in memory as a typed `Settings` struct. Written back to SQLite on each change. The in-memory copy is always up to date because all settings changes go through the settings service.

### 4. Transient pipeline state

Channels, worker goroutines, in-flight file batches during import. Lives only for the duration of an import job. Cleaned up when the import completes or is cancelled.

---

## Write strategy by operation type

Not all writes should be handled the same way:

### Single-asset updates → write immediately

Rating one asset, setting a color label, adding a tag to one file. Just write it. No buffering, no batching, no intermediate state. The round-trip from UI to Go to SQLite and back is imperceptible (sub-5ms). Complexity of buffering single writes adds no value.

### Bulk operations → single transaction

User selects 500 assets, sets rating to 4. This is one atomic operation:

1. Capture the previous state of all 500 assets (for undo)
2. Open a SQLite transaction
3. Update all 500 records
4. Commit

If anything fails, the transaction rolls back. The catalog is either fully updated or unchanged. No partial state. One disk fsync instead of 500.

The transaction IS the buffer. No application-level write queue is needed.

### High-frequency UI events → debounce in frontend

If the UI has a slider or any control that generates rapid events (e.g. scrubbing through a star rating), debounce in the frontend (300ms of quiet) before calling the Go backend. The backend never sees the intermediate states. This keeps the backend simple.

---

## Command pattern

The command pattern wraps every user-initiated catalog edit in an object that knows how to execute itself and how to undo itself. This is what enables undo/redo.

### Command interface

Every command implements:

```
Command
  Execute(ctx) → error     -- performs the action, captures previous state for undo
  Undo(ctx) → error        -- reverses the action using captured previous state
  Description() → string   -- human-readable label for undo menu: "Set rating 4 — 500 assets"
```

### Undo stack

The stack is a simple in-memory data structure owned by the app layer:

```
Stack
  undo []Command     -- history of executed commands, newest last
  redo []Command     -- commands that were undone and can be redone
  maxSize int        -- configurable (default 50)
  mu sync.Mutex      -- protects concurrent access

Stack.Execute(ctx, cmd):
  cmd.Execute(ctx)
  push cmd onto undo stack
  clear redo stack  (any new action invalidates the redo history)
  trim undo stack to maxSize (drop oldest if over limit)

Stack.Undo(ctx):
  pop cmd from undo stack
  cmd.Undo(ctx)
  push cmd onto redo stack

Stack.Redo(ctx):
  pop cmd from redo stack
  cmd.Execute(ctx)
  push cmd onto undo stack

Stack.State() → StackState:
  returns { CanUndo, CanRedo, UndoLabel, RedoLabel }
  UndoLabel: description of the command that would be undone
  RedoLabel: description of the command that would be redone
```

### Previous state capture

**This is the most important detail of the command pattern.** Before executing a bulk operation, the command captures the current state of every affected asset. This allows undo to restore each asset to its individual prior state, even if assets had different values before the bulk operation.

Example: 500 assets selected, ratings were 0, 2, 3, 4, 5 (mixed). User sets all to 4.

- **Without capture:** Undo would need to know what each asset's rating was. It can't.
- **With capture:** `prevRatings map[string]*int` is populated before the write. Undo iterates this map and restores each asset individually.

The previous state is captured inside `Execute()`, before the write:

```
SetRatingCommand.Execute(ctx):
  for each assetID:
    asset = assetRepo.Get(ctx, assetID)
    prevRatings[assetID] = asset.Rating   ← captured before write
  assetRepo.BulkPatch(ctx, assetIDs, {Rating: newRating})

SetRatingCommand.Undo(ctx):
  assetRepo.BulkPatchIndividual(ctx, prevRatings)   ← restore each individually
```

`BulkPatchIndividual` accepts a map of asset ID → patch and applies them in a single transaction.

### What goes on the undo stack

| Operation | Undoable | Reason |
|---|---|---|
| Set rating (single or bulk) | Yes | Trivially reversible |
| Set color label (single or bulk) | Yes | Trivially reversible |
| Set flag (single or bulk) | Yes | Trivially reversible |
| Add tag to asset(s) | Yes | Reversible by removing |
| Remove tag from asset(s) | Yes | Reversible by re-adding |
| Add asset(s) to collection | Yes | Reversible by removing |
| Remove asset(s) from collection | Yes | Reversible by re-adding |
| Create collection | Yes | Reversible by deleting (if empty) |
| Rename collection | Yes | Reversible by renaming back |
| Soft delete asset (move to trash) | Yes | Reversible by restoring |
| Import | **No** | Has side effects (filesystem events, thumbnails on disk). Re-running import is idempotent. |
| Delete from disk | **No** | Irreversible at the OS level. Protected by confirmation modal. |
| Add/remove source | **No** | Configuration action, not an editing action |
| Settings changes | **No** | Not an editing action; complexity not justified |
| XMP sync | **No** | External system operation; hard to reverse cleanly |

### Undo stack persistence

The undo stack is **in-memory only**. It does not persist across app restarts.

Persisting it would require:
- Serialising command state to the catalog
- Handling schema migrations on persisted command formats (commands can evolve)
- Dealing with the case where underlying files changed between sessions, making undo semantics ambiguous

This complexity is not justified. Restarting the app clears the undo history. This is the expected behaviour in virtually all desktop applications.

---

## Frontend communication

After every undo or redo, the catalog has changed. The app layer emits a `catalog:changed` event via Wails, and the frontend re-queries the current view. The undo menu items are driven by `Stack.State()` which the frontend calls to get `{ CanUndo, CanRedo, UndoLabel, RedoLabel }`.

The frontend binds:
- `Cmd+Z` (Mac) / `Ctrl+Z` (Win/Linux) → `App.Undo()`
- `Cmd+Shift+Z` (Mac) / `Ctrl+Y` (Win/Linux) → `App.Redo()`

These are registered as global keybindings (context: "global") in the keybindings system.

---

## Wiring in the app layer

The `App` struct in `app/` owns the undo stack and all service references. Every user-facing command goes through the stack:

```
App.SetRating(ctx, assetIDs, rating):
  cmd = NewSetRatingCommand(assetRepo, assetIDs, rating)
  stack.Execute(ctx, cmd)
  emit catalog:changed event

App.Undo(ctx):
  stack.Undo(ctx)
  emit catalog:changed event
  emit undo_state:changed event (so menu labels update)

App.UndoState() → StackState:
  return stack.State()
```

The app layer is thin. It does not contain business logic — it delegates to commands and services.

---

## No optimistic updates

A deliberate decision: Alexandria does not do optimistic UI updates for catalog operations.

When the user rates an asset, the flow is:

1. User clicks star
2. Frontend calls `App.SetRating()`
3. Go executes the command (SQLite write)
4. Go emits `catalog:changed`
5. Frontend re-queries and updates the display

The Go round-trip takes under 5ms over the Wails IPC bridge. The user will never notice the latency. In exchange:

- The UI always reflects the true catalog state
- There is no optimistic state to reconcile against reality
- There are no edge cases where the UI shows one thing and the DB has another

Optimistic updates are appropriate for high-latency network APIs. They are complexity without benefit for a local SQLite operation.
