# Grouping (file→asset formation): research & round record

**Status: RESEARCH + RATIFIED fragments** (design round in progress, 2026-09-11).
This doc records fetched prior art and the rulings Ari has made in the round so
far. The system's architecture is NOT settled here — proposals live in chat
until ratified. Scope note: this round designs how files group into asset
records. Stacking (grouping of assets) is explicitly a separate, future round.

## Ratified (Ari, 2026-09-11)

- The system's name is **asset formation** (code and docs). "Grouping" in
  requirements prose reads as the user-facing umbrella; stacking (assets →
  stack) remains a separate future round's vocabulary.
- The minting CHECK is relaxed: files commit without assets, and NULL
  `asset_id` = formation pending, for every kind — the thumbnail_at idiom.
  Minting an asset for every file at commit time "reaches outside its step."
- Pipeline shape: walk → prepare → record batch (files only) → asset
  formation pass, a distinct visible step — never chained inside a
  catalog write method.
- RAW+JPEG rule scope: same import + same stem, WITH refuting-evidence
  guards inside the rule (capture time disagreement, camera identity
  disagreement → refuse; absent evidence abstains). The guards are the
  system's answer to sequential-name collisions.
- Cross-import heuristics (late-arriving pair members, orphan sidecar
  adoption) are deliberately out of the current design's scope: the system
  is designed over a freshly committed set of files from the running
  import, filtered by import id.
- The formation pass is invocable and context-agnostic — handed a scope
  (e.g. an import id), not welded to the import flow.
- Engine shape (ratified 2026-09-11, superseding the file→partners claim
  model): relationship-centric, the explicit ER pipeline — generate
  candidate relationships from the blocking key; evaluate each candidate
  against the ordered rules, where a rule is a PURE PAIRWISE FUNCTION
  (refutations return false early, corroborations return true, the final
  line abstains into false); select one subject per sidecar (the tie-break
  as a named stage); union confirmed edges into clusters; floor; persist.
  A rule is one value: an id and that function. Rejected along the way as
  unearned: a rule protocol with existentials, and a typed evidence-guard
  layer (supports/refutes/abstains as an enum) — the tri-state semantics
  lives in check ordering, pinned by the acid-case test.
- P0 scope: grouping runs at import time, inside the import flow; anything
  beyond that is the user's responsibility for now.
- Cadence: per-batch cumulative — after each recorded batch, one formation
  pass sweeps "this import's files where asset_id IS NULL" (the database is
  the worklist; passes are idempotent). Candidate universe = all of this
  import's files, formed included, so a pair completing across batches joins
  the existing asset. Formation passes serialize through the single writer;
  parallelism belongs to prepare (a future pipeline round).
- Batch invariance (ratified 2026-09-11): batch boundaries have NO effect
  on formation — batching is efficiency and UX only, and the outcome must
  equal one all-at-once pass over the same files; divergence is a bug.
  Consequence: when a later pass's edge proves two same-import scaffolding
  assets are one work, they MERGE — the elder (lowest-id) asset survives,
  the others are absorbed: files repointed, provenance untouched, emptied
  asset rows deleted. This supersedes the earlier bridge-refusal semantics
  within an import. Cross-import merging remains nonexistent (anchors can
  only be same-import assets under current scope).
- The floor rule: every non-sidecar file no rule claims gets its own
  singleton asset (`one_asset_per_file`, the v0 id), applied eagerly each
  pass. Ratified 2026-09-11: the floor is a REGISTRY ROW like any other
  rule — it confirms the reflexive candidate (a file's relationship with
  itself) that generation emits per unformed file, and sits last in the
  registry so founding never outranks a real admission (provenance tests
  pin the ordering). Only orphan sidecars remain unformed (asset_id NULL).

- Composition (ratified 2026-09-11): three layers, none trespassing. The
  PIPELINE orchestrates — ImportRun.formAssets() is a named @concurrent
  step, peer to walk and prepare: read, decide, persist. The ENGINE decides
  — AssetFormation.form(files:) is pure (no database access; it logs its
  own events, since observability is not a side effect). The CATALOG stores,
  per table — files(inImport:) in Catalog+Files, recordFormedAssets(_:) in
  Catalog+Assets (one transaction; Cluster is the interchange type). A
  catalog file is named for a TABLE, never a process; a process never owns
  a writer. Formation re-reads committed rows each pass by design — the
  fetch IS the idempotency mechanism (input is always truth, never a
  shadow copy); prepared-batch state lacks catalog identity and cross-batch
  scope.

## Flagged during design (2026-09-11)

- Merge judgment guard: absorbing a scaffolding asset is safe today only
  because no judgment UI exists — nothing can carry user state mid-import.
  When the judgments round lands, an absorbed asset carrying user state
  (rating, flag, representative override) must refuse the merge and
  surface it; user decisions outrank automation.

- Universe fetch narrowing: each pass currently fetches the whole import's
  rows (cumulative cost grows with import size). The named optimization,
  when profiling cares: fetch unformed files plus files sharing their
  stems — bounded by batch size, database stays the single truth. Not
  built; P0 cost is fine.

- Cross-folder pairing must earn itself (RATIFIED with the build go-ahead):
  same-folder candidates pair on stem (guards may refute); different-folder
  candidates require positive corroboration (agreeing capture time or camera
  identity), because abstain-on-absence protects nothing against stripped
  metadata (the archive/stripped-JPEG acid case).
- Sidecar subject tie-break when stem matches multiple assets — built per
  the flagged order: exact stem.ext form, else raw subject, else lowest
  file id.
- Re-import orphans: a sidecar or rendition imported later than its partner
  (skip-existing puts the partner in a prior import) lands unattached /
  unpaired — deliberate consequence of same-import scope; cured by the
  future rescan / cross-import round or the manual add verb.
- Twin-camera residual risk (same model, no serials, same stem, agreeing
  times, different folders): accepted residual false-positive shape; manual
  remove verb is the cure.
- formation_rule provenance is historical — the rule that admitted THAT
  file, never "the rule that would match now" (a floor-formed raw keeps
  one_asset_per_file after its jpeg joins by raw_rendition_pair).
- Failure policy: a formation pass failure rolls back and retries on the
  next pass; a final-pass failure fails the import (formation is
  import-critical).
- Users can manually remove a file from an asset and manually add a file to an
  asset — arbitrarily. (Verbs; UI unscheduled.)
- File-level visibility is never sacrificed: the inspector shows an asset's
  constituent files; the loupe can switch between members (RAW, JPEG, anything
  else in the group). Alexandria manages files at the most atomic grain on
  disk and never hides them Lightroom-style.
- Never-events, product law: unrelated files grouped because sequential
  filenames collide (the PhotoPrism failure below); users forced to rename or
  otherwise mutilate their own archives to escape the automation.
- The rules live in one central registry — a list of named rules, each with
  its matching heuristic maintained in one place; no scattered branching
  logic. RAW+JPEG is the first rule.
- The user controls listed below are all wanted eventually ("we should support
  all of them"), not necessarily in P0.
- Noted, deliberately not designed now: a grid mode letting the user browse
  files instead of assets (query swap over the same substrate).

## Tracked: controls users ask for (wanted, unscheduled)

From the UX survey below; requirements.md already carries the global
auto-grouping toggle and dissolvable groups.

- Turn automation off (global toggle — already a requirement)
- Representative/primary member: default rule + per-asset override (user
  preference genuinely splits RAW vs JPEG)
- Visible badge that a grid item is a group, with constituent access
- Un-group / dissolve that works
- A predictable retroactivity story (LrC's non-retroactive import preference
  is a recurring confusion generator)

## Research: what kind of system this is (fetched 2026-09-11)

"Decide whether records refer to the same underlying entity via evidence
rules" is an established field: **entity resolution / record linkage**
(academic, from Fellegi & Sunter 1969), **match rules** (Master Data
Management industry). Canonical pipeline: blocking/candidate generation via a
derived key → per-field comparison (an evidence vector) → classification →
canonicalization. A filename stem is a textbook blocking key; the field's
scaling insight is one indexed key lookup per record, never pairwise scans.

- Pipeline & blocking: [(Almost) All of Entity Resolution](https://www.science.org/doi/10.1126/sciadv.abi8021),
  [end-to-end ER survey](https://blog.acolyer.org/2020/12/14/entity-resolution/),
  [blocking survey, ACM CSUR](https://dl.acm.org/doi/10.1145/3377455)
- The Fellegi-Sunter model formalizes supporting/refuting evidence: per-field
  agreement weight (positive) and disagreement weight (negative), summed
  against two thresholds → match / non-match / "possible match" for human
  review. [Two-step FS procedure](https://pmc.ncbi.nlm.nih.gov/articles/PMC9336505/),
  [Census application](https://www.census.gov/content/dam/Census/library/working-papers/1991/adrm/rr91-9.pdf)
- Deterministic vs probabilistic: deterministic rule lists are cheap,
  high-precision/low-recall, and self-explaining; probabilistic wins on messy
  human-entered data. Practitioner guidance says deterministic for
  high-quality structured data and auditable decisions.
  [Splink topic guide](https://moj-analytical-services.github.io/splink/topic_guides/theory/probabilistic_vs_deterministic.html),
  [Data Ladder](https://dataladder.com/deterministic-vs-probabilistic-matching/),
  [simulation study](https://www.sciencedirect.com/science/article/pii/S1532046415000921)
- Documented failure modes of weight-summing: correlated evidence gets
  double-counted (conditional-independence violation); thresholds are
  subjective and brittle; honest scores need labeled corpora and tuning
  apparatus (SpamAssassin maintains hand-classified corpora + mass-check + a
  genetic algorithm per release).
  [Fellegi-Sunter limitations](https://www.zingg.ai/post/fellegi-sunter-model-limitations-modern-entity-resolution),
  [SpamAssassin rescoring](https://cwiki.apache.org/confluence/display/spamassassin/RescoreMassCheck310)
- Rule registries in industry: MDM match rule sets are named, ordered,
  individually enable/disable-able rules, exact-or-fuzzy per column, each
  declaring auto vs manual consolidation; filtered rules restrict
  applicability. [Informatica match rule configuration](https://docs.informatica.com/master-data-management/multidomain-mdm/10-3-hotfix-1/sample-ors-guide/match-and-merge-configuration/match-rule-configuration.html)
- Fowler on rules engines: generic engines fail through implicit program flow
  and rule chaining; his recommendation is a limited, custom rules engine for
  the narrow context. [RulesEngine bliki](https://martinfowler.com/bliki/RulesEngine.html)
- Veto precedent: soft negative evidence (FS disagreement weights), hard
  exclusion (MDM filtered rules), and blocking itself (non-candidates are
  never compared).

## Research: UX field survey (fetched 2026-09-11)

**Demand is universal and old.** Standing Adobe feature requests for
[RAW+JPEG stacking](https://community.adobe.com/feature-requests-681/p-please-stack-raw-jpeg-files-661158)
and [auto-stack by capture time](https://community.adobe.com/feature-requests-681/p-auto-stacking-by-capture-time-661611);
[Immich design discussions since 2023](https://github.com/immich-app/immich/discussions/2479)
("I would have the same photo example 3 times in the timeline"); demand strong
enough that a third-party CLI ([immich-stack](https://majorfi.github.io/immich-stack/how-to/real-world-examples/))
exists to fill the gap. Users' stated motivation is grid dedup + not
hand-pairing hundreds of files.

**Failure mode 1 — under-grouping / opacity (Lightroom Classic).** The JPEG
becomes a hidden pseudo-sidecar of the RAW: one catalog entry, no access to
the JPEG, a global import-time-only preference, non-retroactive. Generates
endless "where did my JPEGs go" threads and re-import workarounds.
[Life after Photoshop](https://lifeafterphotoshop.com/lightroom-raw-plus-jpeg-pairs/),
[Lightroom Queen thread](https://www.lightroomqueen.com/community/threads/importing-raw-jpeg-but-only-raws-visible-in-lr.25488/).
The complaint is the invisibility and the one-way door, not the pairing.

**Failure mode 2 — over-grouping / no escape (PhotoPrism).** Stacks by
filename catalog-wide and by EXIF OriginalFileName:
[unrelated photos from different cameras stacked because sequential names collide](https://github.com/photoprism/photoprism/discussions/4605),
[same-name files stacked across different folders](https://github.com/photoprism/photoprism/issues/4309),
and [name-identical stacking cannot be disabled; re-indexing won't unstack](https://docs.photoprism.app/known-issues/).
Users renamed their files to escape. Empirical confirmation that a
self-re-enforcing false positive is the trust-killing defect class.

**The visibility spectrum shipped today.** No mainstream DAM ships a visible,
technical rule system:

- Lightroom Classic: one hidden rule (same folder + same basename), one global
  toggle, opaque result.
- Capture One: display-only name-based pairing
  ([Pair RAWs and JPGs](https://support.captureone.com/hc/en-us/articles/30110560619165-Pairing-RAW-and-JPG-files-in-Capture-One)),
  syncs no metadata, breaks under unsorted batch rename; blunt global
  "Always hide JPEG" filter.
- Mylio: the most user-respecting commercial treatment —
  [pair as one item, global "Prefer RAW" setting, per-image display override](https://manual.mylio.com/24.3/en/topic/work-with-raw-jpeg-pairs).
- digiKam: [manual verbs only](https://docs.digikam.org/en/main_window/image_view.html)
  (group by filename / time ±2s / burst ≤1s).
- Immich (maintainer direction): individual assets + parent references +
  configurable auto-linking at import — independently converging on
  files-under-assets with rules.
- immich-stack: existence proof of power-user demand for tunable rules
  (delimiters, regex, time windows, promotion lists) — config-file-grade,
  self-hosters only.

**Apple Live Photos** pair by a
[shared ContentIdentifier UUID written into both files' metadata](https://github.com/LimitPoint/LivePhoto),
not by stem — the existence proof that camera-declared content evidence can
outrank name evidence when present.
