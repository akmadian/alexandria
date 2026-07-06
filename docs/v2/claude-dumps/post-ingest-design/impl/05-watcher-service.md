# impl/05 — Watcher Service

**Status: design complete; DO NOT START until the ingest pipeline (impl/04) ships and stabilizes.**
**Scope:** new `internal/watcher`. **References:** D14, D9.

## Prime directives

1. **Sensors, not actors.** The service never writes the catalog. Its only outputs: hint paths fed
   to the pipeline, `sources.connectivity` flips (the one sanctioned observation write, via
   ObservationWriter), and reconcile-schedule requests.
2. **Events are hints, not facts.** Truth is re-derived by the pipeline's matrix from the
   filesystem. Event loss degrades freshness, never correctness.
3. **The reconciler is a schedule, not a component**: it's the pipeline in full-walk mode
   (`kind='reconcile'`), triggered at startup (+2s), per poll timer, on remount, on demand, and on
   any watcher failure.

## Structure

One service; per-source **units**, each a state machine: `events` (local FS, adapter running) ⇄
`polling` (network sources always; locals after degradation) ⇄ `offline` (volume gone). The
service owns: the dirty-path set, the debouncer, schedules, and a status snapshot per source
(mode, last reconcile, dirty count) — this feeds the status bar and P3 health panel.

**Event adapters** per-OS in build-tagged files: FSEvents (macOS — NOT kqueue), inotify (Linux —
per-directory watches; on `max_user_watches` exhaustion → degrade that source to polling),
ReadDirectoryChangesW (Windows). Adapter output is normalized: `{path, op: create|write|remove|
rename, renamePair?: {from, to}}`.

**Volume monitor** (one, machine-level, per-OS): DiskArbitration (macOS), epoll on
`/proc/self/mountinfo` (Linux — no daemon dep), WM_DEVICECHANGE (Windows). Mount → read fs UUID
(`/dev/disk/by-uuid`, DiskArb props, volume GUID), match `sources.filesystem_uuid` — NEVER match by
mount path — → connectivity=online → schedule reconcile. Unmount → sources under that mountpoint →
offline, quiesce unit. **Yanked drive**: EIO/ENODEV from any in-flight stat at a source root →
probe → treat failed probe as unmount.

## The debounced dirty set

A SET (dedupes storms) of paths, each with a 500ms timer reset on every new event. Ignore-list is
checked at intake (a .tmp storm never enters). On graduation: settle check (double-stat, stable
size+mtime) → feed pipeline single-path entry. Mid-processing invalidation: mtime changed while
hashing/extracting → abandon, re-queue with backoff. Overflow event from the OS → drop set for
that source, schedule reconcile (one answer to all failures).

## Matrix extensions that land WITH this milestone

- **Rename enrichment**: when the adapter delivers a paired rename, pass the pair; the matrix
  waives the name-match requirement for relink (hash+size still must verify). Unpaired halves →
  normal missing/create flow.
- **Delete-side merge**: on marking an asset missing, look up its (hash, size, name) among
  **recently minted** assets (same session or <10min, tunable const). Exact match AND the young
  asset has ZERO judgments → absorb: old identity adopts the young row's path; young row deleted;
  duplicates row cleaned if one was logged. Heals copy-then-delete "moves" from external apps.
  The zero-judgment guard is what makes this always-safe.
- Event routing per FR: created/modified → hint; deleted → mark missing (NEVER auto-remove);
  rename → enriched hint.

## Sidecar hints

Sidecar-classified paths route to the sidecar upsert + (future) XMP inbound trigger. **Echo check
lives here**: hash the sidecar; equal to the asset's `xmp_hash` (what we last wrote) → drop silently.

## Acceptance

- Save-storm fixture (temp write + rename + double-write within 400ms) → exactly one ingest.
- Rename fixture: paired events relink (judgments preserved); simulated unpaired → missing then
  heals on next reconcile.
- Copy-then-delete fixture → delete-side merge preserves judgments, no stranded missing row.
- Kill -9 the app during watch; restart → startup reconcile converges; no duplicate identities.
- Unmount/remount a test volume (or bind-mount simulation) → connectivity flips, catch-up
  reconcile fires, assets browsable while offline.
- inotify watch-limit simulation → source degrades to polling, log entry, no crash, no divergence
  after next reconcile.
