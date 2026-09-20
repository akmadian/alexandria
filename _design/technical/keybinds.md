# Keybinds and targeting: the command and focus architecture

Keybind/targeting round, 2026-09-20. This brief records the ratified shape
of keyboard routing, the command registry, and key-focus ownership — plus
the platform facts that were paid for during the round, so future sessions
inherit them instead of re-litigating. Register: rulings marked RATIFIED
are Ari's.

## The spine (RATIFIED)

1. **AppKit is the routing system, end to end.** Key events route through
   the responder chain and the menu bar; there is no central dispatcher and
   no event monitor. (A monitor was built on a misread experiment in an
   earlier round and struck; do not resurrect it.)
2. **The menu bar IS the command registry.** Every keyboard command is a
   real NSMenuItem declared in `MainMenu` and nowhere else — no parallel
   command table, no chord declared at any other site. The menu is the OS
   surface: System Settings remapping matches menu titles, Help search
   finds items, Full Keyboard Access reaches them. Toolbar buttons call the
   same verbs and declare no chords.
3. **Enablement and checkmarks are PULL** (`validateMenuItem`): AppKit asks
   at menu-open and key-dispatch time and the answer reads the hub fresh.
   A stale or dead menu item is unrepresentable by construction.
4. **The AppKit lifecycle** (RATIFIED after research): the app shell —
   NSApplication, delegate, window, menu — is AppKit; everything inside the
   window is SwiftUI via NSHostingController (toolbars/title bridge via
   sceneBridgingOptions, macOS 14+). Chosen because the SwiftUI lifecycle
   co-owns NSApp.mainMenu and reasserts its own menu on scene updates, so a
   hand-built menu can never stick (measured in-app; corroborated in the
   wild). Costs accepted and named: Settings window and any future scene
   feature get manual AppKit equivalents when their rounds arrive.

## Targeting (RATIFIED)

- **Commands act on the hub's selection/cursor** (`judgmentTargets`), never
  "the focused pane" — the Lightroom/Photos family, not Capture One's
  split-focus (a documented user-confusion source). Rating 1–5 works the
  same wherever focus sits, except inside a text editor.
- **Focus decides only where navigation keys go** (arrows, page, home/end).
- **Unmodified letter chords are safe beside text fields** because AppKit
  routes bare keys to the first responder BEFORE the menu: a focused field
  consumes the letter as text; only unclaimed keys reach the menu.
- **A focused sidebar owns its typed keys natively** (type-select, arrow
  navigation) — no shim suppresses it.

## Key-focus ownership (RATIFIED)

- **The stage's active renderer claims key focus from limbo** — when the
  window itself holds first responder, AppKit's "nobody does" — on window
  attach. Never from a live responder: full-screen and split-view rebuilds
  that re-attach views steal nothing.
- **Choosing a source hands the keyboard to the stage**: when a new
  source's answer lands, the grid claims focus even from a live pane — the
  one sanctioned steal, because it completes the user's own gesture.
  Keyed on the SOURCE alone; filter and arrangement changes never steal.
- **Quick Look never takes key focus** (window-level veto): the loupe
  surface's key owner is its host; the placeholder QLPreviewView is
  display-only. The veto dies with QLPreviewView at the loupe round.
- The loupe's real viewer inherits this whole contract at its round:
  claim from limbo, implement the navigation selectors against the hub,
  bubble everything else to the menu.

## Menu taxonomy (RATIFIED)

File / Edit / **Asset** (the domain menu, our noun) / View, plus the free
standard menus. Named empty landing slots: a **Metadata** menu when
keywords/tags arrive; a **Collection/Library** menu if collection
operations outgrow File and the sidebar. Chords follow Lightroom's
vocabulary (p/x/u, 1–5, 0, g/e) so trained fingers arrive working.

## Platform facts paid for (do not re-litigate)

- A DISABLED menu item **swallows** its key equivalent: NSMenu's search is
  first-match-wins and does not continue to an enabled duplicate (measured
  2026-09-20). Consequences: no two items may share a chord unless at most
  one is ever present; nothing registers a command before its verb exists.
- Content-dependent chords (space = play/pause vs zoom) therefore belong
  to the stage surface that knows the content, via the responder chain —
  and when a real colliding pair ships, the inapplicable menu twin is
  HIDDEN, not grayed (RATIFIED; mechanics at the loupe round).
- SwiftUI `Commands` enablement is workable in production only through
  explicit invalidation plumbing (cf. CodeEdit's custom property wrapper
  and NSMenuItem swizzles) — push, not pull. Rejected here for the pull
  model, not because SwiftUI is "unworkable".
- NSCollectionView routes every keystroke through its text key-binding
  machinery (beeps on printables, no off switch): `GridCollectionView`
  keeps function-flagged keys and forwards the rest up the chain. The
  loupe key host mirrors the same split.
- `⌘+` as a declared equivalent never matches on a US layout; the zoom-in
  chord is `=`.

## Deliberately unsettled

In-app shortcut editor (System Settings remapping of bare letters is
UNVERIFIED; the KeyboardShortcuts library's recorder is a candidate for
that round). Move-focus commands (⌘J-style). Delete-key semantics.
Space-pair menu mechanics and loupe-mode source-handoff (loupe round).
Focus-return when real inline text editors appear. Multi-window.
