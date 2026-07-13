# Open questions & unheld design rounds

What a design-refinement round should pick up. Ordered by when they block. Question numbers are
stable IDs (cited elsewhere); resolved questions are deleted, so gaps in the numbering are
normal — the answer for a missing number lives in `docs/decisions.md` or git history.

## Design rounds that were never held (deliberately deferred)

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
    categories at scale. (Scenario originally flagged in `sysde.md` failure modes.)

16. **Catalog backup design round.** No backup code exists anywhere: no `VACUUM INTO` / backup-API
    path, no backup-before-migration (a P0 requirement), and the P1/P2 FR features (rolling
    backups, smart retention, multiple destinations, graceful skip) are undesigned. The
    *startup floor* (backup-before-migration + startup integrity check) is owned by the app-host
    milestone (impl/12); the *backup feature proper* — scheduling, retention policy, destinations,
    restore UX, health-dashboard integration — is its own design round. Becomes urgent the moment
    migrations stack on real user catalogs (= first release). Lane note from D28: the schedule
    itself is config (overdue-ness is derivable — convergent-shaped), but retry-with-backoff
    against a flaky destination (sleeping NAS, unplugged drive) is intent-shaped — that round
    should evaluate the D28 intent lane (River) for the destination-write half.

## Empirical tests needed (cheap, do during relevant milestone)

8. Does LrC preserve unknown XMP namespaces (`alexandria:Flag`) when it rewrites a sidecar? (impl/06)
9. River `riversqlite` maturity check at adoption time (D17) — it was "experimental, passes full
   test suite" as of mid-2026; re-verify before adopting.
10. FSEvents/inotify rename-event pairing reliability across the target platforms (impl/05 —
    determines how often the rename enrichment actually fires vs falls back).

17. **Does exiftool's XMP write leave a video's stream payloads bit-identical?** Take a `.mov`,
    `ffmpeg -i f -map 0 -c copy -f streamhash -hash sha256 -`, write an `XMP-xmpDM` marker with
    exiftool, streamhash again, compare. If equal, "annotate a master safely" stops being a gamble
    and becomes a verified invariant — and the whole container-write question in
    `../epics/backend-interop-targets.md` resolves. ffmpeg's own docs caveat that streamhash is for
    content-validation, not remux-validation, so this must be measured, not assumed. Fifteen
    minutes; blocks nothing until outbound Premiere is on the table.

## Known-open product questions (not architecture)

11. **Bundle export/merge-back format** (P2/P3) — self-contained mini-catalog; merge semantics on
    return. D1/D2 made it possible; nobody designed it. **Downgraded 2026-07-10:** the merge is
    less open than it reads — observations re-derive, derived rebuilds, sync-state resets, and only
    *judgments* merge, per-asset LWW on `judgment_modified_at` (the same coarseness #14 already
    accepts for XMP); collection membership is the D22 tag-tombstone rule verbatim. The one edge
    needing real thought is that returned file bytes must never overwrite the creator's masters —
    they land as new assets + a pending review, per D20. **The open question is whether to build it
    at all:** XMP sync already round-trips ratings, labels, keywords and notes, so a bundle's entire
    remaining value is the judgment XMP can't carry — collection membership, flags (the XMP-sync task §flag),
    groups, pins. Contractors on the receiving end mostly run NLEs and design tools, not DAMs. Revisit
    when a real user reports losing collection structure across a handoff; that is the trigger
    condition, and until then this is speculative. Distinct from
    `../epics/backend-interop-targets.md` — do not entangle them.
12. **machine.json exact schema** — trivial; write when the first consumer lands (worker pools at
    ingest tuning time).
13. **Telemetry event schema** (P3, opt-in) — per-extension skip counts and error reason codes are
    the anointed first events; design the consent + preview UI per FR.
14. **Per-field XMP 3-way merge** — upgrade from file-level via an `xmp_base` snapshot column
    (sync-state class) if coarse conflicts annoy real users. Named, deferred.

## Standing risks to watch

- **Wails v2 staleness** vs the ecosystem (v2 LOCKED by Ari 2026-07-07; engine stays runtime-agnostic per D1, so this is a packaging risk).
- **Windows** is third priority and untested by design so far — Job Objects path in `dependency`,
  ReadDirectoryChangesW in watcher, volume GUIDs. Budget a Windows pass per milestone, late is fine.
- **exiftool daemon protocol** quirks under concurrent load (single daemon vs small pool) — impl/07.
