# Decision Log

Numbered, ADR-lite. Each entry: decision, rationale, revisit trigger where meaningful.
NFR references → `requirements-distilled.md`.

> **Implementation status (2026-07-07):** D3 (SQLite/FTS), D8 (data classification → writer-scoped
> repos), D9 (UUIDv7 + matrix), D11 (column promotion), D16 (schema side: sources split,
> keybindings table dropped) are realized through impl/01–02. D6 (unified registry) + D7
> (magic-byte classifier) are realized in impl/03 — note the package shipped as **`assettype`**,
> not `filetype` (Type = format category vs Kind = entity variant). **D12 (pipeline shape),
> D13 (DLQ = import_errors, no retry machinery), D17 (jobs: registry map + OnProgress), and
> D18 (ignore list) are realized in impl/04.** **D14 (watcher) was realized in impl/05:** 05.1
> (matrix extensions, `internal/importer`) kept; 05.2 + 05.3 (the `internal/watcher`
> service) rebuilt after the first cut drifted from the Prime Directives (the watcher was
> deciding actions and writing `file_status`). Corrected boundary (see the "Corrected architecture"
> note atop `impl/05`): watcher = sensor that hands the importer a *path, never a verdict*; the
> importer's single-path entry decides + writes (present→ingest, gone→missing — D20 removed the merge); the watcher's
> only own write is `sources.connectivity`; batch reconcile is the importer's full walk, which the
> watcher schedules. The per-OS event adapters collapsed to one dep (`rjeczalik/notify`), the volume
> monitor to a poll timer (per-OS UUID monitor deferred). Building 05.2 surfaced the broader
> **import/tracking model** (one-shot vs. watched sources, `sync_mode`, loose-files vs. directories,
> cross-source dup handling) — captured in `impl/DEFERRED.md`. Everything else is design-only so far.

## D1 — Single-process desktop app; engine as embedded Go library behind one transport-agnostic async contract

The UI talks to the engine through exactly three channels: request/response (async, DTOs and IDs
only), event push (subscriptions), and binaries by URL (never over IPC). The existing
`frontend/src/api/contract.ts` already has this shape — it is effectively network-shaped without a
network. **Seam tax** (the only cost paid now for future server mode): no synchronous engine calls
from UI; no per-field bindings; no UI feature dereferences a filesystem path directly (always
through an engine verb). Server mode (Plex model) is a v3+ *packaging* option; its genuinely hard
parts (path virtualization, multi-writer conflicts, auth) are deferred until real demand.
The contractor-handoff use case is served instead by **bundle export/merge-back** (P2/P3): a
self-contained mini-catalog that travels by disk and merges back — the Capture One sessions model.

## D2 — Multi-catalog is first-class, delivered as self-containment

Eagle-style: library switcher, recent catalogs, one open at a time. Everything catalog-scoped
lives inside the catalog directory (DB + thumbnails + settings). Per-catalog instance lock.
App-level state (localStorage) must never hold catalog-entity references. Bundle export (D1)
depends on the same self-containment property — one property, three features.

## D3 — Storage: SQLite, polyglot-by-durability-class

SQLite WAL, `synchronous=FULL`, backup via backup API / `VACUUM INTO` only. FTS5 for search
(external-content table + triggers). Rule: **exactly one transactional store for judgment; derived
stores may multiply, each with a registered rebuild path.** Thumbnails live on the filesystem
(sharded dirs), not in the DB (backup bloat, WAL churn, served as files anyway). Rejected: DuckDB
(OLAP shape, we do transactional point-ops), embedded KV (rebuilding SQLite's query layer by hand),
Postgres (kills zero-ops), flat files (dies at 5k). Every future "SQLite can't search X" itch is
FTS5 configuration before it is a new engine.

## D4 — Format decoding via supervised subprocesses, never cgo

Rationale in priority order: crash isolation (decoders parsing untrusted input WILL segfault —
subprocess death is a per-file error, not an app crash), memory/runaway isolation (kill -9 always
works; GOMEMLIMIT can't see C allocations), build sanity (pure Go cross-compiles; cgo drags
toolchains and GPL linking questions), inherited format knowledge (exiftool IS the accumulated
wisdom). Hybrid: pure-Go handles safe common formats in-process (JPEG/PNG/GIF today); everything
exotic goes through the fleet. exiftool runs in `-stay_open` daemon mode (isolation with ~ms
latency). Goroutines orchestrate; subprocesses decode.

## D5 — The `dependency` package (external-tool supervisor)

See `impl/07`. Descriptors (identity, version constraints, per-platform acquisition, invocation
conventions) + discovery (PATH → app-data → user override) + execution policy (timeout =
f(tool, operation, file size)) + daemon lifecycle + per-tool semaphores (NFR-5's physical knob).
Orphan management is layered: stdin-EOF convention for daemons, self-timeouts (`ffmpeg -timelimit`)
for one-shots, pdeathsig on Linux, Job Objects on Windows. No startup reaping (rejected as
unnecessary given the layers). **Not pluggable beyond the descriptor table** — extension = adding a
data row, never adding abstraction. Distribution: detect on PATH → *user-consented* download with
pinned checksum → strip macOS quarantine xattr. Never silent download (NFR-6). App must be useful
with zero tools installed (pure-Go path).

## D6 — One TypeHandler registry, keyed on normalized extension

Single explicit table in one file = the documentation of what's supported. Folds the three
currently-parallel MIME maps (filetype/metadata/thumbnailer). MIME demotes to an attribute (needed
at the seam), not a dispatch key. Capability fields (`Metadata`, `Thumb`, later `Preview`,
`Grouping`) are small independent interfaces; nil = graceful degradation, never an error.
**Interfaces are carved at the second implementation** — `PreviewExtractor` and `GroupingRule` are
designed-for but not defined until their features ship. Explicit central table, NOT
`init()` self-registration (that idiom is for open ecosystems; ours is a closed set).
Generics rule: interfaces for varying behavior, generics for varying data (`Opt[T]` good; a
generic registry wrapper only if 4+ registries emerge).

## D7 — Hand-rolled magic-byte classifier, piggybacked on the hash read

~50-line table of (offset, magic) → canonical type id, checked against the 64KB the hash stage
already read (zero extra I/O). **Extension is primary, content validates**: mismatch → trust
content for the category, badge the asset, log. TIFF-family RAWs share magic — content confirms
container, extension picks dialect. No-extension files: sniff-only. No third-party dependency.

## D8 — Data classification: observations / judgments / derived / sync-state

The load-bearing idea of the whole design. See `data-model.md`. Writer-scoped repository
interfaces make cross-class writes *uncompilable*. Immediate payoffs already banked: the
reimport-clobber bug class eliminated; `sources.status` split into `enabled` (judgment) +
`connectivity` (observation); `judgment_modified_at` added (XMP conflict resolution depends on it);
duplicates rebuild must upsert around judgment-bearing rows; `file_status` is observation-only
(relocate flow *triggers re-observation*, never writes status directly).

## D9 — Identity: catalog-assigned UUIDv7 + reconciliation matrix

Identity is a policy, not a fact — every file property is mutable, so the PK is minted at ingest
and thereafter *matched*. UUIDv7 (time-ordered → index locality; switch from v4). UUIDs are
load-bearing for bundle merge-back and multi-catalog (autoincrement guarantees collisions).
The matrix and its precedence: see `data-model.md` §Identity. Key refinement over pure
path-primacy: **an exact content+name match against a MISSING asset outranks path-based reimport**
(kills the delete-and-copy judgment-cross-attachment bug). Accepted failure modes are documented,
named, and all leave visible, healable residue (duplicates review, missing view, relocate flow).

> **Superseded in part by D20 (2026-07-07):** the *auto-relink* action here is removed — an exact
> content+name match against a missing asset is now *detected and flagged for review*, not
> automatically relinked. The matched-identity model stands; its licence to auto-mutate identity does not.

## D10 — Signature ladder with an authority rule

mtime+size (staleness) → partial hash 64KB+size (exact-ish identity) → full hash (verification) →
phash/P3 (perceptual). **Exact tiers may drive automatic actions; perceptual tiers may only
suggest.** Partial hash may *propose* duplicates; the P2 review UI must *confirm* with full hash
before claiming "identical". phash never feeds the identity matrix — similar ≠ same, and burst
neighbors are exactly where judgments must not cross-contaminate.

> **Superseded in part by D20 (2026-07-07):** "exact tiers may drive automatic actions" now applies
> only to path-fidelity (reimport / mark-missing / add). Exact tiers *propose* identity changes
> (relink/merge) for user review; they no longer act on identity automatically.

## D11 — Column promotion rule (first-class vs JSON blob)

A metadata field earns a real column iff **(a)** an FR filter/sort/group consumes it, **(b)** FTS
must index it, or **(c)** the engine itself consumes it — plus it must be cross-format
normalizable. Everything else → `extended_metadata` JSON, keyed by **exiftool `Group:Tag`
vocabulary** (`"EXIF:Flash"`, `"IPTC:City"`) — standard, documented, ends naming debates.
Promoted this session: `title`, `caption` (FTS targets; note `caption`=observation vs
`note`=judgment — superficially redundant, semantically distinct). Denied with reasons:
flash/metering/WB/altitude/orientation (display-only), audio artist/album (no FR filter yet),
IPTC location (promote at P3 with geocoding). Promotion path is cheap by design: blob → column is
ALTER + backfill from blob — **never requires re-reading files**.

## D12 — Ingest pipeline shape

`SCAN → HASH → MATCH → EXTRACT → THUMB → WRITE`; bounded channels; blocking sends = backpressure;
all wiring in ONE function; MATCH and WRITE are singletons (matrix read-serializability; SQLite
single-writer turned into the batching point). **THUMB precedes WRITE**: an asset commits only
when fully processed — no placeholder cards ever (user decision, emphatic, anti-LrC-half-imported
trauma). Batches of 50/txn; post-commit hooks in order: FTS (via triggers, in-txn), grouping dirty
keys flush, JobProgress + catalog:changed events, session log. Cancellation = commit current batch,
stop; resume is "import again" (idempotency is the recovery mechanism). Streaming walk with
indeterminate→determinate progress (no counting pre-pass). In-run hash map inside MATCH (intra-run
duplicate detection). Sidecars route SCAN→HASH→WRITE. Walk-end diff (known-map minus visited) =
missing detection. Bouncer checks at SCAN's front door (not a separate stage). Implementation reference: `internal/importer/README.md`.

> **THUMB-precedes-WRITE clause superseded by D25 (2026-07-11).** Thumbnails leave the ingest
> pipeline and become the first citizen of the post-ingest enrichment system. Everything else in
> this entry stands.

## D13 — Failure handling: DLQ in DB, no retry machinery

`import_errors` = the DLQ: durable rows (path, stage, reason code, raw error, timestamp, attempt
count). Passive by design — re-drive mechanisms are the next file event, any reconcile, or manual
"retry failed" (re-feeds paths as hints). Retry timers rejected: they'd be a second, dumber
convergence loop. Corrupt/mid-write files: best-effort ingest + error marker + **self-heal via next
change event**; a file with no usable content (fails magic sanity) gets an error row but no minted
identity. Unknown files: never tracked as rows, counted per-extension in the session summary
(visibility + ignore-list hints + future opt-in telemetry).

## D14 — Watcher service: sensors, not actors

One service; per-source units, each a state machine (`events` → `polling` → `offline`). **Events
are hints** — the pipeline re-derives truth; event fidelity affects freshness, never correctness.
The reconciler is not a component, it's a *schedule*: the pipeline in full-walk mode, run at
startup (+2s), on poll timers (network sources), on volume remount, on demand, and as the fallback
for every watcher failure mode (one answer to all failures). Debounced dirty-path SET (500ms/path,
reset on event) + settle check + mid-processing invalidation handles creative-app save storms.
Rename enrichment: paired OS rename events waive the matrix's name-match requirement (hash still
verifies). **Delete-side merge**: asset transitions to missing + exact content/name lives in a
*recently minted, zero-judgment* asset → absorb (heals copy-then-delete "moves" from unpredictable
external apps). Volume monitor: per-OS (DiskArbitration / mountinfo epoll / WM_DEVICECHANGE),
identity by filesystem UUID never mount path; yanked drives detected by EIO probe. Implementation: `internal/watcher`.

## D15 — XMP sync: 3-way file-level merge, exiftool both directions

Read sidecars + embedded XMP; **write sidecars only** (v1). Known asymmetry, documented for users:
LrC ignores JPEG sidecars, so JPEG write-back waits for P2 metadata-editing (explicit opt-in to
file modification). Conflict model: file-level 3-way using `xmp_hash` (sidecar changed?) and
`judgment_modified_at` (user edited?) → apply / write / conflict-per-policy (`xmp_wins` default).
Tags ALWAYS merge (union), never delete — `asset_tags.source='xmp'` keeps provenance. Two-level
loop prevention: file-level (hash echo check in watcher) + state-level (**sync writer never bumps
`judgment_modified_at`** — the writer-class system preventing a logical oscillator). Flags don't
exist in LrC's XMP: write `alexandria:Flag` custom namespace (best-effort, survives our own
bundle/migration flows), never auto-map onto ratings/labels (lossy mappings are opt-in P3 only).
exiftool daemon mode both directions; merge-into-existing writes; atomic temp+rename. Implementation: `internal/xmp` (remaining scope: the XMP-sync task).

## D16 — Settings: three stores, routed by scope

localStorage = pre-paint needs (theme, locale) + lose-and-shrug window chrome ONLY. Catalog
settings KV = anything referencing catalog entities or painful to lose (keybinding overrides, tree
expansion, per-view prefs under `ui.*` keys, ignore list, import defaults). machine.json = facts
about this machine (worker pools/semaphores, memory limit, dependency path overrides, open-in apps,
telemetry consent) — a small JSON file read before any catalog opens. Contract grows a generic
`getUIState/setUIState` passthrough (UI-owned keys, opaque to Go) beside the small typed Settings
envelope (backend-consumed config). The `keybindings` DB table is DROPPED (one `ui.keybindings` KV
value; frontend owns the action vocabulary, defaults in code, DB stores overrides only).

> **Superseded by `impl/11` (2026-07-07, two rounds).** The *scoping* here (localStorage /
> catalog-scope / machine-scope) still holds, but the *storage mechanism* for the catalog-scope
> tier changes: no `settings` DB table at all — `<catalog-dir>/settings.json`, a plain file
> alongside the DB (this catalog-directory-as-bundle model was already implicit in D9's "DB +
> thumbnails + settings" phrasing). Keybindings move out of catalog scope entirely — a keybinding
> preference is a fact about the person, not the catalog, so it doesn't belong in a per-catalog
> store in any form — to `<app-config-dir>/keybindings.json`, sitting beside `machine.json`.
> Frontend-owns-the-vocabulary, defaults-in-code, file-stores-overrides-only is unchanged from the
> intent here, just relocated and re-formatted.

## D17 — Jobs: registry map now, River later, catalog-as-queue for backfills

No workflow engines (Airflow-class = wrong altitude by 1000×; idempotent re-entry + durable state
at both ends makes durable workflow state unnecessary for ingest). V1 job envelope: jobID +
`map[jobID]cancelFunc` + OnProgress callback. When genuinely durable background jobs arrive
(thumb rebuild at scale, transcription), adopt **River with its `riversqlite` driver** (tested
against modernc.org/sqlite — our exact driver; experimental status: pin versions) behind the same
contract job envelope. Backfills (phash, etc.) need no queue at all: `WHERE phash IS NULL` IS the
queue.

> **River clause refined by D28 (2026-07-12).** Adoption is *intent-lane only* (export,
> conversion, transcode — user commands no scan can reconstruct). The examples this entry
> named (thumb rebuild at scale, transcription-as-enrichment) are convergent-lane work and
> never queue durably — D25/D28's derived-completeness doctrine covers them. The
> `riversqlite` maturity re-check (open question #9) remains the adoption gate.

## D18 — Ignore list mechanism lands early (P0/P1, not P2)

Checked at two chokepoints: scanner front door AND watcher hint intake (a `.tmp` storm never even
churns the debouncer). Baked-in defaults in code; live list per-catalog in settings KV (seeded,
editable, reset-to-defaults). Auditable: import summary tallies ignored-by-pattern separately from
unknown-by-extension.

## D19 — Conventions

Per-OS build-tagged files inside the owning package; no shared `platform` package until two
packages need the same OS-specific thing. Package names: short, singular, by what they provide
(`dependency`, `watcher`, `exttool`-style). Channel discipline: created/wired/closed only in the
one wiring function; stages take directional channel params. `aspect_ratio` as a VIRTUAL generated
column (indexable, NULL-safe) — DB features over app code.

## D20 — Reconciliation is detect-and-flag, not auto-mutate (supersedes the auto-move halves of D9/D10)

**Decision (2026-07-07, close-out of impl/05).** The ingest matrix never auto-changes an asset's
*identity*. It automates only what is unambiguously true about a **known path**: refresh a same-path
edit (reimport), mark a gone path `missing`, add genuinely-new files. Content that reappears at a
**new path** — a move, a rename, a copy — mints a new asset and logs a **pending review row** (kind
*derived* from the matched asset's live status: `duplicate` if present, `probable-move` if missing;
see `impl/DEFERRED.md` §5). There is **no auto-relink, no delete-side merge, no move detection**. The
user resolves moves/duplicates in the review queue.

**The bright line:** auto-act on a known *path* (fidelity — reimport / missing / add); **never**
auto-act on *identity* (relink / merge). The test at every matrix branch is: *is identity being
reshuffled?* If yes, it's a flag, not an action.

**Rationale.**
1. **Trust is the product.** A creative catalog is hundreds of hours of irreplaceable judgment; a
   catalog that silently reshuffles identities underneath the user undermines the one thing that
   matters. Predictable beats clever. This is Lightroom Classic's model — the target user is trained
   on it and trusts it (missing files get a `?` badge and wait for a manual reconnect; nothing
   re-homes itself).
2. **Simplification.** Auto-identity-reshuffle was the single most edge-case-laden feature in the
   engine — rename event orderings, cross-source re-homing, partial-hash collisions, delete-side-merge
   judgment guards. Deleting it removes a whole class of bugs and code paths (`actionMove`,
   `healMovedAway`, `FindMoveHealCandidate`, the relink precedence) at once.
3. **It dissolves the DEFERRED §1 source-scoping bug** — with no mutating verdict left, there is
   nothing that can re-home an asset onto the wrong source. `FindByHash` stays global purely as a
   *duplicate-detection flag*, which is safe by definition.

**What is preserved.** Judgments are never lost: a moved file's rating stays on its now-`missing`
asset, and the review queue's confirm-move transfers it on the user's say-so. Duplicate *detection*
stays (the global content flag) — it just never mutates. Reimport (same-path edit) keeps identity +
judgments, as before.

**Cost, accepted.** More review items for routine copy-then-delete "moves" (they no longer
auto-heal); judgments sit on the missing asset until the user confirms the move; the review/reconnect
UX becomes load-bearing (this is LrC's manual-relink flow, which some find tedious). If review burden
proves too high in real use, revisit — most likely via the user-rules idea below rather than by
re-baking heuristics into the engine.

**Future direction — automation as user-granted policy, not engine default.** Rather than the engine
*deciding* when to auto-act, expose the automations as **opt-in rules the user configures** (per-source
or global): e.g. "auto-relink exact content+name moves within this source", "auto-merge same-name
copy-then-delete". The engine stays predictable-by-default; a power user opts specific automations back
on and owns the consequence. This inverts control — automation becomes a grant the user makes, not a
behavior the engine assumes. Design with source management / settings; the matrix keeps emitting the
detection facts (pending rows) that such rules would consume.

**Supersedes.** The auto-*action* halves of **D9** ("exact content+name match against a MISSING asset
outranks path reimport" — the relink) and **D10** ("exact tiers may drive automatic actions" — now:
exact tiers *propose*; only path-fidelity *acts*). The matched-identity model and the signature
ladder's *detection* role stand; only their licence to auto-mutate identity is revoked.

**Revisit trigger:** measured review burden too high in real multi-source use → build the user-rules
engine, or a narrow, explicitly opt-in auto-relink.

## D21 — LrC catalog migration: engine does structure, Lightroom does its own lossy translation

**Decision (2026-07-07, design-only).** Never hand-parse Lightroom Classic's Develop settings (the
`crs:` payload — Lua-serialized, undocumented, drifts across LrC releases). Instead the migration
requires a documented, one-time, catalog-wide prep pass performed *inside Lightroom*: `Convert
Photo to DNG` on raw masters (non-destructive; bakes Develop history into the DNG's own embedded
XMP, which Photoshop/ACR reads natively) and `Save Metadata to Files` on everything else. The
migration engine's job is metadata + structure — ratings/labels/keywords/captions via the existing
impl/06 XMP field map unmodified, plus collections and virtual-copy/stack relationships read
directly (read-only SQLite connection) from `.lrcat`, the one thing the XMP prep pass can't carry.
`Preflight` is pure read with **zero durable writes anywhere** (not the `.lrcat`, not the catalog
DB, not a scratch file) — cheap to rerun after every prep fix, safe to discard. `Commit` is the
only writing step, and it writes through the unmodified impl/04 pipeline. Fully specced in the LrC-migration task.

**Rationale.** Reimplementing Adobe's proprietary Develop-settings decoder buys nothing — there is
no destination for non-destructive Develop history once editing moves to Photoshop/Pixelmator, so
either baked pixels or ACR-compatible XMP is all either target tool can consume, and Lightroom
already knows how to produce both. This mirrors D15's move (never hand-parse RDF/XML, let exiftool
speak the standard dialect) one layer up: let Lightroom be the authority on its own format. Trust
is the actual product here — a prospective migrator is choosing whether to trust Alexandria with
years of catalog work, so every mechanism is chosen to make "we cannot have touched your library"
provable (read-only URI, before/after hash) rather than merely promised.

**Rejected.** Hand-decoding `crs:` in Go (fragile, no documentation, breaks silently across
versions). A cloud/account step anywhere in the flow (violates NFR-6, and undermines the one thing
this feature is selling). Flattened-TIFF as the primary edit-handoff format (DNG dominates it —
same non-destructive editability, smaller, and Lightroom already has the menu item).

**Revisit trigger:** the catalog-wide LrC prep pass proves impractical at very large library sizes
(multi-day Convert-to-DNG runs, disk-space doubling) → consider a Lua-SDK LrC plugin that automates
prep incrementally; does not revisit the core call against decoding `crs:` ourselves.

## D22 — Tag system: adjacency + materialized path, direct-attach junction, judgment tombstones

**Decision (2026-07-07).** Build the long-deferred tag repository (blocked consumers now exist:
impl/06 keyword union, impl/09 LrC import). Storage shape: `tags` keeps `parent_id` as structural
truth (adjacency) and gains a **derived materialized `path`** (`/rootId/…/selfId/`) for subtree
queries via indexed `GLOB` prefix — Lightroom's `AgLibraryKeyword.genealogy` move, materialized on
the *small* table. `asset_tags` stays **direct-attachments only** (implied ancestors resolved at
read time through `path`, never stored). Hierarchy attaches **leaf-only**; a parent filter expands
to its subtree. Keyword ingest maps `dc:subject` + `lr:hierarchicalSubject` with
`hierarchicalSubject` authoritative and flat names deduped against hierarchy node names; slugs
normalize case/whitespace only (keep non-ASCII), Lightroom's `lc_name`. Colors are a `color_mode`
tri-state (`inherit`/`custom`/`none`) with the effective color **derived, never stored** (recolor/
reparent propagates for free). User removal of a tag is a **judgment tombstone** (`asset_tags.
removed_at`), and sync unions with `ON CONFLICT DO NOTHING` so an observation-class writer can never
resurrect a user-suppressed keyword. Implementation: `internal/sqlite/tag_repo.go` (remaining scope: the tag-system task).

**Rationale.** The dominant read — "tag with N assets under it" — is **result-bound**: returning N
assets costs the same under a recursive CTE, a materialized path, or a closure table, because the
hierarchy walk runs over the small tag tree and the N-row read is inherent. So we optimize only the
cheap, safe part (path on the small table) and refuse the expensive denormalization (per-asset
implied-membership rows: write amplification, implied/explicit bookkeeping, reparent recomputation,
sidecar-writeback hazards). The composite PK `(asset_id, tag_id)` already makes asset→tags a
contiguous indexed seek, so the junction needs no help in that direction. Tombstones fall straight
out of D8: a removal is a judgment, judgments beat observations, so the sync path structurally
cannot undo one.

**Rejected.** Tags denormalized onto the `assets` row (breaks the many-to-many, the reverse lookup,
and per-relationship metadata; only the FTS *text* projection is a safe denormalization). A separate
descendant-materialized `tag_assets`/closure table now (optimizes a read that is result-bound
anyway; its one real win — live descendant-inclusive counts across the whole tree — is not a current
requirement and is named as the revisit trigger). `LIKE` for the path prefix (won't use the index;
`GLOB` does). ASCII-only slugs (would drop CJK/accented keywords). Building the full `TagRepository`
CRUD now (no UI consumer — carve at the second implementation).

**Deferred (named).** FTS ⋈ tags integration (ancestor-inclusion, per-asset text maintenance,
rename/reparent rebuild) pends a dedicated FTS schema deep-dive — the tag repo leaves `assets_fts.
tags` untouched until then. `source='ai'` at P4. Materialized membership/closure for whole-tree live
counts. Tag-UI backend (`Tree`/`Update`/`Delete`/replace-`SetAssetTags`).

**Revisit trigger:** live descendant-inclusive counts on the entire tag tree become a requirement,
or a real catalog's tag tree is deep/wide enough that `GLOB`-prefix expansion measurably regresses →
add the closure/materialized-membership table then.

## D23 — Seam method surface: per-entity services, thin boundary, one error shape (impl/15 Phase 1)

**Decision (2026-07-09, impl/15).** The synchronous seam is **per-entity bound services**
(`AssetService`, `CollectionService`, `SettingsService`, `SourceService`) — each thin, each Wails-
bound, the frontend adapter composes them behind one `AlexandriaAPI` later. Chosen over one
40-method struct: keeps files and tests small and scoping obvious. Resolutions of impl/15 §5's
pre-scoped decisions:

- **Errors: one normalization layer.** Every bound method returns through `normalizeError` into a
  single `ApiError{kind, code, detail}` whose `Error()` is compact JSON (so kind+code survive Wails's
  error→string serialization). Codes are `ErrorCode` consts in `internal/seam`, **published to
  `errors.ts` by the impl/14 generator** by type-checking the package — same single-source mechanism
  as the domain enums, no hand-copied TS list. Only codes the Go side actually produces exist
  (keybinding-conflict is frontend-owned, so it is *not* a backend code).
- **Settings write = whole-object set,** not a partial patch: `GetSettings`→edit→`UpdateSettings`.
  Avoids a reflection-based field-merge for zero benefit (the frontend holds the full object). A new
  setting is a new struct field, not a new method.
- **Boundary context = `context.Background()`** via one `seamContext()` helper. Wails v2 gives bound
  methods no per-call context; these are short synchronous engine calls. The helper is the single
  upgrade point to the Wails startup context if a long-running bound call ever lands.
- **`ast.Validate` version errors made typed.** Changed `internal/ast/validate.go` to return
  `*ErrVersionTooNew` / `*ErrStructure` for bad versions instead of a plain `fmt.Errorf`, so the seam
  can map a too-new query to `query_version_too_new` (impl/15 §3 row #1) rather than swallowing it as
  `unexpected`. Behaviour-preserving for valid queries.

**Deferred (named), not stubbed.** ~40% of the contract surface has no backing engine; building those
is each its own feature. They are recorded with per-row triggers in `impl/DEFERRED.md` §7 (folder
tree, native dialog, open-in, tag management, source delete, disk delete, undo/redo, soft-delete-by-
query, keybinding presets, machine.json), plus the contract.ts/`models` TS reconciliation deferred to
the `wails dev` pass (regenerating `wailsjs/` needs the webkit toolchain, kept off the backend gate
by impl/14). The seam is extensible by construction — each lands as one thin wrapper + one
`boundServices()` line when its engine exists.

**Revisit trigger:** each deferred row is pulled in by its named milestone; the per-entity split is
revisited only if the service count or a cross-entity operation makes composition awkward.

## D24 — The schema-compiler round: one grammar authority, C15 doctrine, canonical decisions (2026-07-10)

The vocabulary/enforcement round (Ari + Claude, 2026-07-10) — full rationale in
`docs/vocabulary.md` and CONSTANTS C15. The decisions:

- **Operators derive from value kind + nullability; fields never enumerate them.**
  `internal/ast/vocabulary.go` is one `FieldSpec` row per field (Kind, Nullable, Suggestable,
  column override); `kindOperators` states each family once; `Nullable` appends the presence
  pair. The per-field compiler registry and `fieldToColumn` dissolved into a kind switch +
  mechanical camelCase→snake_case derivation (one exception: `source → source_id`). This fixed
  real drift: text fields had inconsistent `neq`/`startsWith`; width/height lacked `neq` and
  the presence pair.
- **NULL-negation policy: negation includes absent.** "title ≠ x" matches untitled assets;
  `notIn` matches unlabeled; `notWithin` matches undated; NOT groups compile as true set
  complements (`NOT ifnull((child), 0)`). Leaf-level negatives on nullable columns compile an
  `OR col IS NULL` arm. This is part of the query language's DEFINITION — never a user setting
  (a toggle would make one saved smart collection mean two different things).
- **Scope alphabet = frontend/09's:** `library | folder | collection | tag`; Go gained the
  folder payload (`sourceId` + relative path prefix + recursive flag) and its compile. The old
  `all`/`source` kinds retired pre-release (no persisted queries exist).
- **Sort fields use TokenField spellings** (`capturedAt`, `ingestedAt`, `rating`, `filename`,
  `size`) — one vocabulary for filtering and sorting. ORDER BY tiebreaker fixed to `id ASC`
  always (was following sort direction, violating seam/01 §Additions #4).
- **Unrated = NULL end to end.** The catalog stores NULL; the wire carries null; `empty` is the
  query form for "unrated"; **0 is not a rating**. (The earlier "0 = unrated at the seam"
  sketch in contract.ts is retired.)
- **Datetime grammar: ISO 8601.** `DateValue` wire form is `{anchor: "now" | RFC 3339 |
  date-only, duration: ISO 8601 duration string}` ("-P30D", "PT2H", "P3M"). Calendar-exact via
  `time.AddDate`; time-of-day components (H/M/S) supported; weeks exclusive per the standard;
  zero and mixed-sign durations rejected. Decided BEFORE any query is persisted or the date
  editor exists — no migration, no retrofit.
- **Path comparison: Unicode NFC via `domain.PathKey`.** "Compare keys, open bytes": NFC
  normalization for equality/matching/dedup only; on-disk bytes stay the truth for I/O; no case
  folding. (macOS NFD vs NFC is a phantom-identity minter otherwise — D20's trust rule.)
- **AssetGroup deleted** (domain struct + schema tables — zero consumers, drifted stub). The
  grouping design round (open question #7) re-derives the noun from scratch; the shape it will
  likely take: GroupKind registry + writer-class membership + an assettype Grouping capability.

**Also decided in this round, executed later (their own rounds):**

- **`Source` splits into `Volume` + `Folder`** (identity/portability anchor keyed by filesystem
  UUID + connectivity, vs. tracked root with sync scope; one volume, many folders). Resolves
  DEFERRED §1's open sub-question in the direction the ledger leaned; LrC's
  RootFolder/Folder/File split independently validates it. Owner: the source-management round
  (next). The asset/file logical-physical split is evaluated in that round's design phase.
- **Copies are REAL files, marked and linked — no LrC-style virtual copies.** A minted copy is
  a new file on disk (in an explicit user-chosen location, never silently into a watched tree),
  a new asset, plus a `derived_from` lineage edge. Kills the main driver for the asset/file
  table split. The governing principle, refining D15's scope: **the app never mutates the
  user's files except as the direct, explicit object of a user command** — explicit copy/move/
  rename verbs are in-bounds (future rounds), silent mutation never is.

**Revisit triggers:** NULL-policy — only with a stored-query migration plan (post-release it
resemanticizes persisted smart collections). Scope/sort alphabets — frozen at first release.
Volume/folder + copies — their design rounds may refine details but not the direction without
reopening here.

## D25 — Thumbnails leave the pipeline; enrichment is per-artifact, completeness is derived (2026-07-11)

**Supersedes D12's THUMB-precedes-WRITE clause.** Ingest becomes `SCAN → HASH → MATCH →
EXTRACT → WRITE`; thumbnail generation moves to the post-ingest enrichment system alongside
phash, sharpness, auto-grouping, and future signals. Assets appear in the grid at commit with
an honest per-cell state: **enriching / ready / failed** — three states the grid must render
distinctly.

**Why the reversal.** D12's "no placeholder cards ever" was anti-LrC-half-imported trauma, but
the actual LrC sin was *dishonest* placeholders (gray cells that could mean loading, broken, or
never) — not placeholders per se. Adobe itself moved our new direction: LrC 7.x added
embedded-preview-first import because users demanded a browsable grid during import. Prior art
is unanimous — LrC (`Previews.lrdata` is a deletable, rebuildable side cache), Apple Photos
(background analysis over hours/days), immich (per-artifact job types re-runnable in "missing"
mode), digiKam (lazy viewport-driven thumbs + Maintenance rebuild) — nobody couples enrichment
to the catalog commit. Structural win: ingest sheds its slowest stage, and thumbnails lose
their anomalous privileged seat — one enrichment model instead of one-and-a-half.

**Enrichment model — three properties, in doctrine order:**

- **Per-artifact idempotence.** Each enrichment (thumb, phash, sharpness, …) is independently
  re-runnable; re-running on an enriched asset is a no-op or harmless overwrite. **Per-asset
  atomicity considered and rejected**: artifacts are independent — a failed sharpness pass must
  not hold a good thumbnail hostage or roll it back.
- **Completeness is derived, not recorded.** No "fully enriched" flag, no job journal as truth.
  The missing artifact IS the pending state — D17's "`WHERE phash IS NULL` IS the queue,"
  generalized. Crash mid-enrichment recovers for free: on catalog open (and on demand), a
  missing-artifact scan re-enqueues whatever's absent. This is "events are hints, truth is
  re-derived" applied to derived state, and it satisfies the rebuild-path invariant by
  construction.
- **A failure record, because absence is ambiguous.** "Not yet" vs "never will" (corrupt file,
  unsupported codec) needs durable state: an error row with attempt count — D13's DLQ pattern
  extended to enrichment stages. The missing-artifact scan skips exhausted assets; the UI shows
  the failed state, not an eternal spinner.

**Queue ordering favors the viewport**: when the backlog is deep, thumbnails for what the user
is looking at generate first (digiKam's lazy model). Design detail owned by the enrichment
round. Ingest keeps one concession to speed-to-visible: nothing — EXTRACT already yields
enough (dimensions, orientation) for a correctly-shaped placeholder cell.

**Status: direction decided; implementation unscoped.** The enrichment-system design round
(worker shape, D17's River trigger, job envelope, queue-depth UI) owns the details.

**Revisit triggers:** if real-world imports show the placeholder window is long enough to hurt
(slow thumbnailer + fast grid arrival), revisit *priority*, not *placement* — e.g. embedded-
preview extraction as a cheap EXTRACT byproduct. Per-asset atomicity — only if an enrichment
pair emerges with a genuine cross-artifact consistency requirement.

## D26 — Seam-round close-out: generation, deferral doctrine, and the deviations that stuck (2026-07-11)

Folded from the retired seam build specs (impl/14–16, all shipped 2026-07-09/10) so their durable
rationale survives the files. What the code alone doesn't say:

- **No `EnumBind`; enum members are *discovered*, never listed.** The generator (`cmd/generate`)
  loads `internal/domain` with `go/packages` and enumerates each named enum type's string
  constants — the consts are the single source of truth; `internal/domain` stays pure
  `type`+`const`; adding a const auto-surfaces; a renamed/removed type fails the generator
  loudly. Rejected `EnumBind` because it emits TS `enum` (the frontend forbids it — literal
  unions only), leaks a Wails shape into `domain`, and its hand-authored member list is a second
  sync surface. The generator holds only a manifest of *which type names* to publish. Cost
  accepted: `golang.org/x/tools` as a direct (generator-only) dep.
- **The TS freshness gate lives on the BACKEND path** (`check-generated` in `check-backend`,
  webkit-free), not `check-app`: drift is caused by editing `internal/ast`/`internal/domain`,
  and the person doing that must catch it without the app toolchain. `check-app` re-runs it.
  CI is three path-filtered workflows (backend/frontend/app); the backend job going green
  without gtk/webkit IS the toolchain-isolation proof.
- **Bind the verbs when the engine exists — don't fake them.** ~40% of the contract surface had
  no engine to wrap; those methods were deferred with per-row triggers (DEFERRED §7), never
  stubbed. A seam method without a real engine behind it is a lie the frontend will build on.
- **`Emit` derives the topic from the event catalog** (stricter than `Emit(topic, type,
  payload)`) — one fewer degree of freedom for a producer to get wrong; the catalog is the
  authority (C8). `events_wails.go` is the sole `runtime.EventsEmit` caller, forbidigo-enforced.

## D27 — The docs system: lifecycle over land; state is derived, never recorded (2026-07-11)

The docs-restructure round (Ari + Claude). The old tree mixed four document species with
different lifecycles into land-based directories (`backend/`, `frontend/`, …), and recorded
status redundantly (START-HERE ledgers, ✅ blocks, "Full spec:" pointers into completed work).
The replacement applies the codebase's own doctrine — D17/D25's "the missing artifact IS the
queue" — to documentation:

- **Four species, one home each.** Living reference → `docs/` + package READMEs (updated in
  place, no status, no history). Decisions (why) → this file (append-only). Work items →
  `_project-tracking/` phase directories. Roadmap → `functional-requirements.md`.
- **State = directory; agile vocabulary.** `ideation/ → epics/ → tasks/ → deleted`. Transition
  is `git mv`; the audit trail is git history. An **epic** (needs a design round to decompose)
  is one file in `epics/`; its design round closes by minting ALL child tasks at once (absence
  stays unambiguous: a gone child is landed, never unminted), wiring `Blocked by:` lines, and
  deleting the epic. A **task** is agent-sized: one round, one context window, PR-shaped;
  spec the boundary (acceptance + C/D citations), never the interior. Story/task collapsed.
- **Done = deleted (fold-and-delete).** Completing a round folds durable residue (reference
  docs, a decision entry) and deletes the work item in the same closing commit.
  `git log --diff-filter=D -- _project-tracking/` lists every completed item. No done/
  directory, no ✅ ledgers, no status prose anywhere outside this file — recorded state is
  denormalized state and it WILL drift.
- **Blocked-by-existence.** `**Blocked by:** <filename>`; a blocker that no longer exists is
  done, so unblocking is derived and self-healing. The queue is `ls tasks/` in NN order; next
  up = first item whose blockers are gone.
- **Area is an attribute, not a partition** (the D10 MIME lesson): filename prefixes
  (`backend-`, `frontend-`, `seam-`, `ops-`, `perf-`), no per-area directories, no per-area
  trackers. Reference docs colocate with their code; work items centralize.
- **Enforcement ladder:** structure first (state can't go stale — it's never written), then
  `make check-docs` (pure greps: status prose, durable→work-item pointers, dead relative links,
  filename contracts) run by a **git pre-commit hook** (`.githooks/`, auto-installed by `make
  check` via `core.hooksPath`) with CI as backstop — every finding prints file:line + rule +
  remedy. Skills stay for judgment only (task-pickup / pre-commit-review / docs-reconcile).
  This supersedes repo-hygiene's earlier "no git hooks" call: a hook that runs in milliseconds
  and self-explains is not the annoyance that rule banned. DEFERRED.md is exempt from the
  status-prose grep — recording resolution outcomes is that ledger's contract.
- **Guides are post-hoc.** A `docs/guides/` recipe is written only AFTER the path it describes
  is fully manifested in code (the phantom `docs/guides/` links this round deleted are the
  cautionary tale). Trigger for the first real one: the feature-add runbooks note in ideation.

**Revisit triggers:** if the solo-dev+agents assumption breaks (real contributors), revisit
GitHub issues as the work-item store — the system's species boundaries port cleanly. If NN
ordering fights the blocked-by DAG (parallel tracks), add explicit priority prose to
00-START-HERE rather than renumbering.

## D28 — The enrichment engine: two lanes, artifact state machines, no workflow engine (2026-07-12)

The enrichment design round (Ari + Claude), closing `epics/backend-enrichment.md` per D25's
"the enrichment-system design round owns the details." Tasks 18–22 are the mint.

**The formal model: artifact state machines, not workflow runs.** Each (asset, artifact) pair
walks `missing → queued → running → present | failed(n)`. Three of those states are *derived*
(no artifact / artifact exists / DLQ row) and only queued/running are transient, in-memory.
The dependency graph is nothing but the unlock rule between artifact machines ("sharpness may
leave `missing` once thumbnail is `present`"). Workflow engines model *runs* as state machines;
we model *artifacts* — the run, and all its durable orchestration state, is the thing D25
refused to create. The right analogy is a build system (target exists ⇒ done), not a scheduler.

**Two lanes, one system.** Background work divides by whether its existence is derivable:

- **Convergent lane** (enrichment, integrity/verify scans, staleness): work is derived from
  missing/mismatched artifacts by a scan; no job rows, no run identity; crash recovery = rescan.
- **Intent lane** (export, conversion, transcode, batch rename — all P3): user commands that no
  scan can reconstruct; these get durable rows, retries, resume. **River adopted for this lane
  only, when its first feature lands** (adoption gate: the open-questions #9 `riversqlite`
  maturity re-check). This refines D17's River clause — see the dated note there. The backup round (open question #16) should evaluate the intent lane for
  retry-against-flaky-destination; the schedule itself stays config (overdue-ness is derivable).

Integration is at every layer *above* the state model: one C9 job envelope, one worker budget,
one job-kind registry, one debug surface. Merging the state models was considered and rejected —
a durable queue for derived data is a second source of truth to reconcile forever (the exact
reason River was declined for enrichment despite being welcome tooling for intent).

**No DAG scheduler / workflow engine — with the reach documented.** Airflow/Temporal-class
tools manage execution state; our doctrine derives it, so they'd force the job journal D25
forbids, plus a server process a local-first desktop app must not ship. The legitimate wants
behind the reach are met natively, as four **legibility commitments** binding on tasks 18/22:

1. **One registry file is the whole graph** — every job kind (both lanes) is a row: kind, lane,
   applicability (via the `assettype` registry's capabilities), prerequisite artifacts, pool
   default, timeout policy, priority class, producer ref. Reading one file = knowing everything
   that can happen to an asset. (Registries stay the *storage*; hierarchy is the *presentation* —
   the rendered graph reads as a tree per asset type.)
2. **The graph is renderable** — `cmd/dev jobs graph` emits DOT + ASCII, per asset type.
3. **Boot-time validation** — `MustValidate` topo-sorts the registry: cycles, dangling
   prerequisites, kinds applicable to no type fail the suite (C10 pattern).
4. **A live, domain-vocabulary debug surface** — one snapshot endpoint (queue depths, in-flight,
   budget gauges, DLQ counts) consumed by a dev-harness page now and an in-app dev corner later.
   Anti-goal, learned from pprof: never generic dumps; always asset/kind/artifact vocabulary.

**Execution shape.** One dispatcher goroutine + per-kind worker pools (settings-owned counts,
`machine.json`, mirroring `Workers.Ingest`). Two admission-control layers above the pools:

- **A global weighted CPU budget** (semaphore): per-kind pools shape fairness, the budget caps
  the *sum* — import and enrichment running together cannot oversubscribe the machine. Heavy
  decodes acquire weight proportional to estimated size, bounding peak memory by construction.
  Exposed as a user-facing **effort dial** (paused/low/normal/full) — the trust lever; Go cannot
  nice a goroutine, so admission control is the only throttle, which is why it must exist.
- **Per-device I/O tokens**: spinning media gets depth ~2, SSD/NVMe gets dozens; backlog reads
  ordered by path to reduce seeks. (Batching jobs for dispatch overhead was examined and
  declined: a channel send is ~100ns against ~100µs+ of real work — batch only where the fixed
  cost rivals the work, i.e. the WRITE fsync and the exiftool daemon spawn, both already done.)

**Timeouts are policy functions, not constants**: per-kind `f(size, type) → budget` (base +
per-byte rate), and long-running subprocess kinds (transcode-class) use a progress-resettable
stall watchdog instead of a wall clock. External tools stay behind the `dependency` package;
RAW preview extraction delegates to the exiftool daemon permanently (the per-camera quirk table
is exiftool's twenty-year moat — owning it is anti-scope).

**Catalog write path.** A third writer class, **`derived`** — named for the column class it may
touch (data-model.md's observation/judgment/derived taxonomy), not its first consumer. It may
write derived columns only; structurally cannot touch judgment or observation. All enrichment
results flow through **one batched writer goroutine** (the one-cook rule; SQLite serializes
writers regardless, so two orderly writer goroutines — ingest's and enrichment's — take turns
at the WAL lock rather than contending chaotically).

**Job right-sizing: one kind = one artifact = one DLQ row = one registry row.** Thumbnail is
its own kind; the cheap signals (sharpness, clipping, phash) are separate kinds whose
prerequisite is the thumbnail *artifact on disk* (re-decoding a 512px thumb costs single-digit
ms — the price of per-artifact atomicity, paid knowingly; fusing is a later optimization that
must arrive with measurements). Heavy signals (faces, blink, embeddings) mint nothing now: each
is a future registry row at its P3/P4 milestone.

**Priority: hot lane / cold backlog, hints never truth.** The dispatcher's in-memory pending
queue is the *only* place priority exists (no DB priority column). Viewport hints replace the
hot lane wholesale (latest wins; frontend debounces ~150ms); the cold backlog orders by import
recency. When heavy kinds land (P3/P4), within-set ordering by cheap signals — sharpest first,
likely keepers get heavy scores first — refines the hot lane; the FR's signals block carries
that promise. In-flight jobs are never preempted. A confused queue degrades to suboptimal *ordering*,
never incorrectness — that is the invariant that keeps UI state safely coupled to the engine.

**Per-asset visibility: pull-decorated, never event-streamed.** Transient queued/running state
lives in an in-memory tracker (`map[assetID]bitmask`, RWMutex); seam asset responses are
decorated with the in-flight bitmask; the frontend derives done/running/pending per artifact
from (data present / bit set / neither). Events stay aggregates (C9 progress ticks with queue
depth, batched `catalog/changed` invalidations) — thousands of state transitions per second are
bit-flips, not envelopes. Write ordering: DB write → clear bit → emit; so an invalidation never
races the frontend into stale reads.

**Cancel dissolves into pause.** Enrichment has no run identity, so there is nothing to cancel:
**pause** (global or per-kind) stops dispatching, in-flight jobs finish, the backlog sits;
resume = dispatch again; app quit = pause. Rollback never exists (per-artifact idempotence,
D25). Per-asset cancellation exists only as a consequence of asset deletion (context cancel).

**Staleness is a transition, not a state.** The only legitimate byte-change moment is reimport
(`actionReimport`, same-path edit; watcher edits funnel there). That verdict's transaction also
**clears the asset's derived columns** — derived state instantly reads "missing," the next scan
re-enqueues. No stale flag, no generation counters, no per-artifact provenance stamps (rejected
as bookkeeping bloat unless a real gap appears). UX nicety: clear `thumbnail_at` but keep the
thumbnail file — the grid shows the outdated-but-real thumb while regeneration is pending; the
content-addressed URL cache-busts on completion.

**The enrichment DLQ is its own table** — `enrichment_errors(asset_id, kind, reason_code,
message, attempts, last_attempt_at)`. Not an `import_errors` extension: import failures are
path-keyed (pre-identity), enrichment failures are (asset, kind)-keyed (post-identity);
different natural keys, different tables, same D13 pattern. The missing-artifact scan skips
attempt-exhausted rows; the UI renders failed, never an eternal spinner.

**Revisit triggers:** a workflow/DAG engine only if a lane appears whose state is *neither*
derivable *nor* simple intent rows (none is foreseen); job fusion only with profiler evidence
that thumbnail re-decode dominates a signal's cost; per-artifact provenance stamps only if
clear-on-reimport proves too coarse in practice; the effort dial's shape (token counts per
level) is implementation detail, tune freely.

*(2026-07-13, task-18 build round — the engine landed as specced; three in-round calls worth
recording. **Engine scans are a different lane than the query authority:** the missing-artifact
scan and eligibility probe are hand-written SQL in `sqlite.EnrichmentRepo`, deliberately NOT
routed through `internal/ast` — the AST is the user-predicate contract between frontend and
catalog; engine plumbing (DLQ anti-joins, artifact-column probes) must never leak into that
grammar, and the AST must never care about engine internals. The artifact-column allowlist in
that repo is what stands between registry data and the SQL. **I/O tokens shipped per-source,
not per-device** — device detection is its own layer (DEFERRED §11 has the trigger); path-
ordered backlog reads deferred with it. **Jumbo weights clamp to the dialed capacity**, so an
over-budget job serializes against everything rather than deadlocking against tokens the
effort level will never release; the effort→token mapping (full/half/quarter of NumCPU) is a
stated assumption awaiting the gospan samples-table calibration (D30).)*

*(2026-07-14, the naming-and-structure round (Ari) — the engine re-grounded in its own graph
model before anything shipped on it. **Vocabulary:** the registry row is a `JobDefinition`
(node); a `job` is one execution (its identity `JobKey{AssetID, Kind}` — the operand is the
ASSET; "artifact" vocabulary stays on the catalog side, where the artifact actually lives);
queues hold jobs; workers run them; `KindSet` replaces the pipeline-borrowed "Stage" for the
tracker bitmask. **Structure:** the hand-rolled hot/cold lists (steal flags, husk discard,
served-dedup — a bespoke reimplementation of heap.Fix) were replaced with one
`container/heap` per node whose `Less` is the composite key (hinted band, hint order, import
recency) — §7's use-the-stdlib rule, enforced late; the pre-commit checklist now flags
bespoke stdlib-container reimplementations. **Edge emission:** an applied completion enqueues
the node's dependents directly (inheriting the asset's live hint priority — the fast track
carries through the graph at every step); scans demote to authority, cold start, and refill.
**Artifact handoff:** producers' outputs never ride queues (retained memory outside admission
control, and edges are claims — rescan recovery needs the disk path regardless); when the
fusion trigger's measurements arrive, the sanctioned shapes are a bounded lookaside cache or
decode fusion.)*

*(2026-07-14, task-19 build round (Ari + Claude) — thumbnails left ingest; three in-round
calls worth recording. **The thumb capability is a strategy VALUE in the assettype table:**
`thumbnailer.GenFunc` became a method value on the `Thumbnailer` struct (`GenerateRaster` /
`GenerateRawPreview` — the receiver, passed at call time, carries every runtime dependency:
output layout, the exiftool daemon), so the static table stays the single capability truth,
applicability stays `handler.Thumb != nil`, and no second dispatch layer exists. Rejected: a
producer-side type conditional (capability truth leaks into code that drifts from the
applicability predicate — the C10 rot pattern) and a strategy-enum with a constructed
strategy registry (a second registry stacked on the one the table already is). Nil
degradation is two-level and loud: nil `Thumb` = the format has no capability (never
enqueued); nil daemon at runtime = a `tool_unavailable` DLQ row, never silent
inapplicability. Ceiling noted at the `GenFunc` definition: a strategy needing per-call state
not derivable from (Thumbnailer fields + source path) reopens the shape. **Corrupt-pixel
discovery moved engines with the decode:** ingest never decodes, so a valid-magic
undecodable file now commits with NO `import_errors` row — the decode failure is the
thumbnail job's `enrichment_errors` row. D13's self-heal loop is unchanged but cross-engine:
the reimport staleness clear wipes the DLQ row, the scan re-derives the work, and the
corrupt→heal acceptance test walks both engines. **The exiftool stay_open transport is
binary-safe now:** `-b` output carries no trailing newline, so the `{readyN}` ready marker is
matched as a line suffix instead of a whole line — the line-based reader would have hung
forever on the first real preview extraction.)*

*(2026-07-15, task-20 build round (Ari + Claude) — the cheap signals landed as the engine's
second, third, and fourth kinds; the round's calls, most of them scope-narrowing. **The signal
math is pure functions in a new `internal/signals`, hand-rolled, no imaging dependency:** the
"real OpenCV for Go" (gocv) is cgo + a system OpenCV install — banned by the no-cgo/dependency-
package invariant — and the pure-Go options (bild, imaging) aren't load-bearing for one Laplacian
kernel + one histogram + one hash (redundancy test); `internal/enrichment/signals.go` stays thin
producer glue. **phash is dHash, a swappable `PerceptualHasher` strategy value** — pure Go, no
DCT, robust for near-dup ranking; a DCT-based pHash (goimagehash) is a one-line swap if recall
ever falls short. **Sharpness is stored RAW (variance of Laplacian), no normalization:** ranking
is the contract, not the absolute value (the epic's own words), and building a within-burst
ranking scale when no burst concept exists yet is inventing a consumer; 512px is the analysis
input (prior-art-standard for the measure, and the thumbnail the signal's prerequisite already
guarantees) — contrast-normalization (`var/mean²`) and a JPEG-artifact pre-blur are documented
upgrades behind markers, neither with a consumer. **clipping is one kind writing two columns**
(highlights + shadows from one histogram pass; the scan marks on `clipping_highlights`, both
clear together). **The query surface was narrowed below the original acceptance line:** only
sharpness + clipping got `ast` FILTER vocabulary (the C11 "propose as data, dispose via query"
contract); phash's query surface (hamming/near-dup) defers with clustering, sort-by-signal defers
(no cull UI, and sort is already a curated subset), and signal DISPLAY (`catalog.AssetRow`)
defers — all in DEFERRED §12. The stored columns are the durable half; each deferred surface is a
small add onto them when its UI consumer lands. **The `derivedArtifactColumns` allowlist is now
dual-purpose** — the set ClearDerived nulls on reimport AND the pool a registry row's marker must
be drawn from — so a multi-column kind lists every column but marks one.)*

*(2026-07-15, task-21 build round (Ari + Claude) — the seam face of the engine; the round's
calls. **Visibility ships as KIND NAMES, not a bitmask:** the spec's "enriching bitmask" would
need generated bit constants matching the engine's registry-ROW-ORDER bit assignment — fragile
and not self-describing — so the seam ships `[]domain.EnrichmentKind` (a generated union pinned
to the registry by a crosswalk) and the `KindSet` bitmask stays an engine internal. The engine
exposes `RunningKinds`/`FailedKinds` (kind names); the reverse lives in the engine, the seam
never sees a bit. **Read is pull-decorated, write is a thin service, events are aggregate.** The
grid gets `AssetRow.Enriching` (one tracker lock per page) + `AssetRow.Failed` (one DLQ query,
attempt-exhausted only — a retrying row is still pending); decoration is best-effort (a DLQ read
error drops the failed half, never fails the query) and nil-safe (no engine = no decoration).
`EnrichmentEngineService` wraps the engine's pause/resume/effort/hint behind a STRUCTURAL interface, so
the seam package never imports enrichment; the effort dial persists-then-applies (a crash leaves
the durable intent). Aggregate events ride the existing envelope (no new topic): `JobProgress`
gained an optional `QueueDepth` (the convergent lane has no run identity, so done/total stay 0
and the per-kind backlog is the signal), emitted by the free function `EmitEnrichmentBatch` off
the engine's `OnBatchCommitted` hook — a free function, not a method, so the composition captures
`engine.QueueDepths` and no construction cycle forms; the DB→clear-bit→emit ordering holds
because the hook fires after the tracker clears. **No live wiring, by necessity:** no Wails host
binds the seam services yet (frontend-rebuild epic), so this round is contract + capability +
tests — the engine-through-the-seam controls are proven with a real engine in tests, the effort
store is faked (its machine.json adapter lands with the host). **Narrowed:** the per-asset detail
read (error reason codes) and the `blocked` derivation (a kind whose prerequisite failed) defer
with the loupe/inspector that renders them (DEFERRED §13) — the grid's three D25 states
(enriching/ready/failed) are complete without them.)*

*(2026-07-16, task-22 build round (Ari + Claude) — the visibility half: commitments #2 and #4
made concrete. **The snapshot is gauges, harvested for free; distributions are gospan's.** One
`Engine.Snapshot` — the dispatcher builds its scheduling half on its own goroutine (per-kind
queue depths split hot/cold/running, in-flight jobs, pause state — one pass over the ledger,
no lock), the engine fills the effort dial, the budget gauge (a new `inUse` atomic on the
weighted semaphore; the reservation manager bypasses it, so `InUse ≤ Capacity` always), and the
DLQ rolled up by (kind, reason) via one grouped query. Everything reads state the engine already
holds; the round explicitly refused per-kind duration histograms + per-(kind, asset) token cost
(new per-job instrumentation duplicating gospan's spans — the trace file answers distributions by
SQL, D30), DEFERRED §14 holds the line and its trigger. **The JSON shape IS the contract** (json
tags, `internal/enrichment`), served raw at `/enrichment/snapshot.json` for the future in-app dev
corner; its TS types get generated when that second consumer lands (C15), not before — the
snapshot is an operational debug feed, NOT the seam's frontend contract (task 21's services), so
it lives on the engine and the harness serves it, no seam involvement. **The matrix collapses
queued into pending:** the dev page's asset × kind cells read done / running / failed / pending —
"queued" is a transient in-memory scheduling optimization with no durable per-artifact meaning
(the missing artifact IS the queue), so dumping the ledger per-asset is the transient-dump
anti-pattern; queued stays visible as an aggregate per-kind depth instead. **The graph renderer
is pure** (`RenderGraphDOT`/`RenderGraphASCII` over `Definitions(nil,nil)` + the assettype table,
asset types with identical applicable-kind sets collapsed to one sub-graph); `dev jobs graph`
renders it, `dot -Tsvg`-clean. The dev-harness page is hand-rolled HTML + meta-refresh poll,
stdlib only (DEFERRED §9's constraint stands; the import-pipeline `/state` half stays deferred).)*

*(2026-07-16, epic pre-merge review round (Ari) — one contract call, settling the review's one
Important finding. **The convergent lane is a STANDING job — never terminal.** The task-21 round
had a drained backlog report `state: "done"` on the progress tick, contradicting the JobState
contract (`internal/seam/events.go`: terminal states ride the done event exclusively) — and a
lane that new imports/reimports/hints un-drain would terminate and resurrect, forcing exactly
the special-casing C9 forbids on a generic renderer. Fixed the code, not the contract:
`EmitEnrichmentBatch` always emits `running`; a zero `queueDepth` total IS the drained signal
(the frontend hides the indicator at zero). **Per-import enrichment status ("import X is 80%
enriched") was considered and deferred, not refused** — the convergent model makes it a pure
read-side aggregate over catalog truth, needing only the missing asset→session linkage (an
`import_session_id` observation column) + a seam read; run identity stays out of the engine.
DEFERRED §15 holds the shape and its trigger (the frontend surface that renders it).)*

## D30 — gospan adopted: the pipeline is span-traced; trace files are exhaust (2026-07-13)

The gospan validation round (Ari + Claude): [gospan](https://github.com/akmadian/gospan) —
Ari's embedded span tracer, designed with this pipeline as its motivating workload — was wired
into the import pipeline and validated against real and synthetic imports. It stays.

**What was adopted.** `Importer.Tracer *gospan.Tracer` (nil = off, ~4ns; the entire test suite
runs untraced, which IS the nil-off proof). One span vocabulary, dotted per area:
`import.run` (root) / `import.scan` / `import.asset` + `import.sidecar` (one trace root per
item, riding the item as `pipelineItem.ctx` — the gospan pipeline recipe) / per-stage child
spans ended **before** the downstream send (gap = queue time) / `import.await-commit` (write
wait, deliberate: spans are cheap and it saves a query) / `import.write-batch` (each commit its
own tiny trace; items and batch share a `batch_seq` attr — fan-in stays flat, never structural).
Enrichment (task 18) gets `enrichment.<kind>` from birth on the same tracer.

**Doctrine placement.** A trace file is **observational exhaust** — below derived state: the
program never reads it back, deleting it loses diagnostics and nothing else, so it carries no
rebuild path and never enters the catalog (no writer-class or one-cook implications; gospan owns
its own file). The dev harness traces by default (`--trace=false` for A/B); the shipped app
constructs no tracer until the app-host round decides otherwise — nil-is-off makes that a
composition-root choice, not a code path. Analysis is plain SQL: the script library in
`cmd/dev/sql/` (trace-report, trace-asset, catalog-stats, catalog-wipe) is the standard kit.

**Validation evidence (why it stays).** 2,000-file A/B: ~2.5% throughput cost at ~2,000
files/s (small-file worst case), 26,775 events written, zero dropped at the default buffer,
self-reported overhead <1µs/span. A real 1,330-photo import produced the round's headline: the
parallelism query showed `import.thumb` at 5.98x on a 6-worker pool — 437s of 460s total work —
i.e. **the run time IS thumb time ÷ pool size, measured**: D25's "thumbnails leave ingest" now
has its empirical justification, and task 19 can diff before/after trace files as its receipt.
A SIGTERM mid-run left exactly the in-flight items as incomplete spans with the run span
auto-classified `canceled` (the insert-at-start bet, confirmed). The traces also surfaced a
real pipeline gap on day one (duplicate-verdict items skip EXTRACT → assets with thumbnails but
no metadata; flagged as its own work item).

**Reading discipline (folded into the importer README + trace-report header):** aggregates
convict, waterfalls narrate. A bounded-channel pipeline parks items at whatever upstream seam
has buffer room, so a single item's biggest gap points wherever it happened to queue; the
per-name parallelism view (pool pinned at its size) names the true bottleneck.

**Rejected/deferred:** injecting a shared DB handle into the sqlite sink (breaks the sink's
schema/pragma/one-file-per-run ownership and would put exhaust inside backed-up state; the
live-read want is met by opening a read-only handle on `Sink.Path()` — WAL readers never block
the writer). Instrumenting inside `internal/metadata`/subprocess leaves — the extract span
covers it; add leaf spans when a real diagnosis wants them. RAM/CPU sampling is a gospan-side
design round (its DEFERRED "gauge samples table" trigger has now fired; first consumer is
calibrating D28's weighted-budget size estimates against measured heap).

**Revisit triggers:** `Stats().Dropped > 0` on a real workload → tune `WithBufferSize`;
app-host round decides the shipped-app tracer (and whether the D28 debug surface embeds
`Summary()`); the gospan viewer landing replaces hand-rolled waterfall queries. Standing
appointment: a fully-instrumented enrichment engine + concurrent import projects 30–50k
spans/sec against a ~100–150k/sec sink ceiling at typical attr counts — re-run the sink
benchmark against task-18's real span rate before the enrichment engine ships (gospan's
DEFERRED "write-throughput ladder" holds the pre-planned moves, multi-row inserts first).

## D29 — The design system rebuilt as a token-source product (2026-07-12/13)

**The token source is the product; everything visual is downstream of it.** The design
system was rebuilt ground-up across 2026-07-12/13: `docs/design-constitution.md` (§1–§30) is
the design law, and `frontend/design/` is the source of truth it governs —
`tokens.resolver.json` + `tokens/{primitives,semantic}.tokens.json` +
`tokens/worlds/hues-{light,dark}.tokens.json` + `tokens/themes/*.tokens.json` (DTCG 2025.10 +
Resolver Module: structured values, aliases, themes as modifier contexts), with
`contracts.json` (validator input) and `registries.json` (dispatch tables) beside it — never
inside it. Individual design ratifications are recorded **in the constitution** (§29, dated;
§30 worksheet carries tombstones for closed pins) — this entry deliberately does not
duplicate them (D27).

**No frozen legacy:** the interim runtime generator, its frozen `tokens.json` snapshot, and
the old swatch library were deleted once they went stale — a runtime injector is not what we
would build (the pipeline is build-time), and a tool rendering outdated values
authoritatively poisons future sessions. `frontend/src` therefore has no token plumbing and
does not compile, deliberately. Phase C is the code round's first task: one compiler emitting
CSS variables + TS types + docs from `tokens/`, and a validator in `make check` executing
`contracts.json` (APCA per theme, ΔL bands per adjacent register step, hue-distance floors,
quantum membership, ledger exhaustiveness).

**Method ratified for design work:** design sessions ship tokens + constitution amendments,
never implementation code; every value is rendered (library probe pages with real fonts /
live-APCA widgets) and eyeballed before ratification; changes land at the lowest sufficient
tier — role remap before ramp move, register-step multiples for state nudges, a registry row
for a new capability. Orientation: `frontend/design/CLAUDE.md`.

**Superseded and removed:** the 2026-07-08 visual-language epic and its design-handoff
bundle — fully absorbed by the constitution — were deleted from the tracking tree (D27
fold-and-delete); the pointers to them in `docs/frontend-architecture.md` and the
flows-and-views epic were repaired. `frontend/CLAUDE.md` rewritten (the old one taught the
deleted ds-reference system); `frontend/design/CLAUDE.md` added (proximity-loaded design
orientation).

## D31 — Phase C executed: the bespoke token compiler, and the first machine certification (2026-07-17)

**The D29 Phase C compiler landed** as a bespoke frontend-side program
(`frontend/design/compiler/`, TypeScript run by bun; `culori` + `apca-w3` as its only
dependencies — the reference APCA implementation the contracts cite): **resolve** (the DTCG
2025.10 resolver contexts → four flat, alias-free theme trees) → **validate** (`contracts.json`
executed in full; any failure exits nonzero and emits nothing) → **emit**
(`src/styles/tokens.css` with `:root` = the default theme and varying-only `[data-theme]`
overrides; `tokens.ts` carrying the generated theme vocabulary that `lib/theme.ts` now imports
(C15 — its hand-listed pre-v3 names were live drift against index.html); and
`tokens-reference.json` for the design library). Freshness-gated in `bun run check` by the
`check-generated` pattern (regenerate + `git diff --exit-code`), frontend-side. Emitted-name
contract and the adjudications below are recorded in constitution §29 (dated 2026-07-17).

**Bespoke over a library, evaluated not assumed:** Style Dictionary cannot read the 2025.10
Resolver Module (a parallel SD config re-declaring the resolver's contexts is a C15 defect);
Terrazzo's contrast lint is WCAG2-shaped and its modes are not the resolver file. Either would
replace only the trivial resolve third while the validator and the house emission shapes became
plugin code inside someone else's API. Revisit trigger recorded in DEFERRED §16.

**The validator's first pass over the ratified palette found 13 real conflicts** — every one a
PIN hypothesis target written 2026-07-12 colliding with a value Ari later ratified by eye.
Adjudicated same day (Ari): per-polarity text targets (the dark ink retune stands; dark
polarity is derived from `familyDirection` — raise ⇔ dark, never re-declared); the §25 selected
promote rule; the world-varying per-hue **ring step** (structural finding: world-independent
solids measure Lc 20–43 against dark panels, so the flat ring contract was unsatisfiable for
any solid — same shape as the tint/line world split). The first eligibility computation is
itself new information: the dark panels now pass for every hue, but seven hues (peach, orange,
amber, lime, green, teal, cyan) fail the LIGHT panels — their light-world ring aliases the
pre-existing solid — leaving 5 of 12 accent-eligible (red, blue, indigo, purple, magenta).
Whether that attrition stands or the light-world ring detaches from the solid is the
token-gaps round's call, alongside the dark ring's eye-tune. Register-step multiples and sRGB-gamut
excess are validator WARNINGS, not failures: the ratified ramps predate the register-step's own
tokenization, and §28 already tracks P3 headroom — promoting either to a failure is the
token-gaps round's call, with Ari's eye at the gate.

*Dated note (2026-07-17, the task-24 round):* type-role unit classes emit inside `:where()` —
zero selector specificity, so the class supplies role DEFAULTS and any component declaration
wins regardless of stylesheet injection order. Found live in the browser pass: equal
specificity left the fill rung's label ink-on-ink on carbon.

## D32 — The material round, Stage A: chrome selection is a light pill, split from the register (2026-07-22)

**Decision.** The reference vocabulary (the `Figma-ish` set) renders a selected chrome item
as a **light "pill" just off the surface anchor + an ink/weight jump**, not a dark fill. Our
system did the opposite because `surface.selected` (paper 0.888) was **overloaded**: it was
both the selection fill (tabs, toggle-ON, list rows) *and* the deepest register step (the tint
Button's pressed fill), and on light themes the §7 "interaction moves toward ink" invariant
made the selection *darken* — a dark slab, the backwards direction.

**The split.** Selection leaves the register and becomes its own token. Added `surface.pill`
(a light selection fill, direction-EXEMPT — deliberately NOT in the surface `stepOrder`, so
monotonicity is untouched); `surface.selected` stays as the deep register step. Selection is
carried by **`ink.1` + semibold** — the ink/weight does the findability work, the pill fill
stays calm. Paper was pole-anchored so the pill has a ground to rise from: `surface.panel`
0.975→1.0, `surface.hover`→0.97, `surface.transient`→1.0, `ink.1` 0.25→0.15; `radius.control`
re-pinned 4→6.

**Eye-gate.** Paper ratified by Ari against `frontend/design/library/material-probe.html`
(the L-A / pole-anchor candidate). `linen` + the dark themes got `surface.pill` ONLY: linen's
full pole-anchor is deferred (pole-anchoring its panel fights the hairline seam contract's ΔL
band and erases its warm-dim identity — the "wrong tier, reframe" rule); the dark themes
already RAISE (§7), so their selection reuses the existing raised step. Contracts hold; the
compiler's fixture tests were updated to the new ratified values.

**Spatial model reaffirmed (the frame for this whole round).** Docked + tonal. The Figma
*floating-card + shadow* look is explicitly NOT adopted — shadow needs a canvas to fall onto,
which a dense docked layout does not have; dimension comes from tone (pole anchor + pill), not
elevation. §2 (cliff), §3 (separation ladder), §6 (shadow = occlusion, transient-only) stand.

**Deferred to their own stages/rounds:** the CODE round migrating consumers onto `surface.pill`
+ ink/weight + radius 6 (workspace tab, ToggleButton, and the SegmentedControl rebuild); two
line grades (§3, Draft C); flat-white buttons / fill-only fields / tint chips (Stage B); canvas
dots + transient r12 (Stage C); the linen + dark anchor eye-gate; and the eventual
`surface.selected`→`surface.sunken` rename cleanup. The §7/§29 constitution prose still needs
the refinement written in (this entry is the authority meanwhile, per D27).

**Revisit trigger:** if the pill fails "findable at a glance" at real density, promote to a
stronger step (the probe carried a 0.925 "CAND stronger" variant).

*Dated note (2026-07-22, the same round, carried through code + eye-gate on all four themes).*
The split resolved into a **three-token selection vocabulary**, learned by rendering the
segmented control across every theme:

- `surface.pill` — **on-panel** selection (tabs, toggles, list rows sitting directly on
  panel): a dab just *below* the anchor so it reads on paper's white panel. paper 0.945 /
  linen 0.93 / graphite 0.405 / carbon 0.32.
- `surface.raised` — the **in-well** selection ceiling (the segmented-control pill, rising out
  of a recessed track): sits *at-or-above* panel in EVERY theme, so it is the **brightest
  element** and reads by value alone — **borderless**. paper 1.0 / linen 0.98 / graphite 0.44
  / carbon 0.36. The two tokens diverge only because paper's pole-anchored 1.0 panel leaves no
  headroom for on-panel selection; on dark they nearly coincide.
- `surface.sunken` — the **deep track** grouped controls rise from (distinct from `surface.field`,
  the shallow input well): a solid register step below panel in every theme so the pill
  separates tonally. paper 0.91 / linen 0.85 / graphite 0.24 / carbon 0.155.

**The load-bearing diagnosis (why this shape):** the pill communicates "selected" by being the
brightest thing above a dropped ground — figure/ground by pure value. That only works if the
selection surface reaches the theme's light extreme, which a *compressed* (non-pole-anchored)
ramp cannot give it — hence the border was doing the work on linen/dark. `surface.raised`
manufactures that ceiling per-theme without re-tuning whole ramps, so linen/dark keep their
identity and stay non-pole-anchored. Confirms: the borderless selection read REQUIRES the
raised ceiling; the earlier `pill = panel` attempt failed on every theme but paper (whose panel
*is* the ceiling).

Also landed this round: `size.control-inset` (2px, registered sub-grid exception like
icon-stroke) — the grouped-control track inset at the dense 24px height, size-proportional
(control-inset on `control`, `space-1` on `control-lg`, both a 20px segment); the code round
migrating workspace tab + ToggleButton onto `surface.pill` and rebuilding SegmentedControl on
the pill/sunken/raised grammar; Stage B controls (fill-only fields on `surface.field` fixing a
`cell.well` misuse, the neutral machinery chip → tint, buttons already flat-white via the
anchor); `radius.transient` 8→12.

**Still deferred:** the broader page **canvas** (dropped grey + Draft-G dots) is untouched, so
the pill-vs-page read only exists locally (pill-vs-track) — a full canvas is the layout round;
moving tabs/toggles onto the lifted read for consistency; and the `surface.selected`→`sunken`
rename cleanup (blocked only by naming, `surface.selected` is now purely the tint-pressed deep
step).

## D33 — The control-size round: a three-rung derived ladder (2026-07-22)

**Decision.** Chrome sizing was two magic height literals — `size.control` (24) and
`size.control-lg` (28), pinned by §8 as "two control heights." Two problems: there was **no
interactive control below the 24px hit-target floor** (the inspector renders read-only metadata
dense at the 16px line-box, but had no control small enough to sit inline — e.g. a click-to-edit
field), and heights were **hand-maintained literals**, so "scale it down" meant inventing and
reconciling more numbers.

**The formula.** Every dense type role already shares `line-height: 16` (the ratified "two sizes
do the work, both lh 16" decision). So the whole vertical ramp is **one derivation**:
`height = 16 (the dense line-box) + 2·breathe`, breathe ∈ {0, 2, 4, 6} → **16 / 20 / 24 / 28**.
Breathe is **realized by centering** (`align-items: center`), never authored as a vertical
padding token — so heights stay on the 4px grid and no off-grid rung is minted. To scale a
control down you drop a rung (and, if needed, step its type role to a smaller line-box); only the
horizontal inline pad steps by tier (`space.2`/`space.3`/`space.4`). This is the Primer control-
sizing pattern (`height = lineBox + 2·padBlock`), grounded further in native pro-desktop density
(macOS regular/small/mini; Lightroom/IDE ~18–22px rows), not web-framework defaults (which bottom
out at 24 — Ant/Spectrum — or 28/32 — Primer/Carbon).

**The tokens.** Added `size.control-sm` = 20; renamed `size.control` → `size.control-md` (24, the
default); kept `size.control-lg` = 28. 16 is **not** minted as a control size — it is the line-box
atom the ladder derives from and stays display-only (`row-text`), because a 16px interactive
control would break the 24px hit-target floor. `control-sm` is display-dense's interactive twin:
read-only stays `row-text` (16); **editable** dense sits in `row-list` (24) hosting a `control-sm`
(20). `control-sm` is visual 20 but its interactive hit target still fills 24 (fills its row, or
transparent block padding standalone) — no interactive element drops below the §8 floor.

**The usage API.** A t-shirt `size` prop — `sm` | `md` | `lg` (default `md`) — set **explicitly
per control** (no density cascade). SegmentedControl offers `md`/`lg` only: a 20px track crops its
segment to a cramped 16px. Consequence: §8's "density switches at section boundaries, not
row-by-row" is now **usage discipline**, not enforced by construction — a future usage-lint if
drift shows. Migrated Button / ToggleButton / TextField / SegmentedControl + every call site and
test off the old `control`/`control-lg` prop values.

**Amends** §8 (two → three control heights + the derivation rule) and closes the control-height
half of §30 item 5, disambiguating the stale "44/28px" note (44 = the top-chrome *bar* band, a
separate open axis, never a control height). Icon stays one chrome size (16px; 2px inset in a 20).

**Still deferred:** the **inspector inline-edit** affordance itself (the motivating consumer) —
its own surface. It will also extend the `Row` grammar, which today restricts children to
`control`-intent (28) rows; hosting a `control-sm` in a `row-list` (24) row lands with that
consumer. Standalone `control-sm` hit-area expansion ships when the first standalone dense
icon-button lands (documented on the token; no consumer yet).

**Addendum (same day) — the fourth rung + sizing the indicator primitives.** 20px still read
too large for the metadata inspector, so the ladder gained a fourth rung: `control-xs` = 16 (=
the line-box, breathe 0), the **inspector inline-edit tier** — visual 16 matches the read-only
`row-text` row exactly, so a display→edit swap is zero layout shift. It is interactive *below*
the 24px hit-target floor by design: a sanctioned **desktop-mouse-only** exception (§28), justified
because the inspector's inline controls are full-width (large horizontal target even at 16 tall).
§8's hit-target rule was amended to name this lane rather than let it read as a silent violation.
`xs` was added to Button / ToggleButton / TextField (SegmentedControl still excludes it *and* `sm`
— a 16–20 track crops the segment). Checkbox / Switch / Rating gained the same `size` ladder, but
**§14 pins their 16px indicator** (box / track / star are chrome icons) — so their tier scales the
label role + the min-height that carries the hit target, never the mark. A useful side effect: it
lifts those three off their old flat-16 (sub-floor) targets at sm/md/lg. Whether the indicators
should ever scale (a §14 amendment) is left open — parked until an eye-gate says the pinned mark
reads wrong at a tier.

**Addendum 2 (same day) — the proportional pivot.** The eye-gate came immediately: a tier that
scales only its height (constant text/icon/indicator) reads as the *same control stretched*, not a
smaller one — xs and sm text were indistinguishable. So the model moved from **A (Primer: constant
line-box, height = text + padding)** to **B (proportional: the tier height is a container every
metric scales into)** — how Spectrum / Carbon / Ant work. A tier is now a full bundle: **text**
(a `control-text` type ramp — control-xs 10 / control-sm 11 / control 12 / control-lg 13, medium),
**icon** (a `size.icon` ramp — 12 / 14 / 16 / 18; 14 and 18 are 2px sub-grid, added to the quantum
exemption like control-inset), **indicator** (checkbox box / switch track+thumb / rating star, all
derived from the reassigned local `--alx-size-icon`), **inline pad**, and **height**. Mechanism:
each component's size class composes its tier's `control-*` text role and reassigns `--alx-size-icon`
— everything reading that var (the Icon, the checkbox box, the switch geometry `calc`s, the rating
stars) scales for free. SegmentedControl extended down to `sm` (20 track → 16 segment, the floor;
xs excluded). This **reverses Addendum 1's §14 indicator-pin** and **amends §14** (one icon size →
a per-tier ramp) and the **"two sizes do the work" type rule** (control text is now a 4-step ramp).
The height formula `16 + 2·breathe` survives as the height rung; the other metrics get their own
per-tier ramps. The text ramp is **split by the pre-existing weight convention**: interactive
labels (buttons / toggles / tabs / segments) ride the **medium** `control-text` ramp; CONTENT text
(TextField input, Checkbox / Switch labels) rides a parallel **regular** `value-text` ramp
(value-xs 10 / value-sm 11 / value 12 / value-lg 13) — same sizes, the weight each context wants.
**All px values (text 10–13, icon 12–18) are first-pass PIN** — eye-gate on real Geist pending.

## D34 — ControlRow: a control-hosting row sized on the control ladder (2026-07-22)

**Decision.** A new `ControlRow` primitive pairs a left label with a right value/control
(any primitive — Button, SegmentedControl, Switch, Checkbox, Rating, Badge, or plain text),
on a row whose **height is its own `size`** on the *control* ladder (16/20/24/28), decoupled
from the read-only *intent* ladder. The row owns its height and its label's type role; the
hosted control keeps **its own** size — the row never resizes it (no cascade, per D33's
"size set explicitly per control"). Content is centered vertically; label left, value right.

**Why not the existing `Row`.** `Row` is intent-driven (§8: `control` 28 / `list` 24 /
`text` 16), and each intent **binds height to permitted type roles** so read-only metadata
rows can't put body type in a 16px row — load-bearing for the inspector, kept intact. A
control-hosting row needs the opposite: a free height decoupled from that binding, because
its value is an arbitrary control that carries its own size. Fusing "one size scales the row
and its control together" was rejected — it reverses the no-cascade rule and orphans the
20px rung (no row intent exists there). Separating "row height" from "control size" dissolves
both problems: the row is a sized, centering container; the control is sized independently.

**Amends** §8: names a **control-row** as a distinct row kind that sizes on the control
ladder (16/20/24/28), decoupled from the intent-bound read-only ladder — its height carries
no type-role binding (the hosted control brings its own size; the row's own label steps with
height via the medium control-text ramp). Read-only metadata rows remain intent-bound.

**Scope.** Primitive + design-library specimen only; no product surface wired yet (the repo's
"every primitive lands its matrix in `#/design-library`, product wiring later" method). Reuses
existing tokens (`size.control-*`, `size.row-inset`, the `control-text` roles) — no design-source
or compiler change. Deferred (no consumer): an optional label info-tooltip slot; `role="group"`
+ `aria-labelledby` to associate the visible label with the hosted control.

**Addendum (same day) — form-style arrangement + the ControlGroup container.** Two follow-on
calls, grounded in how property inspectors are built (Apple HIG grouped lists / SwiftUI Form
Section; the Figma / Unity `labelWidth` / Zoho form-level convention that the label-column width
is set at the form level, not per field):

- **Arrangement = form style, not inspector style.** ControlRow's default flips from "value
  right-aligned to the edge" (the read-only `Row` look) to **fixed label column + control fills
  left-aligned** — the dominant idiom for a row hosting editable controls, so a stack reads as
  aligned columns. The label column width rides a component-local CSS var `--control-row-label`
  (default `auto` standalone; capped at 60% so a long label can't crowd the control). Tunable
  either way by overriding the var.
- **ControlGroup** (new sibling primitive): a flush stack of ControlRows that owns the shared
  label-column width (`labelWidth` prop → `--control-row-label`, default 40%), so every row's
  label aligns. Rows stack flush — §8 puts space *inside* rows, not between them; separation is
  BETWEEN groups (the parent's gap), matching the grouped-list idiom. Still presentational.
  Deferred (no consumer): an opt-in §8-style hairline divider between rows; auto-fit-to-longest-
  label column (needs subgrid or measurement) — the explicit width covers today's need.

## D35 — One control-container material (2026-07-22)

**Decision.** Everything neutral that *holds* a value or a control — input wells, dropdown/select
triggers, neutral value chips, filled value-rows — uses the SAME recessed material, `surface.field`
(the canonical *control-container*). Components never source a neutral container from `surface.pill`,
`surface.hover`, or a per-hue `tint`. Two sanctioned exceptions, each because tone must stand in for
the §6-forbidden chrome shadow: the grouped-control **track** alone deepens to `surface.sunken` (so
the borderless in-well selection pill pops by tone), and **selection** leaves the container entirely
(`surface.pill` on panel / `surface.raised` out of a well, D32). Chrome reads as one material with two
named exceptions, not six near-materials.

**Why.** A material audit found six sources backing "container-ish" things — `surface.field` (0.955),
`surface.sunken` (0.91), `surface.pill` (0.945), `surface.hover` (0.97), per-hue `color.*.tint`, and
`cell.well`. On the light worlds these are within ~0.01 (visually one material, but named six ways and
free to drift); on the DARK worlds they split by *direction* — `pill`/`hover`/`raised` sit ABOVE panel
(rise) while `field`/`sunken` sit below (recess), so a value chip and a quiet button currently float up
while the field beside them sinks. Reference inspector panels (Hex/Attio) read cohesive because one
recessed chip repeats for every holder; this closes the gap toward that at the role level.

**Scope (honest).** This is a **naming + enforcement consolidation and a dark-world fix, not a re-tint**
— no theme L value changes. The eye-gate (a before/after probe, `library/control-container-probe.html`,
four themes) confirmed the surface change is near-invisible on the light worlds; the *visible* cohesion
win is a separate lever (filled value-rows, generous density, softer radius). Chosen path is **AFTER-B**
(keep `surface.sunken` as the sole deeper exception for the segmented track, over AFTER-A which retired
it — A's pill pop fell to 0.045 on paper, too weak for the borderless chip without a §6-forbidden shadow).

**Amends** §7 (the one-control-container clause + its two exceptions). Dated role notes on
`surface.field` (reframed input-well → canonical container) and `surface.sunken` (narrowed to the sole
exception).

**Deferred to the component-convergence round (code):** repointing Badge (neutral chip → container, off
per-hue tint), giving Row/ControlRow a filled value-row material, the dropdown/select trigger primitive
(absent today — the material's biggest future consumer), unifying the two selection substrates, and the
`surface.field` → `surface.container` rename (bundled there because it touches every consumer). The
visible payoff lands with that round; a usage-lint (containers must reach for the role) lands with it too.
