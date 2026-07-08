# Review

**Status:** design locked 2026-07-07; naming settled as **Review** for now ("the Desk" — library
circulation desk — noted as the whimsy alternative; UI copy is i18n data, renameable forever).
This is the frontend face of backend D20 (detect-and-flag, never auto-mutate) and DEFERRED §5
(the duplicates/moves review projection).

## What it is

One first-class surface collecting everything the engine *noticed but refused to decide*:

| Category | Source |
|---|---|
| Probable moves / renames | matrix's pending `duplicates` rows (kind derived from live file_status, per impl/05 close-out) |
| Duplicates | same content hash, both present |
| Missing files | file_status = missing (with relocate flow) |
| XMP conflicts | sync-state, per impl/06 conflict grid |
| Import errors | import_errors DLQ |
| Suggested rejects / proposed groups | signals (`06`), when built |

Nothing is ever silently mutated; everything noticed is *presented*. This is the positioning
("never acts behind your back") made visible in chrome — LrC scatters these across grayed
folders, ?-badges, and modal dialogs; nobody in the space does this well.

## Presentation

- **Sidebar item with count badge** — ambient awareness, always visible, never modal. In-grid
  corner ticks on affected assets (`02`).
- **Opening it is a task view** (C3): enter, process, leave. Full-window because processing the
  queue is task-shaped — evaluate, decide, done.
- **Inbox grammar without the inbox name**: a categorized list processed top-to-bottom,
  keyboard-forward, bulk actions, zero-when-done. Creative pros already know how to process a
  queue; borrow the grammar, not the email connotations.
- Per-category bespoke row UX: moves show old path → new path with confirm-relink; duplicates get
  side-by-side with metadata comparison (keep both / remove one / link as group); missing files
  get the relocate flow; XMP conflicts show both sides + the policy that would apply.
- Bulk resolution is the norm ("confirm all 34 relinks"), single-item inspection the exception.

## Resolution actions

Each category's actions are the *user-granted* versions of what D20 removed from the engine:
confirm move (relink — the deferred `DeleteByID` consumer), keep both, merge metadata, remove
from catalog, delete from disk (double-confirmed, never undoable), resolve XMP per side, retry
import errors. All catalog-editing resolutions ride the command pattern (undoable) where the
backend contract allows.

## Deferred: automation rules

The eventual power feature: "stop making me take the same action over and over" — e.g. "always
relink moves within the same source when hash+size+name match." Deliberately deferred until
Review v1 has real usage showing *which* repetitions hurt.

Two decisions reserved now so the door stays open:

1. **Automation is a user grant, never an engine default** (D20's future-direction note) — rules
   are opt-in, per-source or global, visible and revocable in one place.
2. **Rule conditions reuse the query token vocabulary** (`04`) — a rule is "when an event
   matching *this filter* arrives, do *this action*." No second condition language.
