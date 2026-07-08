# impl/05 — Watcher Service

**Status (2026-07-07): impl/05 COMPLETE. 05.1 (matrix extensions, `internal/importer`) DONE and kept; 05.2 (the `internal/watcher` sensor) and 05.3 (connectivity via poll timer + `reconcile.go` retired) rebuilt against the corrected architecture and DONE. The rebuild fixed the first cut's drift (the watcher was deciding actions and writing `file_status`); the watcher now hands over PATHS only and makes exactly one write (connectivity). Rename policy settled at close-out: an unpaired name-changing rename is recorded as a *probable move* for review, not auto-relinked (see DEFERRED §5). See "Corrected architecture" below.**

> **SUPERSEDED IN PART by D20 (2026-07-07) — read this before trusting the move-detection
> details below.** After close-out we removed **all** auto-move machinery: the relink verdict
> (`actionMove`) and the **delete-side merge** (`healMovedAway`/`FindMoveHealCandidate`) are
> **deleted**. Reconciliation now *detects and flags* — it never auto-mutates identity. So every
> "gone → mark missing + delete-side merge" and "relink" description below is historical: the
> importer's single-path entry now does **gone → mark missing** (full stop), and a file that
> reappears at a NEW path is a new asset + a pending review row, not a relink. Same-path
> reappearance still restores via reimport. See decision **D20** and **DEFERRED §5** for the
> current model; the watcher *sensor* architecture (paths not verdicts, poll connectivity) is
> unchanged.
**Scope:** new `internal/watcher` + matrix extensions in `internal/importer`. **References:** D14, D9.

> **Corrected architecture (2026-07-07) — the boundary that must not blur.** The first watcher build
> drifted because the doc's old "deleted → mark missing" line read as a *watcher* action. It isn't.
> The clean split, now enforced in the Prime Directives:
>
> - **Watcher = sensor.** Owns the dirty-path set, debounce, the settle-stat, and graduation rules.
>   On graduation it hands the importer a **path, never a verdict**. It schedules reconciles (calls the
>   importer's full walk) on overflow / poll / remount. It makes exactly **one** catalog write of its
>   own: `sources.connectivity` (mount/unmount — connectivity **(a)**: the watcher writes it directly,
>   since it is what detects mount state). It does **not** stat-to-decide-an-action, and does **not**
>   write `file_status`.
> - **Importer single-path entry = the actor.** Receives a path, stats it, and decides: present →
>   ingest (matrix); gone → mark missing + delete-side merge. This is the *same* decision the full
>   walk makes per item, in one place — so a watcher-fed delete heals identically to a walk-detected
>   one. Mark-missing lives HERE, not in the watcher.
> - **No `Ingester` / `Fidelity` interface split.** The watcher depends on the importer's single-path
>   entry (one seam) plus its own connectivity write. There is no separate "fidelity" surface the
>   watcher performs — that framing was what let the sensor start acting.
> - **Batch reconcile is importer-side** (`Run`, full-walk mode); the watcher only *schedules* it.
>   The standalone per-file `Reconcile` variant (dev harness / future loose-files) is likewise a
>   pipeline-family operation, not watcher-owned.
>
> **One instance per source; orchestration is a higher layer** (the app-host supervisor — DEFERRED §2),
> not the watcher's concern.

> **Cross-cutting design that surfaced building 05.2 lives in [`DEFERRED.md`](../DEFERRED.md), not here.**
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
> - **05.1 — Matrix extensions** ✅ **DONE and kept** (`internal/importer`, pure catalog logic, no
>   watcher): rename enrichment (paired rename waives the matrix name-match; hash+size still verify)
>   and delete-side merge (asset → missing + exact content/name in a *recently-minted, ZERO-judgment*
>   asset → absorb; the zero-judgment guard is what makes it always safe). Built: `classify(...,
>   renamed bool)` + `Importer.IngestRenamed`; `AssetRepo.FindMoveHealCandidate` / `DeleteByID`;
>   `pipeline.healMovedAway` folded into the walk-end `markMissing`. Tests in `pipeline_test.go`:
>   `TestRenameEnrichment_WaivesNameMatch` (+ control), `TestDeleteSideMerge_HealsCopyThenDelete`,
>   `TestDeleteSideMerge_GuardedByJudgment`. **Corrected-model follow-up (part of the rebuild):** the
>   importer's single-path entry must gain the gone→**mark missing + delete-side merge** branch, so a
>   watcher-fed delete heals identically to a walk-detected one (today only the walk-end path heals).
> - **05.2 — Watcher service** ✅ **DONE** (`internal/watcher`) against the corrected
>   architecture above. The first cut worked but blurred the boundary: it stat-decided in the watcher
>   and wrote `file_status` via a `MarkMissing`/`Fidelity` surface. Rebuilt so the watcher owns the
>   dirty set (500ms debounce), ignore-list at intake, settle-stat, and graduation; it hands the
>   importer a **path** (present or gone) and nothing else; overflow / kill-9 → schedule a full-walk
>   reconcile. `rjeczalik/notify` stays the one event-source file; `EvalSymlinks` root fix stays.
>   No `Ingester`/`Fidelity` split — one seam onto the importer's single-path entry. Deferred
>   (`ponytail:`): rename *pairing* (notify has no portable from→to link) — and, decided at
>   close-out, an unpaired rename (name changed) is **not** auto-relinked at all; it is recorded
>   as a *probable move* for user review (DEFERRED §5), because a 64KB+size fingerprint match is
>   not certain enough to silently merge two differently-named files. Also deferred: sidecar echo
>   check (nothing writes `xmp_hash` until impl/06).
> - **05.3 — Connectivity via poll timer** ✅ **DONE** (`internal/watcher/poll.go`): per-source poll
>   ticker → `mode` (`events ⇄ polling ⇄ offline`), root-stat probe via the pure `probeReachable`.
>   Unreachable → the watcher's **one** sanctioned write, `MarkConnectivityBySource` (assets
>   online→offline, never missing — connectivity **(a)**), then quiesce (the `offline` gate stops the
>   event loop feeding paths); reachable-again → online + schedule catch-up walk. Subscribe failure
>   (inotify watch-limit) → degrade to polling, never crash. `reconcile.go` **retired** — its
>   offline-flip moved here, its missing/restore half was already the pipeline walk. The connectivity
>   write is the watcher's own (not routed through a `Fidelity` interface); batch reconcile is the
>   importer's `Run` the watcher *schedules*. **Known gaps (deferred, `ponytail:`):** plain-stat probe
>   misses an unmount that leaves an empty mountpoint (needs the filesystem-UUID monitor); after
>   remount the unit stays poll-driven rather than re-subscribing live events. The `sources.connectivity`
>   *column* (`SourceRepo.SetConnectivity`) is not written yet — no consumer reads it (P3 health panel).
>
> **Deferred with `ponytail:` markers (add when the trigger fires):** per-OS mount daemons (poll
> covers correctness — add for latency) · P3 health panel (the per-source status snapshot feeds it;
> no UI consumes it yet) · XMP inbound trigger (impl/06 owns it).

## Prime directives

1. **Sensors, not actors — the watcher hands over PATHS, never verdicts.** The service makes exactly
   ONE catalog write of its own: `sources.connectivity` flips (mount/unmount — the watcher is what
   detects mount state; written via ObservationWriter). Everything else it produces is a *hint path*
   fed to the importer, or a reconcile-schedule request. It does **not** decide or perform any
   per-file catalog action — not ingest, and **not mark-missing**. A deleted path is fed to the
   importer exactly like any other path; the *importer* marks it missing.
2. **Events are hints; the importer re-derives truth from the filesystem.** The watcher never trusts
   *what* an event claims (create / write / delete / rename). It debounces the path and hands it
   over; the importer's single-path entry stats it and decides the action — present → ingest (matrix:
   new / reimport / relink / duplicate); gone → mark missing + delete-side merge. This is why event
   loss degrades freshness, never correctness, and why the "what is this path now?" decision lives in
   exactly ONE place (the importer) for both the full walk and a single fed path. The watcher **does**
   stat — for the settle check, to *time* the handoff — but a stat result is never turned into a
   catalog decision.
3. **The reconciler is a schedule, not a component**: it's the pipeline in full-walk mode
   (`kind='reconcile'`), triggered at startup (+2s), per poll timer, on remount, on demand, and on
   any watcher failure. The watcher *schedules* it (calls the importer's full walk); it does not own
   or perform reconciliation.

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
checked at intake (a .tmp storm never enters). On graduation: a settle check (double-stat, stable
size+mtime) confirms a *present* file has stopped changing before handoff; a path that is simply
*gone* is already terminal and graduates as-is. Either way the watcher feeds the **path** — never a
verdict — to the importer's single-path entry, which stats it and decides the action (present →
ingest; gone → mark missing). The settle stat only *times* the handoff; it does not choose the
action. Mid-processing invalidation: mtime changed while hashing/extracting → abandon, re-queue with
backoff. Overflow event from the OS → drop set for that source, schedule reconcile (one answer to all
failures).

## Matrix extensions that land WITH this milestone

- **Rename enrichment**: when the adapter delivers a paired rename, pass the pair; the matrix
  waives the name-match requirement for relink (hash+size still must verify). Unpaired halves →
  normal missing/create flow.
- **Delete-side merge**: on marking an asset missing, look up its (hash, size, name) among
  **recently minted** assets (same session or <10min, tunable const). Exact match AND the young
  asset has ZERO judgments → absorb: old identity adopts the young row's path; young row deleted;
  duplicates row cleaned if one was logged. Heals copy-then-delete "moves" from external apps.
  The zero-judgment guard is what makes this always-safe.
- **Event handling (the watcher does NOT route by type).** Every event — create, write, delete —
  just marks its path dirty. On graduation the path is fed to the importer, which stats and decides:
  present → ingest; gone → mark missing (NEVER auto-remove) + delete-side merge. Event *type* informs
  only two things, and never the catalog action: (a) debounce set membership, and (b) rename-pairing
  — a paired rename is passed as an enriched hint so the matrix can waive the relink name-match.
  "Deleted → mark missing" is therefore the *importer's* action on a gone path, not the watcher's.

**The single-path entry owns "what is this path now?".** Both the full walk's per-item logic and a
watcher-fed path go through the *same* importer decision so mark-missing and its delete-side merge
can never be bypassed or duplicated: stat → present → matrix (ingest); gone → mark missing + attempt
delete-side merge. The watcher supplies the path; it never supplies the verdict.

## Sidecar hints

Sidecar-classified paths route to the sidecar upsert + (future) XMP inbound trigger. **Echo check
lives here**: hash the sidecar; equal to the asset's `xmp_hash` (what we last wrote) → drop silently.

## Acceptance

- Save-storm fixture (temp write + rename + double-write within 400ms) → exactly one ingest.
- Move fixture (D20 — any new-path reappearance, same-name or not): original left missing + new
  path minted as a distinct asset + one pending review pair. Never auto-relinked/merged. A file
  reappearing at its ORIGINAL path is restored online via reimport.
- Copy-then-delete fixture → original stays missing with judgment intact + copy is a distinct
  asset + a pending review pair (no auto-merge — D20).
- Kill -9 the app during watch; restart → startup reconcile converges; no duplicate identities.
- Unmount/remount a test volume (or bind-mount simulation) → connectivity flips, catch-up
  reconcile fires, assets browsable while offline.
- inotify watch-limit simulation → source degrades to polling, log entry, no crash, no divergence
  after next reconcile.
