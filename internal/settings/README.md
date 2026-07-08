# internal/settings

User/machine/keybinding config, stored as **plain JSON files** — not a catalog DB
table. Files were chosen so a user can hand-edit them while the app runs; every
open file is watched and hot-reloaded. Each is **created with defaults on first
run**, the same strategy `sqlite.Open` uses for the catalog DB. Full rationale in
[`impl/11-settings-service.md`](../../_project-tracking/backend/impl/11-settings-service.md).

## Three files, three scopes

| Type | File | Scope | Lifetime |
|------|------|-------|----------|
| `Settings` | `settings.json` | per-catalog (XMP policy, ignore list, import defaults, opaque UI state) | opened with the catalog, `Close`d with it |
| `Machine` | `machine.json` | per-machine (worker counts, memory limit, dep paths) | process lifetime |
| `Keybindings` | `keybindings.json` | per-user (overrides only; defaults live in frontend) | process lifetime |

`settings.Open(dir, logger)` opens all three in `dir` and returns a `*Service`.
**Today `dir` is `<catalog-dir>/settings/`** — all three colocated with the catalog
(what the dev harness wires: `dev import <path> --catalog <dir>` creates them under
`<dir>/settings/`). `machine.json`/`keybindings.json` are app-scoped by design and
should live in an OS app-config dir outliving any one catalog; they sit here
provisionally until the app-host milestone resolves `<app-config-dir>` and splits
them out. The file a value lives in *is* its scope tag; there's no ownership enum.

## Shape

One generic `configFile[T]` ([`configfile.go`](configfile.go)) does everything —
tolerant load, cached value, atomic save, debounced hot-reload watch, change
notification. `Settings`/`Machine`/`Keybindings` are just `*configFile[T]` with
different defaults and sanitizers ([`settings.go`](settings.go)). The composition
root holds all three via `Service` and is the *only* caller.

```go
cfg, _ := settings.OpenSettings(catalogDir, logger)
defer cfg.Close()
cfg.Get()                       // cached struct — no per-read file I/O
cfg.OnChange(func(s Settings){}) // fires on UI Save *and* external edit
cfg.Save(next)                   // marshal indented, atomic temp+rename, update cache, fire callbacks
```

## The one thing to understand: cold load ≠ live reload

They fail differently **on purpose**:

- **Cold load** (at open, no prior value): missing file → write defaults and use
  them. Bad JSON → **quarantine** the file (`.invalid-<UTC>` sibling, preserved not
  deleted) and boot on defaults, but do *not* auto-rewrite (so the user can inspect
  the quarantined copy; the next `Save` writes a fresh one). A malformed file must
  never block startup.
- **Live reload** (`reloadJSON`, from the watcher): bad JSON → **keep serving the
  last-known-good value, leave the file untouched.** The user is probably mid-edit;
  quarantining or reverting live behavior mid-run is more disruptive than waiting.

Valid-but-bad *values* (e.g. `"hash": -3`) degrade **per-field**, not per-file — a
sanitizer clamps non-positive/unknown fields back to defaults so one bad field
doesn't nuke the other good ones. No schema-validation library; a dozen fields
don't need one.

## Owns the D18 ignore list — data *and* matching

`Settings.IgnorePatterns` is the source of truth for the ignore list, so **all** the
matching lives here too ([`ignore.go`](ignore.go)): `Settings.MatchIgnore(name)` (the
matched pattern, for SCAN's per-pattern skip tally) and `Settings.Ignored(name)` (a
bool, the watcher's intake filter). Consumers hold a `settings.Settings` value and call
these directly — no ignore logic of their own. The zero value is the null object: an
empty `Settings` has no patterns, so `MatchIgnore` returns `""` — a bare `Importer{}`
ignores nothing, no nil-guards needed. `internal/importer` used to own a baked list
(`ignore.go`); that's gone.

## Watching & boundaries

- The watch is on the file's **parent directory**, not the file — an editor's
  atomic-save (rename over the file) would sever a file-inode watch. Events are
  filtered by basename and debounced 300ms (editors emit several per save).
- `Save` is atomic (temp+rename), so the watcher never sees a half-written file and
  our own writes round-trip to a no-op reload — no self-write suppression needed.
- **This package is a leaf**: it imports no other internal package. `importer`,
  `watcher`, `xmp` never import it — they take plain config via constructor fields;
  the composition root bridges `OnChange` to them. Keep it that way (there's a
  `go list -deps` acceptance check enforcing it).

## Not here yet

Live mid-run pipeline resize (spec §5) — deferred to the app-host milestone, since
the `Machine.OnChange → run.Resize` hook lives in the composition root and there's
nothing to wire it to today.
