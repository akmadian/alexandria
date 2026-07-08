# Keyboard, Actions, and the Command Palette

**Status:** design locked 2026-07-07. Builds on the P1 keyboard-system requirements (action
registry, configurable bindings, contexts) — this doc extends them; it doesn't replace them.

## Action registry

Every user-invokable operation is a registry entry:

```
{ id, title, aliases, context predicate (when enabled), handler, default binding }
```

- Defaults live in code as data; user overrides in `keybindings.json` (settled in impl/11) — new
  actions auto-appear on update.
- Grows incrementally: each feature registers its actions as it lands. No big-bang build.
- Completeness/conflict enforcement per C10 (a binding referencing a missing action id, or two
  actions claiming one key in one context, fails the table test).

## Dispatch is two-keyed

```
(context, key) → action
```

Contexts: `global`, `grid`, `loupe`, `compare`, `cull`, `import`, `review`, `palette`. Platform-
normalized `primary` modifier (Cmd/Ctrl). **Asset type is NOT a dispatch dimension** — that would
be a mostly-empty 3D matrix. Instead, a small set of *media verbs* consult the cursor asset's
type inside their handler, via the frontend type registry.

## Verb grammar

- **Universal verbs — identical everywhere, never type-varying** (the muscle memory): navigate
  (arrows/J/K), rate (1–5, 0 clears), label (6–9, − clears), flag (P pick / X reject / U clear),
  view modes (G/E/C/D), Escape (step down toward Grid), open-in (O), quick preview (Space in
  Grid).
- **Media verbs — type-interpreted** (the Finder Quick Look precedent): **Space = "engage the
  asset"** — photo: toggle 100% zoom; video/audio: play/pause. Scrub, in/out points, etc. join
  this small set only where meaningful. Users accept verb-level consistency with type-sensitive
  semantics; they reject rating keys that move.

## Command palette (day one, not P2)

The palette ships with the keyboard system because it *is* the registry's face — and it teaches:
every entry shows its current binding. It also absorbs chrome: rare actions (Open Settings,
Export Logs, Rebuild Thumbnails) live here instead of spending toolbar real estate.

Implementation (deliberately boring):

- **Fuzzy subsequence matcher** (VS Code/fzf scoring: word starts, consecutive runs, shorter
  targets win) over titles + aliases, filtered by context predicate.
- **Frecency ranking** (recent + frequent floats up) — also the requirements' "recently used
  prioritized" applied here.
- **Prefix modes** (Sublime convention): bare text = search (parses via `03`'s tiers, emits
  pills), `>` = actions, `#` = go-to (collection/tag/source navigation).
- **Cmd+K** opens in action mode; **Cmd+F** (or `/`) opens the same palette in search mode — the
  global retrieval entry point.

The healthy-palette test: everything reachable in the palette has a home elsewhere too (except
deliberately-buried rare actions). A palette that's the *only* path to common operations is
covering for bad UX.

## Keybinding preset sets

- Named default sets in code: **Alexandria** (= LrC grammar), **darktable**, (Capture One
  candidate). First-run picker; user overrides layer on top of the chosen preset.
- LrC's shortcuts aren't user-configurable, so there's no LrC keymap file to import — LrC users
  just pick the preset. darktable (`shortcutsrc`) / digiKam import is a later nicety, filed with
  the external-tool migration surface (alongside the LrC catalog bootstrap, impl/09).
- Conflict detection on reassignment; reset per-binding or all (per P1 requirements).
