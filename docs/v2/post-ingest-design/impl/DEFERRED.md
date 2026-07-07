# Deferred & Open Questions — a running ledger

Things surfaced *during* implementation that we deliberately chose **not** to build
yet, captured so "later" doesn't quietly become "never." This complements the
design-level [`04-open-questions.md`](../04-open-questions.md) (which predates the
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

This gives a scoping **principle**: the matrix's *auto-mutating* verdicts — relink
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

**XMP metadata sync** rides the same axis but is **impl/06 — not built**. Live when
watched, on-reconcile otherwise, once it exists.

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

**Open sub-question:** does `source` formally split into `volume` + `tracked_root`
(cleanest, bigger schema change), or stay one table with a `scope: tree | file-set`
attribute (smaller, reuses walk vs. per-file reconcile)? Decide with source
management.

**The bug the source-scoping fix closes (still live in code today):**

- `AssetReader.FindByHash(hash, size)` and `AssetRepo.FindMoveHealCandidate` are
  **global** (no `source_id`). Used for *auto-mutating* verdicts (relink,
  delete-side merge), that means a "move" heal can re-home an asset onto the wrong
  source (`healMovedAway` writes the *walking* source's ID) whenever the same
  content exists under two roots. Scope these to `source_id` (see the principle
  above); keep the global lookup only for the *duplicate flag*.
- Two `notify` subscriptions on one subtree → every FS event fires twice (only an
  issue if two *watched* roots overlap).

**Urgency (assessed 2026-07-07): low — cleanly additive, defer.** This is a
migration + keying off a new field, not a gut-rewrite: the pipeline / matrix /
watcher / reconcile key off `source.ID` and `base_path` mechanically, and the
schema is **pre-1.0 with no real user catalogs**, so a migration is at its cheapest
and waiting does not make it harder (shipping real data would). Building 05.3 moves
*toward* this model — its per-file reconcile IS the loose-file primitive — not into
a corner. The **one item worth doing independently** of the model decision is the
source-scoping fix below: it is correct whether `source` ends up meaning volume or
tracked_root, and latent correctness bugs are best closed while fresh. Everything
else waits for source management.

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
  reconcile, dirty count) → status bar and the P3 health panel.
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

**The decision (conservative, "never auto-merge on a heuristic").** A filename
change is only a *probable* move, so we do **not** auto-relink it. We record it and
let the user resolve it. This keeps faith with the codebase's structural-trust
ethos: the one thing we auto-heal by content is the *unambiguous* case; anything
uncertain is surfaced, never silently merged. Concretely the policy line is:

| Trigger | Behavior |
|---|---|
| Same basename, unjudged, copy-then-delete (`mv a/x.jpg b/x.jpg`, or an app's temp-rewrite) | **auto-heal** — the delete-side merge (`healMovedAway`, zero-judgment guarded) absorbs it. No review. |
| Name changed (`mv a.jpg b.jpg`) | **review** — original left `missing`, new path minted as a distinct asset, pair recorded `pending`. |
| Any *judged* file moved/renamed | **review** — the zero-judgment guard forbids auto-heal, protecting judgment. |

Why the partial hash makes the name guard load-bearing: `partial_hash` is
xxhash(first 64KB + size), a change-detection *fingerprint*, not a full-content
hash. Waiving the name check (auto-relink any missing→present content match) was
considered and rejected — it would let a rare fingerprint collision silently
*merge two differently-named files into one identity* (data-loss-shaped from the
user's view), with no cheap way to re-verify because the missing file is gone. The
market splits exactly here: path-based DAMs (Lightroom Classic, Capture One) mark
external renames "missing" and make the user relink; only hash-based digiKam
auto-reconnects by content. We chose the conservative camp, with a review queue as
the resolution surface.

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

**What is already built (impl/05 baseline — the entry point stands):**
- Detection + recording: a name-changed content match to a missing asset flows to
  `actionDuplicate`, minting the new path and logging a `pending` duplicates row
  linking original→new. Holds in both event orderings and on both the walk and the
  single-path (watcher) entry. Locked by `TestUnpairedRename_RecordedForReview`.
- Nothing is auto-merged and no judgment is touched for a name change.
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
     judgments. This is exactly the existing `actionMove` + `healMovedAway`
     machinery (`AssetRepo.UpdatePath` + `DeleteByID`, FK-cascade cleans the dup
     row) — lift it behind a user-triggered `ConfirmMove(originalID, newID)` rather
     than re-implementing. Handle the case where the user judged the *new* row in
     the meantime (warn, or merge-judgments — pick with the UI).
   - **move → reject:** keep separate — original stays `missing`, new stays its own
     asset, row → `ignored` so it stops surfacing.
   - **duplicate → delete one / keep both:** `SoftDelete` the unwanted identity, or
     mark `ignored` to keep both. (Overlaps §1's cross-source duplicate flow —
     same table, same actions.)
3. **Source-scoping interaction (see §1).** Once `FindByHash` /
   `FindMoveHealCandidate` are scoped to `source_id`, a *move* is intrinsically
   **same-source** (missing here, present here); a **cross-source** identical file
   is always a *duplicate*, never a move. The projection's kind rule should read:
   same-source missing→present = move; anything cross-source = duplicate. Fold this
   in when the source-scoping fix (§1) lands so the two features ship coherent.

**Trigger:** the review-UX / source-management milestone (the first UI that lists
pairs for the user), or the source-scoping fix in §1 — whichever lands first should
pull in at least the projection so the kinds are computed in one place.

---

## Not promoted (tracked elsewhere — left as inline `ponytail:` markers)

These are real deferrals too, but each is either owned by a named milestone or is a
self-contained tuning knob, so they live as inline comments (harvest anytime with
`/ponytail-debt`), not as ledger entries:

- Jobs = map+mutex, River later — **D17** (`importer/jobs.go`).
- `--debug` HTTP server — **impl/08** (`cmd/dev/main.go`).
- Volume-monitor precision — **impl/05.3 shipped the lazy poll-stat form**; the
  filesystem-UUID monitor (detects an unmount that leaves an empty mountpoint) and
  re-subscribing live events after a remount remain deferred (`watcher/watcher.go`).
- Ignore-list editable in settings KV — **settings service / D16** (`importer/ignore.go`).
- Thumbnail size tiers (one 512 for v1) — thumbnail feature (`thumbnailer/thumbnailer.go`).
- tx `BEGIN` deferred→`IMMEDIATE`, per-item re-commit on poisoned batch, 10-min
  move-heal window, transparent-thumb fill, notify overflow signal, benign
  double-graduation, settle-in-loop — self-contained tuning/heuristics with the
  trigger named at the site.

*(Audit note: as of 2026-07-07 none of the 14 `ponytail:` markers were stale —
nothing had been completed-but-left-commented, so none were removed.)*
