# impl/12 — App Host

**Status:** not started, no design round held. Created 2026-07-08 (backend audit) to give a home
to the work that keeps deferring to "the app-host milestone" — until now that milestone didn't
exist as a trackable thing, so its contents were scattered triggers with no owner.

**What it is:** the long-running process that owns a catalog for a session — the Wails v2
composition root wiring engine services to the UI, plus everything that only makes sense once a
process *stays up*: the startup sequence, service supervision, live settings reaction.

## Owned work (collected from existing triggers)

1. **Wails v2 wiring / composition root** — *split 2026-07-09; **built by impl/14 (DONE
   2026-07-09)**.* The composition root exists: `main.go` (thin entrypoint) + `app.go` (the
   `host` struct: resolve catalog → `sqlite.Open` → construct seam services → `Bind`) +
   `wails.json` + `build/`, with the walking-skeleton `ListSources` bound end to end. Bound
   methods and the event bridge continue in impl/15 ∥ impl/16. This milestone *grows the same
   host in place* with everything below — `newHost`/`onStartup`/`onShutdown` are the seams to
   grow into (their doc comments point back here).
2. **Startup sequence** (FR P0): resolve catalog dir → instance lock → open SQLite → migrations →
   integrity check (background) → wire dependencies → seed defaults → start watcher → update check
   → `app:ready` → background catch-up scan. Two hard exits (can't open DB, can't migrate);
   everything else degrades.
3. **Startup checks — named by the 2026-07-08 audit as ownerless, now owned here:**
   - `PRAGMA integrity_check` at startup (background, non-blocking, warn on failure) — not wired.
   - **Backup-before-migration** — the P0 floor: one `VACUUM INTO` (never raw file copy) before
     any pending migration runs. The backup *feature* (rolling, retention, destinations) is a
     separate design task — `../04-open-questions.md` #16.
4. **Watcher supervision** (`DEFERRED.md` §2): supervisor with restart+backoff, one unit per
   enabled source; per-source status snapshot; lifecycle wiring on enable/disable/connectivity.
5. **Live mid-run worker-pool resize** (`DEFERRED.md` §6): the `Machine.OnChange → run.Resize`
   hook — the composition root is the place it wires.

## Pre-design notes (2026-07-09, from the seam-round structure research)

**Structure, locked (Ari):** Wails v2 forces the main package at the project root (`wails
dev`/`build` run where `wails.json` lives; upstream declined `cmd/` layouts — wails issue
#2568). So the repo root gains `main.go` + `app.go` + `wails.json` + `build/` (graft from a
`wails init` template rather than hand-writing `build/`), and the bound seam services live in
`internal/seam`. `cmd/dev` stays as the throwaway harness. Wails v2 now; migrate to v3 once it
stabilizes — the thin-root + `internal/seam` split is what keeps that migration contained.

**App-home layout, decided (Ari, 2026-07-09, during impl/14):** one directory holds everything
Alexandria owns for a user — default `~/.alexandria` (env `ALEXANDRIA_HOME`), containing
`logs/` and the default `catalog/`. "Everything in one place, easy to find, copy, and back up."
The **catalog stays relocatable** (a self-contained dir of db + thumbnails + settings that can
live anywhere — beside the photos on an external drive; `ALEXANDRIA_CATALOG` overrides);
`~/.alexandria/catalog` is only the default. Logs are **app-level** (they span catalogs), so
they sit beside the catalog, never inside one. Deliberate trade-off vs. the macOS-native split
(`~/Library/Application Support` + `~/Library/Logs` + `~/Library/Caches`): the one-dir model wins
on portability/backup, and a platform-gated symlink (`~/Library/Logs/Alexandria → <home>/logs`,
macOS only) buys back Console.app's Log Reports browsability. Thumbnails-in-catalog (vs.
`~/Library/Caches`) was already the model, so nothing regresses. The resolution + logging live in
`internal/app` (kept out of the webkit-coupled root package so they stay unit-testable).

**Code-state audit (verified 2026-07-09)** — what the startup sequence can reuse vs. must build:

| Piece | State |
|---|---|
| Instance lock | Built — `sqlite/lock_unix.go` (Windows placeholder, DEFERRED §3) |
| Migrations | Built — `migrations.Migrate` |
| Settings service | Built — `settings.Open` (impl/11); machine/keybindings move to `<app-config-dir>` here |
| Watcher (single unit) | Built — `Watcher.Run`; supervision unbuilt (DEFERRED §2) |
| `PRAGMA integrity_check` wiring | Not built |
| Backup-before-migration (`VACUUM INTO`) | Not built |
| Update check | Not built (settings field was YAGNI-dropped in impl/11; returns with this) |
| Catalog-dir resolution | **Seeded** by impl/14 — `internal/app.CatalogDir` (env `ALEXANDRIA_CATALOG` override, else `<app-home>/catalog`). This milestone grows the first-run picker + recent-catalog list. |
| App home + logging | **Seeded** by impl/14 — `internal/app` resolves the one-dir app home (`~/.alexandria`, env `ALEXANDRIA_HOME`), holding `logs/` + the default `catalog/`; `app.SetupLogging` writes per-run timestamped files there via `internal/logging`, and on macOS symlinks `~/Library/Logs/Alexandria → <app-home>/logs` so Console.app surfaces them. This milestone owns retention, a settings-driven level, and the real dir-resolution UX. |

**Wails v2 idioms to map the design onto (verify against docs during the round):**
- Lifecycle hooks: `OnStartup(ctx)` is where the startup sequence runs (and where the
  events-capable context lives); `OnBeforeClose` = exit veto/confirm; `OnShutdown` = ordered
  teardown. The startup-sequence design is effectively the body of `OnStartup` plus what must
  precede `wails.Run`.
- `options.SingleInstanceLock` (app-level, v2.9+) is a *different concern* from our per-catalog
  DB lock — likely both; decide in the round.
- The binary channel (seam/02) maps to `assetserver.Options` handler middleware — the
  thumbnail/preview/original URL scheme is served there, never via bound methods.
- CI: ubuntu-latest (24.04) ships webkit2gtk-4.1 only → the root package builds with
  `-tags webkit2_41` once the wails dependency lands (apt packages added to ci.yml 2026-07-09;
  the build tag lands in the Makefile with impl/14).

## Not owned here

- The catalog backup system proper — design task, `../04-open-questions.md` #16.
- Windows instance-lock hardening (`DEFERRED.md` §3) — Windows QA pass trigger, unchanged.
- Frontend implementation — its own area (`../../frontend/`).

**Trigger to start:** ~~the seam round completes~~ — impl/14 has landed the host (2026-07-09) and
impl/15 ∥ impl/16 are filling in the contract in parallel. The next trigger for *this* milestone
is the **startup-sequence design round**: design it first; it's the piece with no existing spec
beyond the FR bullet list, and it grows the impl/14 host in place.
