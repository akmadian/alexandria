# Keybindings

## Overview

Alexandria is designed for keyboard-driven workflows. A creative professional doing triage on 2,000 photos cannot rely on mouse-only interaction. All common operations — rating, flagging, labelling, navigation, opening in app — are accessible by keyboard. All bindings are user-configurable.

---

## Platform normalisation

Mac uses `Cmd` (⌘) for system shortcuts. Windows and Linux use `Ctrl`. Users expect platform conventions — a Mac user will never reach for `Ctrl+Z` to undo.

Alexandria handles this by defining a virtual modifier: **`primary`**. At runtime, `primary` maps to `Cmd` on macOS and `Ctrl` on Windows/Linux. This mapping happens in exactly one place in the frontend (`resolveCombo` function) so the rest of the application never sees a distinction between `Cmd` and `Ctrl`.

Key combos are stored in the database using `primary` notation:
- `primary+z` → Cmd+Z on Mac, Ctrl+Z on Windows/Linux
- `shift+primary+z` → Cmd+Shift+Z on Mac, Ctrl+Shift+Z on Windows/Linux
- `1` → just the 1 key, platform-independent

---

## Context scoping

The same key can mean different things in different parts of the app. Keybindings are scoped to a `context`:

| Context | Where it applies |
|---|---|
| `global` | Everywhere, always |
| `grid` | When the asset grid is focused |
| `detail` | When the detail/preview panel is active |
| `import` | When the import modal is open |

Resolution order: specific context wins over global. If `space` is bound in `grid` context AND in `global` context, the `grid` binding fires when the grid is focused.

---

## Default bindings

Seeded at first launch for the current platform. Platform-specific defaults differ only where platform conventions differ (e.g. delete key behaviour).

### Global

| Action | Default (Mac) | Default (Win/Linux) |
|---|---|---|
| Undo | `primary+z` | `primary+z` |
| Redo | `shift+primary+z` | `primary+y` |
| Select all | `primary+a` | `primary+a` |
| Deselect all | `escape` | `escape` |
| Open in app | `primary+o` | `primary+o` |

### Grid context

| Action | Default |
|---|---|
| Rate 1 star | `1` |
| Rate 2 stars | `2` |
| Rate 3 stars | `3` |
| Rate 4 stars | `4` |
| Rate 5 stars | `5` |
| Clear rating | `0` |
| Flag as pick | `p` |
| Flag as reject | `x` |
| Clear flag | `u` |
| Label red | `6` |
| Label yellow | `7` |
| Label green | `8` |
| Label blue | `9` |
| Label clear | `-` |
| Navigate next | `arrowright` |
| Navigate previous | `arrowleft` |
| Navigate row down | `arrowdown` |
| Navigate row up | `arrowup` |
| Toggle fullscreen preview | `space` |
| Add to collection | `primary+shift+c` |
| Delete (soft) | `delete` (Mac: `backspace`) |

### Detail context

| Action | Default |
|---|---|
| Navigate next | `arrowright` |
| Navigate previous | `arrowleft` |
| Close detail / return to grid | `escape` |
| Toggle fullscreen | `space` |
| Zoom in | `primary+=` |
| Zoom out | `primary+-` |
| Open in app | `primary+o` |

---

## Storage

Default bindings live **in code** (`internal/keybindings/defaults.go`), per platform. The `keybindings` table (see schema doc) stores only user overrides, keyed by `(action, context)`. The effective binding set is defaults merged with overrides — an override wins for its action+context; an override with an empty `key_combo` means the user unbound a default.

```
action      TEXT    -- e.g. "rate_1", "flag_pick", "nav_next"
context     TEXT    -- "global", "grid", "detail", "import"
key_combo   TEXT    -- e.g. "1", "primary+z"; "" = unbound
```

Conflict detection (two actions on the same key in the same context) is enforced at the application layer against the **merged** set before writing an override, returning a typed error that the UI uses to show a helpful message ("This key is already bound to X. Reassign?").

Action constants are stable string identifiers defined in `internal/domain/keybindings.go`. They are the bridge between the stored string ("rate_1") and the application behaviour it triggers.

---

## Frontend implementation

### Binding cache

All bindings are loaded from Go at startup into a frontend lookup map:

```
bindings: { "grid:1": "rate_1", "global:primary+z": "undo", ... }
```

Key format: `"{context}:{key_combo}"`. O(1) lookup on every keypress. The cache is reloaded when the user changes a binding (the settings UI triggers a `keybindings:changed` event which the main app subscribes to).

### Central keydown handler

A single `keydown` event listener is registered at the document root, with `{ capture: true }` so it sees all events before they bubble. This is the only place keybindings are resolved.

```
document.addEventListener('keydown', (e) => {
    if (e.repeat) return  // ignore held keys for most actions

    const combo = resolveCombo(e)   // normalises to "primary+z" etc.
    const ctx = activeContext()     // "grid", "detail", "import", or "global"

    // try specific context first, fall through to global
    const action = bindings[`${ctx}:${combo}`] ?? bindings[`global:${combo}`]
    if (!action) return

    e.preventDefault()
    dispatch(action)
}, { capture: true })
```

`e.repeat` is checked to avoid firing repeatedly on held keys. Most actions should not fire repeatedly. (Zoom might be an exception — consider making repeat opt-in per action.)

### resolveCombo

Normalises a `KeyboardEvent` into the platform-neutral combo string:

```
function resolveCombo(e: KeyboardEvent): string {
    const parts = []
    if (e.metaKey || e.ctrlKey) parts.push('primary')  // Cmd or Ctrl → 'primary'
    if (e.altKey) parts.push('alt')
    if (e.shiftKey) parts.push('shift')
    parts.push(e.key.toLowerCase())
    return parts.join('+')
}
```

Both `metaKey` (Cmd on Mac) and `ctrlKey` (Ctrl on Win/Linux) produce `primary`. The frontend never knows which physical key was pressed — only the normalised combo.

### activeContext

Returns the current context string based on app state:

```
function activeContext(): string {
    if (importModalOpen) return 'import'
    if (detailPanelFocused) return 'detail'
    if (gridFocused) return 'grid'
    return 'global'
}
```

This is simple application state — no complex focus tracking needed because the app has a clear modal hierarchy.

### dispatch

Routes action strings to the appropriate handler:

```
function dispatch(action: string) {
    switch (action) {
        case 'rate_1': commands.SetRating(selectedAssetIDs, 1); break;
        case 'undo':   commands.Undo(); break;
        case 'nav_next': navigateNext(); break;
        // ...
    }
}
```

Some actions call Go backend commands (`SetRating`, `Undo`). Others are pure frontend state changes (`navigateNext` moves the cursor in the grid). The distinction is clear: anything that modifies the catalog goes to Go; anything that only changes what's visible goes to frontend state.

---

## Conflict detection

When the user tries to assign a key combo that is already bound in the same context:

1. Frontend calls `App.SetKeybinding(action, context, keyCombo)`
2. Go checks: `SELECT * FROM keybindings WHERE context = ? AND key_combo = ?`
3. If found and `action` differs: return `ErrKeybindingConflict { Combo, ConflictAction }`
4. Frontend shows: "The key `1` is already bound to `Rate 1 star` in grid context. Reassign?"
5. If user confirms: delete the conflicting binding, write the new one
6. If user cancels: no change

---

## Reset to defaults

The "Reset keybindings to defaults" action is simply `DELETE FROM keybindings` — with no overrides, the in-code defaults apply. Presented as a destructive action with a confirmation.

"Reset this binding" per-binding is `DELETE ... WHERE action = ? AND context = ?`.

---

## Adding new actions

When adding a new user-facing action:

1. Add a constant to `internal/domain/keybindings.go`: `ActionNewThing = "new_thing"`
2. Add a default binding to the platform default sets in `internal/keybindings/defaults.go`
3. Add the case to the frontend `dispatch` function

No migration is needed — defaults live in code, so existing users pick up the new binding on update automatically (unless they have an override on that key, in which case the conflict is surfaced in the settings UI).
