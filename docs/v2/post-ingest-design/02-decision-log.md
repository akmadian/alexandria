# Decision Log

Numbered, ADR-lite. Each entry: decision, rationale, revisit trigger where meaningful.
NFR references → `01-requirements-distilled.md`.

> **Implementation status (2026-07-07):** D3 (SQLite/FTS), D8 (data classification → writer-scoped
> repos), D9 (UUIDv7 + matrix), D11 (column promotion), D16 (schema side: sources split,
> keybindings table dropped) are realized through impl/01–02. D6 (unified registry) + D7
> (magic-byte classifier) are realized in impl/03 — note the package shipped as **`assettype`**,
> not `filetype` (Type = format category vs Kind = entity variant). **D12 (pipeline shape),
> D13 (DLQ = import_errors, no retry machinery), D17 (jobs: registry map + OnProgress), and
> D18 (ignore list) are realized in impl/04.** **D14 (watcher) is IN PROGRESS in impl/05:** 05.1
> (matrix extensions, `internal/importer`) DONE and kept; **05.2 + 05.3 (the `internal/watcher`
> service) are being REBUILT** — the first cut drifted from the Prime Directives (the watcher was
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

The load-bearing idea of the whole design. See `03-data-model.md`. Writer-scoped repository
interfaces make cross-class writes *uncompilable*. Immediate payoffs already banked: the
reimport-clobber bug class eliminated; `sources.status` split into `enabled` (judgment) +
`connectivity` (observation); `judgment_modified_at` added (XMP conflict resolution depends on it);
duplicates rebuild must upsert around judgment-bearing rows; `file_status` is observation-only
(relocate flow *triggers re-observation*, never writes status directly).

## D9 — Identity: catalog-assigned UUIDv7 + reconciliation matrix

Identity is a policy, not a fact — every file property is mutable, so the PK is minted at ingest
and thereafter *matched*. UUIDv7 (time-ordered → index locality; switch from v4). UUIDs are
load-bearing for bundle merge-back and multi-catalog (autoincrement guarantees collisions).
The matrix and its precedence: see `03-data-model.md` §Identity. Key refinement over pure
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
missing detection. Bouncer checks at SCAN's front door (not a separate stage). Full spec: `impl/04`.

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
identity by filesystem UUID never mount path; yanked drives detected by EIO probe. Full spec: `impl/05`.

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
exiftool daemon mode both directions; merge-into-existing writes; atomic temp+rename. Full spec: `impl/06`.

## D16 — Settings: three stores, routed by scope

localStorage = pre-paint needs (theme, locale) + lose-and-shrug window chrome ONLY. Catalog
settings KV = anything referencing catalog entities or painful to lose (keybinding overrides, tree
expansion, per-view prefs under `ui.*` keys, ignore list, import defaults). machine.json = facts
about this machine (worker pools/semaphores, memory limit, dependency path overrides, open-in apps,
telemetry consent) — a small JSON file read before any catalog opens. Contract grows a generic
`getUIState/setUIState` passthrough (UI-owned keys, opaque to Go) beside the small typed Settings
envelope (backend-consumed config). The `keybindings` DB table is DROPPED (one `ui.keybindings` KV
value; frontend owns the action vocabulary, defaults in code, DB stores overrides only).

## D17 — Jobs: registry map now, River later, catalog-as-queue for backfills

No workflow engines (Airflow-class = wrong altitude by 1000×; idempotent re-entry + durable state
at both ends makes durable workflow state unnecessary for ingest). V1 job envelope: jobID +
`map[jobID]cancelFunc` + OnProgress callback. When genuinely durable background jobs arrive
(thumb rebuild at scale, transcription), adopt **River with its `riversqlite` driver** (tested
against modernc.org/sqlite — our exact driver; experimental status: pin versions) behind the same
contract job envelope. Backfills (phash, etc.) need no queue at all: `WHERE phash IS NULL` IS the
queue.

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
only writing step, and it writes through the unmodified impl/04 pipeline. Full spec: `impl/09`.

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
resurrect a user-suppressed keyword. Full spec: `impl/10`.

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
