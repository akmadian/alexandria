# Open Questions & Unheld Design Rounds

What a design-refinement instance should pick up. Ordered by when they block.

## Blocks nothing yet, decide during ingest implementation

1. **FTS `tags` column** — recommended: keep, app-maintained by `SetAssetTags` (FR P0 requires
   tag-name search). Alternative: drop column, rely on tag_ids filter. Micro-decision; recommendation
   is firm enough to implement unless refinement finds a problem.
2. **Sort fallback for NULL captured_at** — `COALESCE(captured_at, mtime)`? Query-layer decision;
   affects index usability (expression index if so). Decide in the query-layer round.
3. **Delete-side merge window** — "recently minted" = same session? N minutes? Recommend: same
   import session OR < 10 minutes, tunable constant. Decide in impl/05.

## Design rounds that were never held (deliberately deferred)

4. **Query layer round** — consolidate the filter→SQL builder (single query authority that smart
   collections P2 will reuse); COUNT strategy for grid scrollbar (`total`) — separate COUNT vs
   `COUNT(*) OVER()`; whitelist map for sort fields (audit found raw SQL interpolation);
   smart-collection query JSON format (nested AND/OR/NOT groups). Small round; do before the seam.
5. **The seam round** — reconcile `frontend/src/api/contract.ts` against the now-designed engine:
   backend `AssetFilter` parity (scope, fileStatus, absence queries: unrated/unflagged/…),
   `ListAssetsResult.total`, `getUIState/setUIState` verbs, job envelope wiring, thumbnail URL
   cache-busting (URL must include `thumbnail_at` or content token — thumbnails regenerate in place
   at P2 auto-refresh). The contract's conventions doc-comment is good; hold the round with both
   contract and engine in view.
6. **UI runtime selection** — Wails v2 (current, v2-maintenance-mode risk) vs Wails v3 (alpha
   status?) vs Tauri + Go sidecar (splits the process model — evaluate against D1's single-process
   decision) vs Electron + Go child. Blocks ONLY frontend milestone. Evaluate: seam mechanics
   (bindings vs HTTP/WS), binary-URL channel support, packaging/notarization, maturity. Note the
   engine is runtime-agnostic by construction (D1), so this decision cannot invalidate backend work.
7. **Grouping engine deep dive** — user explicitly parked it to focus singularly later. Settled
   already: derived/recomputable per (dir, stem) key, per-batch incremental recompute, anchor-declared
   directional companions (no cycles by construction), origin auto|manual, CoverRank min-wins with
   deterministic tiebreak. Open: CompanionPattern stem-matching modes (`IMG_1234.CR3.xmp` vs
   `IMG_1234.xmp`, `-Edit` suffix families), LrC-exported-vs-camera-JPEG cover heuristics, anchor
   priority table, group kind vocabulary.

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
- The FR itself has one stale line the session overrode: it files "current view" persistence under
  localStorage; D16 routes it to catalog KV (multi-catalog correctness). The FR should eventually
  be updated to match the decision log where they conflict.
