# Grouping PRD — files into assets

**Status: RATIFIED** (adopted 2026-09-09; distilled from the old repo's grouping
round, invoked by Ari as archaeology and re-expressed in the current noun model.
Concepts crossed as prose; no schema, code, or old-model structure carried.
"For now" markers and the open questions at the end are exactly that.)

Grouping is how multiple files become one asset — the RAW is the capture, the
JPEG its rendition, one work. It is distinct from stacks (assets grouped over
assets) and from collections (authored sets). Judgments and keywords attach to
the asset ([data-models.md](technical/data-models.md)); this document covers how
assets form from files and which user decisions about them are permanent.

## Formation doctrine

- **Derivation, never comparison.** Each file derives one key (stem-based at
  its simplest — `IMG_1234.CR3` meets `img_1234.jpg` with zero configuration).
  Key collision *proposes* a membership; files are never compared pairwise.
  Cost stays one indexed lookup per file at any library size.
- **Collision proposes; evidence decides.** Named, legible grouping rules each
  answer supports / refutes / abstains (abstain = no information, never counts
  against). Content evidence is about the work (capture-time proximity,
  camera/serial agreement); filing-habit evidence is about organization
  (same directory, same import). The combination is a readable sentence, not a
  score: *any refute is an absolute veto; a candidate joins on same-directory
  or on at least one content support; filing-habit signals below same-directory
  never suffice at any count.* Weighted scoring and vote-counting were
  considered and rejected in the prior design: scores launder reasoning, and
  counting assumes independent evidence when the dangerous cases are exactly
  the correlated weak signals.
- **Formation is catalog-wide, never folder-fenced.** `trip/raw/` +
  `trip/jpeg/` must pair; prescribing same-folder organization to the user is
  unacceptable. The evidence gate, not a folder fence, is the safety.
- **The acid test** any future rule must pass: a decade-deep archive with a
  wrapped file counter — `photos/2015/IMG_1234.CR3` and
  `photos/2024/IMG_1234.jpg`, stripped metadata. Filing-habit signals co-fire;
  the pair must not form.
- **The precision invariant.** A false negative is inconvenient but correct —
  cured by a better rule or the manual verb. A false positive is a defect. A
  false positive that automation re-enforces over time is the worst defect the
  system can produce.
- **Formation rides import.** A file never appears in the app un-assetted and
  never visibly regroups after appearing; grouping is decided before first
  display.

## User decisions outrank automation, permanently

- Removing a file from an asset creates a standing exclusion no automatic
  process ever overrides; a manual re-add deletes it — a user decision outranks
  its own past.
- A rule change never resweeps the catalog on its own. New behavior applies
  where recompute naturally lands (new imports, touched files). Regrouping
  en masse exists only as an explicit, user-consented rebuild — and user
  choices survive it by construction.
- Every automatic membership records which named rule admitted it, so "why are
  these grouped?" always has an answer.
- **Transparency is the posture, not a feature.** The rules are the user-facing
  explanation of automatic grouping — named and documented; gaps are invited as
  bug reports, never hidden as tuning.

## Presentation grain ("for now" — Ari, 2026-09-09)

An asset wears its representative file's metadata in display and sort — per
field: the representative's value, else the first member that has one —
computed at read, never copied, so it cannot go stale. Inspector presentation
of member metadata may evolve; this is the accepted starting point.

Each asset has one representative file: a default rule picks it, the user can
override per asset, and the override survives everything automatic. The default
(ratified 2026-09-09 as a revisitable starting heuristic): the readily
displayable rendition — a JPEG when available, else the RAW.

## Requirements

Judging and acting:
- The user judges a shot once, regardless of how many files it occupies.
- File-lifecycle actions (delete, move, rename) default to the whole asset; the
  system never silently separates members on disk.
- Exactly which files an action will touch is legible before the user commits.
- The user can deliberately act on a single member, or dissolve an asset into
  per-file assets, without fighting the app.
- A file removed from an asset stays removed; user decisions and judgments
  survive everything the system does automatically.

Discoverability and trust:
- Grouping never makes a file undiscoverable: a search/filter match on any
  member surfaces the asset — exactly once.
- Displayed counts and displayed items never disagree.
- No file is ever present-on-disk but invisible-in-app.
- What an asset contains is legible at a glance.

Formation and life on disk:
- Inferable groups form automatically, with zero per-shoot labor.
- Membership is re-derivable from what is on disk and tolerates members
  appearing, disappearing, or arriving late.
- An asset's identity, and every user choice about it, survives recomputation.
- The user's place — scroll, cursor, selection — survives ordinary events.

Mechanism:
- Kind-agnostic: RAW+JPEG is one instance — extensible to video+proxy, audio
  masters+encodings, vector+exports without structural change (the registry
  idiom).

## Deliberately not carried from the old design

- **The parent-group layer** (groups of groups, judgments at every level, depth
  caps, the aim-at-a-node verb rule). Replaced by stacks, whose judgment
  semantics the noun round settled differently (judgments stay on member
  assets; collapsed-stack judging hits the cover by default). The old
  camera-declared burst/bracket ideas map to consent-based auto-stacking, when
  stack design happens.
- **Sidecar special-casing.** How files are differentiated within an asset
  (original vs sidecar etc.) is deliberately unsettled in the noun model; the
  old never-a-member rule is an input to that discussion, not a decision.
- **All schema and enforcement specifics** (membership columns, id minting,
  constraint machinery) — code-shaped; nothing crosses.

## Open questions

- Inspector presentation of member metadata beyond the wearing rule (above).
- Sidecar / file-differentiation interaction with membership (noun-model
  question, deliberately unsettled).
- Metadata write-back fan-out to members — tracked in
  [data-models.md](technical/data-models.md).
- What the grouping-rule vocabulary can express and where rules live in the
  model — the schema round's open item.
