# Open Questions & Unheld Design Rounds

What a design-refinement instance should pick up. Ordered by when they block.

## Resolved during implementation (impl/01–03, 2026-07-06)

- **FTS `tags` column** (was #1) — **RESOLVED: keep, standalone trigger-maintained table**, `tags`
  app-maintained, rebuildable via `sqlite.RebuildFTS`. Chose standalone FTS5 over external-content
  (the old-value bookkeeping for a non-content column was more code). See impl/01 status block.
- **Sort-field whitelist** (part of #4) — **DONE in impl/02**, then superseded by impl/13: the
  old `sortColumns` map + `List` method deleted; the AST compiler handles sort validation now.
- **Type-registry package naming** — **RESOLVED: `assettype`** (not `filetype`); type `Handler`
  (not `TypeHandler`). Repo convention: "Type" = format category, "Kind" = entity variant.
- **`InTx` isolation** — shipped with deferred BEGIN; **follow-up (non-blocking):** switch to
  `_txlock=immediate` (modernc DSN param) if write-lock contention ever appears. Single-writer
  design makes it moot for now.

## Blocks nothing yet, decide during ingest implementation

1. **`ContentFamily → domain.FileType` map** (NEW, impl/04) — the Sniff mismatch policy (impl/03
   built `Sniff` and deferred the wiring to impl/04) needs this ~15-entry map to reclassify a
   mislabeled file to the content's family. Build it in the pipeline with its consumer.
2. **Sort fallback for NULL captured_at** — `COALESCE(captured_at, mtime)`? Query-layer decision;
   affects index usability (expression index if so). Decide in the query-layer round.
3. **Delete-side merge window** — "recently minted" = same session? N minutes? Recommend: same
   import session OR < 10 minutes, tunable constant. Decide in impl/05.

## Design rounds that were never held (deliberately deferred)

4. **Query layer round** — **✅ RESOLVED (impl/13, built 2026-07-08).** `internal/ast` (grammar +
   vocabulary + validation + JSON + `CompileToSQL`); full query/command surface (`QueryAssets`,
   `AssetIDSlice`, `IndexOfAsset`, `DistinctValues`, `ReadTriageStates`, `ApplyTriagePatchByQuery`);
   collections CRUD; FTS⋈tags recomposition; COALESCE expression index for captured_at sort
   fallback (#2). `AssetFilter`/`List`/`buildFilterSQL`/`sortColumns` deleted (zero production
   callers, tests migrated). Separate COUNT strategy chosen (parallel query, not window function).
   Query/arrangement/page split done (seam ledger #1). All folded-in scope items shipped.
5. **The seam round** — reconcile `frontend/src/api/contract.ts` against the engine. The work
   list now lives as the **reconciliation ledger** in `../seam/01-queries-and-commands.md`
   (10 numbered deltas: AST filter, tag scope, arrangement, stale Settings/SourceStatus,
   file-based keybindings, job envelope, generated models, smart-collection CRUD, thumbnail URL
   cache-busting). The contract's conventions doc-comment is adopted as standing seam convention.
6. **UI runtime selection** — **RESOLVED (Ari, 2026-07-07): Wails v2 LOCKED.** The engine stays
   runtime-agnostic by construction (D1), so the residual v2-staleness risk (see standing risks)
   is a packaging concern, not an architecture one.
7. **Grouping engine deep dive** — user explicitly parked it to focus singularly later. Settled
   already: derived/recomputable per (dir, stem) key, per-batch incremental recompute, anchor-declared
   directional companions (no cycles by construction), origin auto|manual, CoverRank min-wins with
   deterministic tiebreak. Open: CompanionPattern stem-matching modes (`IMG_1234.CR3.xmp` vs
   `IMG_1234.xmp`, `-Edit` suffix families), LrC-exported-vs-camera-JPEG cover heuristics, anchor
   priority table, group kind vocabulary.

## Surfaced by the 2026-07-08 backend audit (design tasks, not yet scheduled)

15. **Mid-scan volume disconnect — the walk-completeness problem.** `stage_scan` tolerates
    per-entry errors and never aborts (correct for one unreadable file), so a drive/share that
    disconnects mid-walk yields a "completed" walk with a partial visited set — and the walk-end
    missing diff (`pipeline.go markMissing`) then flips every unvisited asset to `missing`.
    Self-heals on the next reconcile (same-path reappearance restores identity automatically),
    but a wall of "?" badges after a cable wiggle is exactly the "catalog shifting underneath
    me" event D20 exists to prevent. **This is a design task, not a quick guard** — the fix has
    UX and trust ramifications: When is a walk trustworthy enough to diff against? (Root-stat
    check? Directory-level error count? Unvisited-fraction threshold?) What does the user see
    when the diff is withheld ("volume disappeared mid-scan" — where, how loud)? Does the
    session record partial-walk status? How does it interact with source `connectivity` and the
    volume monitor? Do it before the frontend renders missing badges / Review missing-file
    categories at scale. (Scenario originally flagged in `_scratch/sysde.md` failure modes.)

16. **Catalog backup design round.** No backup code exists anywhere: no `VACUUM INTO` / backup-API
    path, no backup-before-migration (a P0 requirement), and the P1/P2 FR features (rolling
    backups, smart retention, multiple destinations, graceful skip) are undesigned. The
    *startup floor* (backup-before-migration + startup integrity check) is owned by the app-host
    milestone (impl/12); the *backup feature proper* — scheduling, retention policy, destinations,
    restore UX, health-dashboard integration — is its own design round. Becomes urgent the moment
    migrations stack on real user catalogs (= first release).

## Empirical tests needed (cheap, do during relevant milestone)

8. Does LrC preserve unknown XMP namespaces (`alexandria:Flag`) when it rewrites a sidecar? (impl/06)
9. River `riversqlite` maturity check at adoption time (D17) — it was "experimental, passes full
   test suite" as of mid-2026; re-verify before adopting.
10. FSEvents/inotify rename-event pairing reliability across the target platforms (impl/05 —
    determines how often the rename enrichment actually fires vs falls back).

## Known-open product questions (not architecture)

11. **Bundle export/merge-back format** (P2/P3) — self-contained mini-catalog; merge semantics on
    return (tag merge rules exist; collection/judgment merge needs design). D1/D2 made it possible;
    nobody designed it.
12. **machine.json exact schema** — trivial; write when the first consumer lands (worker pools at
    ingest tuning time).
13. **Telemetry event schema** (P3, opt-in) — per-extension skip counts and error reason codes are
    the anointed first events; design the consent + preview UI per FR.
14. **Per-field XMP 3-way merge** — upgrade from file-level via an `xmp_base` snapshot column
    (sync-state class) if coarse conflicts annoy real users. Named, deferred.

## Standing risks to watch

- **Wails v2 staleness** vs the ecosystem (see #6).
- **Windows** is third priority and untested by design so far — Job Objects path in `dependency`,
  ReadDirectoryChangesW in watcher, volume GUIDs. Budget a Windows pass per milestone, late is fine.
- **exiftool daemon protocol** quirks under concurrent load (single daemon vs small pool) — impl/07.
- ~~The FR files "current view" persistence under localStorage; D16 routed it to catalog KV.~~
  **Resolved by the 2026-07-07 frontend round + impl/11:** `frontend/02-state-model.md` locks
  layout/theme/density/current view mode → localStorage (lose-and-shrug chrome), saved queries →
  catalog, keybindings → `keybindings.json`. D16's scope-routing principle stands; its storage
  mechanism was superseded. The FR matches the newer decision.
