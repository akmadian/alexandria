# impl/12 — App Host

**Status:** not started, no design round held. Created 2026-07-08 (backend audit) to give a home
to the work that keeps deferring to "the app-host milestone" — until now that milestone didn't
exist as a trackable thing, so its contents were scattered triggers with no owner.

**What it is:** the long-running process that owns a catalog for a session — the Wails v2
composition root wiring engine services to the UI, plus everything that only makes sense once a
process *stays up*: the startup sequence, service supervision, live settings reaction.

## Owned work (collected from existing triggers)

1. **Wails v2 wiring / composition root** — the seam methods bound, generated TS models, event
   bridge. Sequenced after the seam round (see the master head's dependency tree).
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

## Not owned here

- The catalog backup system proper — design task, `../04-open-questions.md` #16.
- Windows instance-lock hardening (`DEFERRED.md` §3) — Windows QA pass trigger, unchanged.
- Frontend implementation — its own area (`../../frontend/`).

**Trigger to start:** the seam round completes (there is a contract to host). Design the startup
sequence first; it's the piece with no existing spec beyond the FR bullet list.
