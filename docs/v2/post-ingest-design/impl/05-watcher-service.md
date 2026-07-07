# impl/05 — Watcher Service

**Status: IN PROGRESS (2026-07-07). 05.1 (matrix extensions) + 05.2 (watcher service) DONE; 05.3 (poll-timer connectivity) remaining.**
**Scope:** new `internal/watcher` + matrix extensions in `internal/importer`. **References:** D14, D9.

> **Cross-cutting design that surfaced building 05.2 lives in [`DEFERRED.md`](DEFERRED.md), not here.**
> The watcher forced the question "when may the catalog change on its own?" — which opened the import/
> tracking model (one-shot import vs. watched folders; `sync_mode` = Manual/Scheduled/Watched;
> loose-files vs. directories; cross-source duplicates → user action; source-scoping the auto-mutating
> matrix lookups). All of that is recorded in `DEFERRED.md §1`, deferred to the source-management
> milestone (cleanly additive — see its Urgency note). This spec covers only the watcher *mechanism*.

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
> - **05.2 — Watcher service** ✅ **DONE (2026-07-07)** (`internal/watcher`): debounced dirty SET
>   (500ms/path + 50ms settle double-stat), ignore-list at intake (`importer.Ignored`),
>   `rjeczalik/notify` normalized → hints, overflow → drop set + reconcile, startup reconcile
>   (kill-9 recovery). **Key simplification vs. the prose below:** graduation *re-derives truth from
>   the filesystem* — a dirty path that exists is ingested (`IngestFile`), one that's gone is marked
>   missing (`importer.MarkMissing`, the sole delete observation). So the watcher never branches on
>   OS event type (create/write/delete/rename all just mark the path dirty); the stat at graduation
>   is the fact. Hosted via `cmd/dev watch <path>`. Files: `watcher.go` (service + debounce loop),
>   `source_notify.go` (the one file touching the notify backend), `event.go`. Tests
>   (`watcher_test.go`, race-clean, fake event source): save-storm → one ingest, ignore-at-intake,
>   delete → missing, overflow → reconcile; plus a live FSEvents smoke through `cmd/dev watch`.
>   **Deferred with `ponytail:` markers:** rename *pairing* (notify gives no portable from→to link;
>   the 05.1 `IngestRenamed` seam waits for an inotify-cookie enhancement — an unpaired rename
>   degrades to missing+duplicate, healed by reconcile) · sidecar echo check (nothing writes
>   `xmp_hash` until impl/06, so it has nothing to echo against — YAGNI until then) · the full
>   `events ⇄ polling ⇄ offline` state machine (05.3 owns connectivity). **macOS gotcha handled:**
>   `Run` canonicalizes the root via `EvalSymlinks` so `/var`→`/private/var` doesn't make every
>   FSEvents path look like it escaped the tree.
> - **05.3 — Connectivity via poll timer** (remaining): startup reconcile (+2s), per-source poll timer,
>   EIO/ENODEV probe → offline + quiesce unit, root reappears → online + catch-up reconcile.
>   `MarkConnectivityBySource` is the only observation write. Acceptance: unmount/remount flips
>   connectivity, assets browsable while offline; inotify-limit sim degrades to polling.
>   **Scope grew out of the 05.2 discussion:** the same poll timer is also the **"Scheduled" sync tier**
>   (`DEFERRED.md §1`), and the per-file `Reconcile` it wraps is the **loose-file fidelity primitive** —
>   so `Reconcile` ([reconcile.go](../../../../internal/importer/reconcile.go)) is **no longer slated
>   for removal**; its whole-source-offline branch moves to the volume monitor, but its per-file
>   stat-and-flip logic likely earns a permanent home rather than retiring. Decide when wiring 05.3.
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
