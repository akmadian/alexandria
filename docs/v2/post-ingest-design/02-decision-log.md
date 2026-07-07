# Decision Log

Numbered, ADR-lite. Each entry: decision, rationale, revisit trigger where meaningful.
NFR references → `01-requirements-distilled.md`.

> **Implementation status (2026-07-07):** D3 (SQLite/FTS), D8 (data classification → writer-scoped
> repos), D9 (UUIDv7 + matrix), D11 (column promotion), D16 (schema side: sources split,
> keybindings table dropped) are realized through impl/01–02. D6 (unified registry) + D7
> (magic-byte classifier) are realized in impl/03 — note the package shipped as **`assettype`**,
> not `filetype` (Type = format category vs Kind = entity variant). **D12 (pipeline shape),
> D13 (DLQ = import_errors, no retry machinery), D17 (jobs: registry map + OnProgress), and
> D18 (ignore list) are realized in impl/04.** D14 (watcher) is IN PROGRESS (impl/05, started
> 2026-07-07): **05.1 (matrix extensions) + 05.2 (watcher service) DONE; 05.3 (poll-timer
> connectivity) remaining.** Reconciled build plan at the top of `impl/05` — the per-OS event
> adapters collapsed to one dep (`rjeczalik/notify`), the volume monitor to a poll timer. Building
> 05.2 surfaced the broader **import/tracking model** (one-shot vs. watched sources, `sync_mode`,
> loose-files vs. directories, cross-source dup handling) — captured in `impl/DEFERRED.md`, deferred
> to the source-management milestone. Note: `Reconcile` is **no longer** slated for removal — its
> per-file logic is the loose-file fidelity primitive (see `DEFERRED.md §1`). Everything else is
> design-only so far.

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

## D10 — Signature ladder with an authority rule

mtime+size (staleness) → partial hash 64KB+size (exact-ish identity) → full hash (verification) →
phash/P3 (perceptual). **Exact tiers may drive automatic actions; perceptual tiers may only
suggest.** Partial hash may *propose* duplicates; the P2 review UI must *confirm* with full hash
before claiming "identical". phash never feeds the identity matrix — similar ≠ same, and burst
neighbors are exactly where judgments must not cross-contaminate.

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
