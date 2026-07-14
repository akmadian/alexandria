# Deferred & Open Questions — a running ledger

Things surfaced *during* implementation that we deliberately chose **not** to build
yet, captured so "later" doesn't quietly become "never." This complements the
design-level [`ideation/backend-open-questions.md`](ideation/backend-open-questions.md) (which predates the
code) and the inline `ponytail:` markers (per-site shortcuts). This file is for
cross-cutting deferrals that don't belong to a single milestone spec.

Each entry: the problem, why it's deferred, the current leaning, and the trigger
that should pull it back onto the board.

---

## 1. Import & tracking model — when may the catalog change on its own?

**Surfaced:** impl/05.2 (multi-source path handling) → widened to "what makes the
catalog grow or shift, and does that match user expectations."

**Core principle (2026-07-07 discussion).** *The catalog must only GROW when the
user asks.* A catalog that silently accumulates or reshuffles as the filesystem
churns undermines trust — the user should never feel it's "a weird, constantly
shifting collection." That forces us to separate three behaviors the current
"watched source" primitive muddles together — only **one** of which ever adds
unrequested rows:

1. **Import — one-shot, explicit. The default.** User points at files/a folder →
   the pipeline ingests once. Idempotent, so re-importing a child/parent/overlapping
   tree just skips the known files (`"0 new, N already in catalog"`) — no magic, no
   surprise. This is the LrC/traditional expectation. (Already what `RunJob` does.)
2. **Fidelity reconcile — keeps EXISTING assets honest.** moved→relink,
   deleted→missing, volume gone→offline. **Never mints a new identity.** Expected
   and trust-*building* (LrC shows the missing "?" badge). Runs on-launch/on-demand.
   (Already: impl/04 walk-end diff + impl/05.1 relink/merge.)
3. **Watched folder — opt-in, per-location. The ONLY auto-add.** "Auto-import new
   files in X." Off by default. This is what the impl/05 watcher powers — mirrors
   Immich's "external libraries" split (worth studying its schema).

**"Source" becomes storage/portability grouping + a `watched` flag**, not "a thing
we continuously absorb." Keep the `(source_id, relative_path)` normalization — its
`base_path` is the single place a mount location lives, so a remount rewrites one
row, not every asset. **Do NOT denormalize an absolute/base path onto each asset**
(the tempting "one-shot import, self-contained asset" idea): it buys nothing the
`watched` flag doesn't, and it throws away remount portability. Watched sources get
the live watcher (#3) + live fidelity; unwatched sources are import-once +
on-demand fidelity.

This **dissolves the earlier nested-source UX worry**: a descendant one-shot import
is just an idempotent skip — no "we secretly have it" message needed. Only *watched*
roots need the disjoint invariant strictly.

**Cross-source duplicates → user action, not auto-merge (resolved 2026-07-07).**
The same physical file under two roots mints two identities; that is acceptable
*provided we surface it*. Route it into the existing `duplicates` table
(`status='pending'` + `DuplicateRepo.ListPending` already exist — the *storage* is
built) for the user to resolve. Never silently unify across sources. The queue's
kind-classification and resolution actions — which now also cover probable
moves/renames — are specced in **§5**; a cross-source duplicate is one kind it
holds.

> **Dissolved by D20 (2026-07-07).** The scoping principle below was written when
> the matrix had *auto-mutating* verdicts (relink, delete-side merge) that had to be
> confined to `source_id`. **D20 removed those verdicts entirely** — reconciliation
> now detects-and-flags, it never auto-mutates identity. With no mutating verdict
> left, there is nothing that can re-home an asset across sources, so the latent bug
> below is gone. `FindByHash` stays global *by design* now — it is only a
> duplicate-detection flag, exactly the "global-but-flag-only" endpoint this
> principle was aiming at. Kept below for history.

This gave a scoping **principle**: the matrix's *auto-mutating* verdicts — relink
and delete-side merge — must be **same-source** (`WHERE source_id = ?`). Today
`FindByHash` and `FindMoveHealCandidate` are **global**, which is the latent
re-home bug noted below; scope them. Only *duplicate detection* stays global, and
its output is a **pending flag for the user, not a mutation**. This fixes the
correctness bug and enforces "only change on request."

**Sync level = one per-source enum, not orthogonal toggles (leaning, 2026-07-07).**
`sync_mode` on the source, three levels — this is edge routing, not a new path
through the pipeline stages:

- **Manual (default for one-shot imports):** no watcher, no timer. Fidelity of what
  was imported is caught only by an explicit "Synchronize Folder" (LrC's floor) or
  a launch reconcile. New files are never auto-added.
- **Scheduled:** periodic reconcile (add + fidelity, not instant). Nearly free — it
  IS the impl/05.3 poll timer.
- **Watched:** live watcher — auto-add + live fidelity (move/edit/delete) + live XMP.
  In `graduate`, an *unknown* new path is minted; a change to a *known* asset always
  applies.

Maps 1:1 onto the machinery: Watched → run a watcher; Scheduled → run a poll timer;
Manual → neither. **Manual sync is always available regardless of mode** — the mode
only decides what happens automatically on top.

*Why an enum, not two booleans (`auto_add` × `watch_existing`):* the market
(LrC, Immich external libraries, Capture One, digiKam) universally offers a single
per-library sync *level*, never split axes — because `auto_add ON + fidelity OFF`
is an incoherent state (new files appear, moved files stay broken forever) and
`fidelity ON + auto_add OFF` is a real but niche curator preference. Collapse to
levels now; add the "fidelity-without-add" advanced split only if a user asks (YAGNI).

**XMP metadata sync** rides the same axis — **impl/06 core DONE** (2026-07-08).
Live when watched, on-reconcile otherwise.

**Loose files vs. directories — `source` is doing two jobs (open, 2026-07-07).**
Importing a *set of individual files* (scattered, drag-and-drop) must not create a
source per file. The market never does: LrC/Capture-One reference each asset by
(volume, path) and *derive* the folder tree; Immich splits managed **uploads**
(a bag of assets, no source) from **external libraries** (folder-rooted). The
catalog is fundamentally a bag of assets referencing locations; the folder/source
is a *view* + a sync scope, not per-file.

Our `source(base_path)` conflates two roles that loose files pull apart:

- **Identity/portability anchor** = the **volume** (filesystem UUID — already what
  the D14 volume monitor keys on).
- **Sync scope** = a directory we walk/watch and whose "missing" we can infer.

For directory import they coincide; for loose files they don't (volume yes,
sync-scope no). Leaning: one "mint assets referencing (volume, path)" core, with
**two sync scopes**, which map onto mechanisms we already have —

- **Directory import → pipeline full walk** (add + fidelity + walk-end missing-diff).
- **Loose files → per-file `Reconcile`** (stat each *known* asset; no mass-missing,
  no auto-add). This is exactly the transitional `Reconcile` we planned to retire in
  05.3 — it may earn a **permanent home** as the referenced-file fidelity primitive.

**Hard constraint that forces the distinction:** the pipeline walk-end missing-diff
assumes it walked the *entire* source (unvisited known = missing). Point a source at
a volume and import 30 loose files, and the next reconcile marks the *rest of the
volume* missing. So loose files can NOT be modeled as "a source with a volume-wide
`base_path`" — they need the per-file scope.

**Open sub-question — ✅ RESOLVED (D24, 2026-07-10): the formal split.** `Source`
splits into **`Volume`** (identity/portability anchor: filesystem UUID +
connectivity) and **`Folder`** (tracked root: sync scope + sync_mode); one volume,
many folders. LrC's RootFolder/Folder/File schema independently validates the shape.
Executed by the source-management round (which also evaluates the asset/file
logical-physical split and the real-copies lineage edge — see D24). Until it lands,
"source" must not propagate into new surfaces.

**The bug the source-scoping fix closes — ~~still live in code today~~ DISSOLVED by D20:**

- ~~`AssetReader.FindByHash(hash, size)` and `AssetRepo.FindMoveHealCandidate` are
  **global** (no `source_id`)... a "move" heal can re-home an asset onto the wrong
  source.~~ **Gone:** D20 deleted `FindMoveHealCandidate`, `healMovedAway`, and the
  relink verdict — there is no auto-mutating verdict left to re-home anything.
  `FindByHash` remains global but only feeds the *duplicate flag* (a pending row),
  which is safe by definition.
- Two `notify` subscriptions on one subtree → every FS event fires twice (only an
  issue if two *watched* roots overlap).

**Urgency (assessed 2026-07-07): low — cleanly additive, defer.** This is a
migration + keying off a new field, not a gut-rewrite: the pipeline / matrix /
watcher / reconcile key off `source.ID` and `base_path` mechanically, and the
schema is **pre-1.0 with no real user catalogs**, so a migration is at its cheapest
and waiting does not make it harder (shipping real data would). Building 05.3 moves
*toward* this model — its per-file reconcile IS the loose-file primitive — not into
a corner. (The source-scoping fix that was called out here as "worth doing
independently" is now moot — **D20 dissolved it** by removing all auto-mutating
verdicts.) Everything else waits for source management.

**Why deferred:** source-management + settings (the `sync_mode` field) + browse UX
don't exist yet. The engine already supports all three behaviors — this is a
policy/gating layer (which entry point the UI calls + one field), **not a rewrite**;
the impl/05 watcher work stands.

**Guardrail until then:** the dev harness `ensureSource` (`cmd/dev/main.go`) matches
by **exact `base_path` only** and always watches — it models neither one-shot vs.
watched, nor overlap. Don't ship a real add flow without the explicit-add /
watched-opt-in split.

**Trigger:** building the add-source / import flow and the settings service.

---

## 2. Watcher orchestration / supervision

**Surfaced:** impl/05.2 — the watcher is a single per-source unit; nothing runs or
supervises many.

**Current state.** `Watcher.Run` is one blocking unit; `cmd/dev watch` runs exactly
one. The design's "one service; per-source units" (D14) is **unbuilt**, because
there is no long-running app host yet (Wails wiring deferred) — no process owns N
watchers.

**What to build when the host lands:**

- **Supervisor with restart + backoff.** Start a unit per enabled source; if a
  unit's `Run` returns a non-cancel error (watch-limit blowup, source error,
  recovered panic), log and restart after backoff. Restart is safe and cheap — a
  restarted unit does a startup reconcile and re-converges. This is just
  "reconcile answers every failure" applied to the *lifecycle*.
- **Per-source status snapshot** (mode `events`/`polling`/`offline`, last
  reconcile, dirty count) → status bar and the P3 health panel. *The seam side
  already exists (impl/16): the `watcher/sourceStatus` event type and the
  `seam.SourceStatus` payload are declared, shaped so this fuller snapshot extends
  it additively (no new event type). The supervisor is the missing **producer** —
  it calls `emitter.Emit(seam.EventSourceStatus, …)` on a connectivity/mode flip.*
- **Lifecycle wiring:** start/stop/quiesce units on source enable/disable/remove
  and on connectivity flips (from impl/05.3's volume monitor).

**Explicitly NOT building: an active health-check / kill-unhealthy watchdog.** The
impl/05.3 poll timer is a *periodic full reconcile*, which makes watcher liveness
**not load-bearing for correctness** — even a silently wedged unit cannot cause
divergence, because the periodic reconcile re-derives truth from disk regardless.
Health is therefore a **telemetry** concern (show the mode), not a correctness
watchdog. Detecting a true deadlock (vs. a crash) for telemetry is a later
heartbeat nicety, not v1. Building a probe-and-kill subsystem would be paying for a
guarantee the periodic reconcile already provides for free.

**Trigger:** the app-host (Wails wiring) milestone, once multiple sources run in one
process.

---

## 3. Windows single-instance lock is a placeholder

**Surfaced:** promoted from a `ponytail:` marker in `internal/sqlite/lock_windows.go`.

The instance lock that stops two app processes opening one catalog is real on
Unix but a **placeholder on Windows** — it holds the lock for the process but does
not enforce single-instance the way the Unix path does. Two Windows instances on
one catalog could race writes. No milestone owns this; easy to lose.

**Trigger:** Windows becomes a supported/tested target (or any Windows QA pass).

---

## 4. Orphaned derived files (thumbnails) — no GC

**Surfaced:** promoted from a `ponytail:` marker in `internal/sqlite/asset_repo.go`
(`DeleteByID`, the delete-side merge).

Thumbnails are named by asset ID and never stored as paths. Hard-deleting an asset
(delete-side merge absorbing a young duplicate) leaves its thumbnail file orphaned
on disk. Harmless individually (byte-identical to the survivor's own thumbnail),
but there is **no thumbnail/derived-file garbage collector** anywhere — orphans can
only accumulate (this delete path, plus any future hard-delete). Wants one sweep
that reconciles the thumbnail dir against live asset IDs.

**Trigger:** orphans measurably accumulate, or the first other hard-delete path
lands.

---

## 5. Review queue is not just duplicates — it also holds probable moves/renames

**Surfaced:** impl/05 close-out (2026-07-07) — a live-watch test of `mv img.jpeg
img2.jpeg` exposed that an unpaired rename (the OS emits an independent
remove+create; `notify` gives no portable from→to link, so pairing is deferred)
cannot be *safely* auto-relinked, and forced the question of how it should surface.

**The decision — extended to its conclusion by D20 (2026-07-07): NO auto-move at
all.** The `mv img.jpeg img2.jpeg` case first pushed us to flag *name-changed*
renames rather than auto-relink them. Following the same trust logic to its end, D20
removed **every** auto-move: a moved/renamed/copied file — same-name or not, judged
or not — is never auto-relinked or auto-merged. It becomes a `missing` original + a
distinct new asset + a `pending` review pair, and the user resolves it. Concretely:

| Trigger | Behavior (post-D20) |
|---|---|
| Same-path in-place edit (`a.jpg` overwritten) | **reimport** — refresh observation, keep identity + judgments. Automatic (path fidelity, not identity reshuffle). |
| Missing file reappears at its **original** path | **reimport → online** — restored automatically, same identity. |
| Content reappears at a **new** path (move / rename / copy) — same-name OR name-changed, judged OR not | **review** — original left `missing`, new path minted as a distinct asset, pair recorded `pending`. Never auto-linked. |

Why no auto-move even for the "obvious" same-name case: `partial_hash` is
xxhash(first 64KB + size), a change-detection *fingerprint*, not a full-content
hash — and more importantly, silently reshuffling identity is exactly the
"catalog changing underneath the user" that undermines trust in a hundreds-of-hours
catalog (D20). The market splits here: path-based DAMs (Lightroom Classic, Capture
One) mark external moves "missing" and make the user reconnect; only hash-based
digiKam auto-reconnects by content. We chose the conservative camp — automation of
identity is a *user-granted policy* later (D20's future direction), not an engine
default.

**Kind is DERIVED from live `file_status`, not stored — this is the load-bearing
design idea.** The existing `duplicates` table already records the pair in *both*
event orderings (verified: delete-first and create-first both end at "original
missing + new online + one `pending` row"). There is **no reliable detect-time
kind** (create-first looks like a plain duplicate until the original later
vanishes), so don't try to stamp one. Instead the two kinds fall out of the pair's
*current* status at read time:

| Original asset | Other asset | Kind | Resolution offered |
|---|---|---|---|
| online | online | **duplicate** | delete one / keep both (ignore) |
| **missing** | online | **probable move/rename** | confirm move (relink, keep judgments) / reject (keep separate) |

So `duplicates` is really a **pending content-match / resolution ledger**: its job
is to remember which pairs exist and what the user *decided* (`pending`/`resolved`/
`ignored`). **Keep the table and name as-is** (decided 2026-07-07) — no migration,
ordering-proof; the projection names the kinds.

**What is already built (impl/05 + D20 baseline — the entry point stands):**
- Detection + recording: **any** content match at a new path (move / rename / copy,
  same-name or not) flows to `actionDuplicate`, minting the new path and logging a
  `pending` duplicates row linking original→new. Holds in both event orderings and on
  both the walk and the single-path (watcher) entry. Locked by
  `TestUnpairedRename_RecordedForReview`, `TestCopyThenDeleteMove_RecordedForReview`,
  `TestWalk_FolderReorgRecordsMove`.
- Nothing is ever auto-merged or auto-relinked; no judgment is touched (D20).
- `DuplicateRepo.ListPending` returns the raw pending pairs.

**What is deferred (build with the source-management / review-UX milestone, which
owns the consuming UI — none exists yet):**

1. **Projection read** — `ListPendingReviewItems(ctx) []ReviewItem`, a pure read
   that joins each `pending` duplicates row to *both* assets' current rows and tags
   each `kind: duplicate | move` per the table above (plus the two asset summaries
   the UI shows). Could be a SQL view over `duplicates ⨝ assets ⨝ assets`. Edge
   rows to fold in: both-missing (stale — hide or auto-resolve `ignored`); the
   `duplicate_asset_id` itself now missing (both gone).
2. **Resolution actions** — these are structural / judgment-class writes, so they
   belong to the **user-action service** (NOT ingest's observation writer, which
   structurally cannot touch judgment). Each flips the ledger row to `resolved`/
   `ignored` and stamps `resolved_at`:
   - **move → confirm:** relink — adopt the new path onto the *missing original*
     and hard-delete the throwaway new identity, preserving the original's
     judgments. D20 removed the automatic `actionMove`/`healMovedAway` that used to
     do this, but left the **repo primitives** it was built from — `AssetRepo.UpdatePath`
     + `DeleteByID` (FK-cascade cleans the dup row). `ConfirmMove(originalID, newID)`
     re-composes them in the resolution service (delete new, UpdatePath original).
     Handle the case where the user judged the *new* row in the meantime (warn, or
     merge-judgments — pick with the UI).
   - **move → reject:** keep separate — original stays `missing`, new stays its own
     asset, row → `ignored` so it stops surfacing.
   - **duplicate → delete one / keep both:** `SoftDelete` the unwanted identity, or
     mark `ignored` to keep both. (Overlaps §1's cross-source duplicate flow —
     same table, same actions.)
3. **Source awareness in the projection (see §1).** With D20 there is no scoping
   *fix* to make (nothing auto-mutates), but the projection's *kind* rule should
   still be source-aware: a **same-source** missing→present pair is a *move*; a
   **cross-source** identical file is always a *duplicate*, never a move (a file
   can't move between two roots and keep one identity). So the derivation is:
   same-source + original missing → move; anything cross-source, or both present →
   duplicate.

**Trigger:** the review-UX / source-management milestone (the first UI that lists
pairs for the user) — build at least the projection then, so the kinds are computed
in one place.

---

## 6. Live mid-run worker-pool resize — machine.json applies next-run only

**Surfaced:** impl/11 (2026-07-07) — wiring the ingest worker counts to
`machine.json` (`Machine.Workers.Ingest`, read by `resolvePools` at
`newPipeline`).

Worker counts are now settings-owned and hot-reloadable *as config* — an edit to
`machine.json` is picked up live by the settings watch and the **next** import run
reads it. What is **not** built is impl/11 §5: changing the pool size *during* a
running import (drain the current generation of stage goroutines, relaunch at the
new count on the same channels). A user watching a large ingest can't dial workers
up/down mid-run; they change the config and it applies to the next run.

§5 specs the mechanism (a per-stage `stagePool` with cancel→`WaitGroup.Wait()`→
relaunch) but also flags an **unresolved run-teardown race** as a correctness
requirement: a hot-reload `OnChange`→`Resize` and the run's own end-of-run
channel-close are two independent triggers on the same pool, and must transition
under the *same* lock so a `Resize` either lands cleanly or observes "already
finished" — never a partial handoff. Building the `stagePool` now would be a resize
engine with no live caller (the `Machine.OnChange → run.Resize` hook lives in the
composition root, and the app host / `<app-config-dir>` don't exist yet) — YAGNI.

**Current leaning:** do it with the app-host milestone, when there's a real live
run to resize and a place to wire the `OnChange` hook. Extract the shared
`stagePool` primitive only if the export pipeline needs the identical shape too
(two consumers, not one).

**Trigger:** the app host lands (something to wire `OnChange`→`Resize` to), or a
user concretely asks to retune workers without restarting an in-flight import.

---

## 7. Seam methods awaiting their backing engines (impl/15 Phase 1 scope cut)

**Surfaced:** impl/15 (2026-07-09). impl/15's charter is *"services **wrapping** the
catalog interfaces."* When the contract surface was inventoried against real engine
capability, a cluster of contract.ts methods turned out to have **no engine to
wrap** — building those engines is each its own feature, not seam glue. Per the
"bind the verbs when the engine exists — don't fake them" rule (spec §3, applied to
undo/redo and extended here), they are **deferred, not stubbed**. The seam is
extensible by construction (a new bound method is one thin wrapper + one line in
`host.boundServices()`), so each lands cheaply the day its engine does.

**Phase 1 shipped (this change):** `AssetService` (QueryAssets, GetAsset,
AssetIDSlice, IndexOfAsset, DistinctValues, UpdateAssets by ids/query+exceptIds,
RemoveFromCatalog), `CollectionService` (full CRUD + membership), `SettingsService`
(settings get/set, keybindings get/set/reset), `SourceService` (+Create/Update),
the `ApiError` normalization layer + generated `errors.ts` code catalog.

**Deferred, each with the engine it waits on and its trigger:**

| Seam method(s) | Missing engine | Trigger to build |
|---|---|---|
| `getFolderTree` | no path→tree deriver (folders are derived from asset paths, never stored) | the browse/sidebar UI that renders the folder tree |
| `pickDirectory` | native OS dialog (Wails runtime, not the engine) | the Add-Source flow; wire via `runtime.OpenDirectoryDialog` in the app host |
| `openAsset` / `openWith` / `revealInFileManager` | no OS-shell executor (`Machine.OpenInApps` is config only, no launcher) | the open-in feature + its executor (uses `Machine.OpenInApps`) |
| `tagTree` / `createTag` / `updateTag` / `deleteTag` / `setAssetTags` (replace) | `TagRepository` exposes only keyword-import + `AddAssetTags` (additive); no tree/get/update/delete/replace, by design ("lands when the UI is the caller") | the tag-management UI milestone |
| `removeSource` | no `SourceRepo.Delete` (source removal + asset re-homing/cleanup policy unresolved) | source-management milestone (decide cascade vs. orphan policy first) |
| `deleteFromDisk` | `AssetRepo.DeleteByID` removes the row, not the file; no on-disk unlink path, and orphaned-thumbnail GC is itself deferred (§4) | a hard-delete feature (file unlink + thumbnail GC together) |
| `undo` / `redo` (+ history events) | history/command service is a later milestone (spec §3 already defers) | the undo/history service milestone |
| soft-delete **by query** (`RemoveFromCatalog` currently ids-only) | no `ApplySoftDeleteByQuery`; the judgment writer has by-query only for triage | when a "delete all matching" UX needs it (add the engine method mirroring `ApplyTriagePatchByQuery`) |
| keybinding **preset** list/apply | no preset engine; the default set is the frontend command registry's vocabulary | the keyboard-settings UI, if presets are still wanted then |
| `machine.json` exposure (worker pools, dependency paths) | settings engine exists, but no UI consumes it and machine scope is app-host-owned | the performance/settings UI milestone |

**Also deferred to the `wails dev` pass (not an engine gap — a toolchain one):** the
**`wailsjs/` method-binding regeneration** half of the contract reconciliation.
*(Narrowed 2026-07-11: the MODEL half — AssetRow + event payload interfaces — shipped
early via the D24 struct emitter; see the DONE entry below. What remains here:)*
regenerating the `wailsjs/` bindings for the new services runs `wails generate`,
which needs the webkit toolchain and runs the app — impl/14 already ruled that out
of the webkit-free backend gate (drift caught at the next `wails dev`/`build`). So
the method-binding side lands when the frontend rebuild runs under Wails (which is
also when the `TriagePatchInput` raw-JSON wire encoding gets its final shape).

**Event PAYLOAD TypeScript types — ✅ DONE EARLY (2026-07-10, the D24 schema-compiler
round).** The struct emitter landed ahead of its event-pump trigger: `cmd/generate`
reflects `catalog.AssetRow` + the envelope/payload structs (json tags = the contract)
into the generated `models.ts`, and `contract.ts`'s hand-written `AssetRow` is now a
composition over the generated model (adapter adds `kind` + `thumbURL` only). The
Go-side `AssetRow` gained `durationSecs`/`cameraModel` + camelCase json tags in the
same change. **Still deferred to the `wails dev` pass:** regenerating the `wailsjs/`
method bindings (webkit toolchain) and wiring the event pump itself (frontend/09
§Event pump owns that).

**Mock ⇄ SQL parity fixtures (golden vectors) — deferred (D24 round, 2026-07-10).**
The mock engine and the SQL compiler are two evaluators of one AST. The D24 round
aligned their semantics (NULL-negation policy, unrated-is-NULL, id-ASC tiebreaker,
ISO-8601 durations) and marked the known deliberate gaps with `ponytail:` markers in
`mock.ts` (`under` ≡ flat `has` until the tag browser; `matches` is substring, not
FTS). A shared golden-vector fixture (query + dataset → expected ids, run by both Go
and vitest) is only worth building **if the mock outlives the Wails bind** as a
permanent dev backend. Trigger: the Wails adapter lands and the mock is kept.

**Trigger (umbrella):** each row above is pulled in by its named milestone; none
blocks impl/16 or the frontend rebuild's read path.

---

## Not promoted (tracked elsewhere — left as inline `ponytail:` markers)

These are real deferrals too, but each is either owned by a named milestone or is a
self-contained tuning knob, so they live as inline comments (harvest anytime with
`/ponytail-debt`), not as ledger entries:

- Jobs = map+mutex, River later — **D17** (`importer/jobs.go`).
- `--debug` HTTP server richer surface — **§9 below** (`cmd/dev/main.go`).
- Volume-monitor precision — **impl/05.3 shipped the lazy poll-stat form**; the
  filesystem-UUID monitor (detects an unmount that leaves an empty mountpoint) and
  re-subscribing live events after a remount remain deferred (`watcher/watcher.go`).
- ~~Ignore-list editable in `settings.json`~~ — **DONE, impl/11 (2026-07-07):** the
  list and matching are owned by `internal/settings` (`Settings.MatchIgnore`/`Ignored`);
  importer SCAN and the watcher hold a `settings.Settings` value and call it. Seeded
  with defaults on first run, hand-editable, hot-reloaded.
- Thumbnail size tiers (one 512 for v1) — thumbnail feature (`thumbnailer/thumbnailer.go`).
- tx `BEGIN` deferred→`IMMEDIATE`, per-item re-commit on poisoned batch, 10-min
  move-heal window, transparent-thumb fill, notify overflow signal, benign
  double-graduation, settle-in-loop — self-contained tuning/heuristics with the
  trigger named at the site.

*(Audit note: as of 2026-07-07 none of the 14 `ponytail:` markers were stale —
nothing had been completed-but-left-commented, so none were removed.)*

---

## 8. `domain.PathKey` exists but is not yet wired into the identity paths

**Surfaced:** the D24 schema-compiler round's pre-commit review (2026-07-11).

D24 declared the policy — "compare keys, open bytes": `domain.PathKey` (Unicode NFC, no case
folding) for every path/filename equality/match/dedup; on-disk bytes stay the I/O truth. The
helper and its tests landed with D24, but **no production path calls it yet**: the importer/
reconciler identity matrix, the scanner skip-map (`ListKnownFiles`), `FindBySourcePath`, and the
new folder-scope compile all still compare raw bytes. The NFD phantom-identity risk D24 names
(macOS-ingested name later matched against an NFC form → spurious new asset + review pair) is
therefore still live.

**Design note for the wiring:** normalizing only the query side is WRONG against NFD-stored
rows — folder-scope `LIKE` and the path lookups likely need a stored normalized key column
(derived, rebuildable) rather than per-query normalization, so the comparison happens
key-to-key. Decide alongside the volume/folder schema change (one migration event).

**Trigger:** the source-management round (owns the volume/folder split — same tables, same
migration; D24 records the direction).

---

## 9. Dev-harness observability surface (`cmd/dev --debug`) — stdlib only for now

**Surfaced:** impl/08 (2026-07-07); the spec retired with the docs-restructure round, remainder
recorded here.

`cmd/dev --debug` mounts stdlib **pprof + expvar** at `localhost:6060` — the goroutine dump
already gives the live pipeline picture. Deferred: **statsviz** (live charts), a custom
**`/state`** JSON snapshot (pipeline stage depths, session counters), the `go:embed` HTML
dashboard, and `--json` output for scripting.

*(2026-07-13, gospan round: the calculus changed. `dev import` now writes a per-run gospan
trace file — per-stage percentiles, queue-time gaps, and batch analysis are one SQL query
against a closed file, which covers most of what the `/state` snapshot and statsviz wanted
post-hoc. What remains genuinely deferred here is the LIVE surface; task 22's enrichment
snapshot endpoint is its likely first real consumer.)*

**Trigger:** a real workload the stdlib surface can't diagnose — chasing a throughput regression
or a stuck-stage bug where watching queue depths over time would have answered it faster.

---

## 10. Dependency fleet beyond exiftool — download/verify/consent machinery

**Surfaced:** impl/07 (2026-07-07); the spec retired with the docs-restructure round, remainder
recorded here.

`internal/dependency` ships the exiftool slice only: `ToolID`/`Descriptor` table, `Discover`
(override path wins, else PATH; version floor; a missing tool is a `Status`, never an error —
D5). Deferred: additional tool rows (ffmpeg, dcraw, …), the app-data tools-dir tier, and the
user-consented **download + integrity-verification** flow (D4/D5: no silent downloads, ever —
fetch requires explicit consent + checksum verification).

**Trigger:** the first feature that needs a second external tool (video thumbnails/metadata →
ffmpeg is the likely first), or the packaging round deciding to bundle/offer managed installs.
