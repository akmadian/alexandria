# impl/05 — Watcher Service

**Status: IN PROGRESS (started 2026-07-07). impl/04 shipped and stabilized (2026-07-06), so this is unblocked.**
**Scope:** new `internal/watcher` + matrix extensions in `internal/importer`. **References:** D14, D9.

> **Reconciled plan (2026-07-07).** The original design below stands as the target; this section
> is how we actually build it and where we deliberately spend less than the letter of the spec.
> Where the two disagree, this section wins (same convention as impl/04).
>
> **Two rungs collapse most of the milestone.** The spec's six OS-specific subsystems (three cgo
> event adapters + three volume monitors) become one dependency plus a timer:
>
> - **Event adapters → `github.com/rjeczalik/notify` (pinned).** It wraps exactly the three backends
>   the spec names — recursive **FSEvents** on macOS (NOT kqueue, which the spec rejects for the same
>   reason we do: fd-per-file, no recursion), **inotify** on Linux (surfaces the watch-limit/overflow
>   signals we degrade on), **ReadDirectoryChangesW** on Windows. We keep the event-source boundary
>   to ONE normalize function (`notify.EventInfo → {path, op, renamePair?}`) with a `ponytail:` marker,
>   so swapping the backend later is a one-file change. Hand-rolling three cgo adapters is rung-6 work
>   when rung-4 is sitting right there. (fsnotify was considered and rejected: kqueue on macOS, no
>   recursion — wrong for large photo trees.)
> - **Volume monitor → the poll timer we already owe.** Network sources poll on a timer regardless
>   (D14). Remount detection is that same timer stat-probing the source root and flipping connectivity
>   + scheduling a reconcile. This covers mount/unmount/yanked-drive **everywhere with zero per-OS
>   code**. Ceiling: a remount is noticed within one poll interval, not instantly. Deferred with a
>   `ponytail:` marker; add DiskArbitration / mountinfo-epoll / WM_DEVICECHANGE only when that latency
>   is measured to matter.
>
> **Staging** — three PRs, correctness-first (the judgment-preservation logic has zero platform risk
> and is the sacred part, so it lands before any OS code):
>
> - **05.1 — Matrix extensions** ✅ **DONE (2026-07-07)** (`internal/importer`, pure catalog logic, no
>   watcher): rename enrichment (paired rename waives the matrix name-match; hash+size still verify)
>   and delete-side merge (asset → missing + exact content/name in a *recently-minted, ZERO-judgment*
>   asset → absorb; the zero-judgment guard is what makes it always safe). Built: `classify(...,
>   renamed bool)` + `Importer.IngestRenamed` seam for 05.2; `AssetRepo.FindMoveHealCandidate` /
>   `DeleteByID`; `pipeline.healMovedAway` folded into the walk-end `markMissing` (a "move" now heals
>   instead of stranding a missing row). Tests in `pipeline_test.go`: `TestRenameEnrichment_
>   WaivesNameMatch` (+ negative control), `TestDeleteSideMerge_HealsCopyThenDelete` (judgment
>   preserved, dup row cascade-cleaned), `TestDeleteSideMerge_GuardedByJudgment`. The rename-true
>   runtime path (mark the from-path missing, then `IngestRenamed` the to-path) is wired by 05.2.
> - **05.2 — Watcher service** (`internal/watcher`): per-source unit state machine
>   (`events ⇄ polling ⇄ offline`), debounced dirty SET (500ms/path + settle double-stat), ignore-list
>   at intake (reuse `importer` ignore rules), `rjeczalik/notify` normalized → `IngestFile`, overflow/
>   watch-limit → drop set + schedule reconcile, sidecar echo check (hash == asset's `xmp_hash` → drop
>   silently). Hosted via a new `cmd/dev watch <path>` subcommand — there is no long-running app
>   process yet (Wails deferred), so the dev harness is the only host. Acceptance: save-storm → one
>   ingest; kill-9 → startup reconcile converges.
> - **05.3 — Connectivity via poll timer**: startup reconcile (+2s), per-source poll timer, EIO/ENODEV
>   probe → offline + quiesce unit, root reappears → online + catch-up reconcile.
>   `MarkConnectivityBySource` is the only observation write. Retires `Reconcile`'s offline branch
>   ([reconcile.go](../../../../internal/importer/reconcile.go)) once it lands. Acceptance:
>   unmount/remount flips connectivity, assets browsable while offline; inotify-limit sim degrades to
>   polling.
>
> **Deferred with `ponytail:` markers (add when the trigger fires):** per-OS mount daemons (poll
> covers correctness — add for latency) · P3 health panel (the per-source status snapshot struct is
> built and populated; no UI consumes it yet) · XMP inbound trigger (impl/06 owns it — the echo check
> ships here, but a sidecar that survives the echo check just routes to the sidecar upsert for now).

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
