# impl/09 — Lightroom Classic Catalog Migration

**Status: design only, not started.** No blockers within this doc set beyond impl/04 (ingest
pipeline, DONE) and the tag repository (find-or-create + hierarchy, still unbuilt — same
dependency impl/06's keyword sync is blocked on).
**Scope:** new `internal/lrcimport` (engine) + `cmd/lrcimport` (standalone CLI) + a Wails-bound
wizard (frontend, later). **References:** D1 (seam), D8 (classification), D12 (pipeline), D15
(XMP field map), D21, `01-requirements-distilled.md` (NFR-6 zero-network).

## Why this exists

Photographers with years invested in a Lightroom Classic catalog have no real way out: the
catalog format is undocumented, the Develop history is proprietary, and every "migration tool"
in the market either re-implements Adobe's raw processing (never matches) or drops everything
Lightroom-specific and calls it a fresh start. This feature is Alexandria's answer to that specific
door: **not a general importer, a purpose-built exit ramp.** For a target user who has been
trained by a decade of Adobe subscription changes to distrust exactly this kind of tool, the
migration path *is* the product's first impression — get it wrong once and there is no second
pitch.

Positioning: Alexandria takes over catalog management (search, rate, tag, collect); Photoshop,
Pixelmator Pro, or whatever the user picks takes over pixel editing. This is a deliberate
narrowing — Lightroom's Develop module has no successor in this design, and that is what makes the
rest of the problem tractable (see D21).

## D21 — engine-first, LrC does its own lossy translation, preflight is pure read

**Decision.** Never hand-parse Lightroom's Develop settings (the `crs:` payload, Lua-serialized,
undocumented, drifts across LrC releases). Instead the migration requires a **documented, one-time
prep pass performed inside Lightroom itself**, which hands the migration engine only standard,
stable formats:

1. **`Convert Photo to DNG`** on every raw master (catalog-wide, two clicks: Select All → Library
   menu). This is non-destructive — the original sensor data is preserved (or losslessly
   compressed) inside the DNG container — and it bakes the current Develop history into the DNG's
   embedded XMP as standard `crs:` fields. Photoshop's Camera Raw (and Pixelmator Pro, with less
   fidelity) opens the result non-destructively editable, picking up exactly where Lightroom left
   off.
2. **`Metadata → Save Metadata to Files`** catalog-wide, for everything a DNG conversion doesn't
   already cover (JPEGs, anything the user chooses not to convert). Writes ratings, labels,
   keywords, captions, GPS into embedded/sidecar XMP using the same fields impl/06 already reads.

The migration engine's job is therefore **metadata and structure, not pixel-editing decode**. This
is the same "never hand-parse the RDF/XML, let the authoritative tool speak a standard dialect"
move D15 already made for exiftool — applied one layer up, to Lightroom itself as the authority on
its own Develop settings.

**What the engine reads directly from `.lrcat`** (opened via a read-only/immutable SQLite URI,
never written to) is only what the XMP prep pass structurally cannot carry:

- Collections + membership (`AgLibraryCollection` / `AgLibraryCollectionImage`)
- Virtual-copy / stack relationships (`Adobe_images.masterImage`, `AgLibraryFile` stack tables) —
  the one relationship that stops existing once files hit disk as independent DNGs
- LrC catalog schema version (`Adobe_variablesTable` or equivalent), to gate unsupported versions
  rather than silently misreading them

**Preflight is pure read, with zero durable writes anywhere** — not to the `.lrcat` (read-only
connection), not to Alexandria's catalog DB, not to a scratch file. Everything it produces (parsed
catalog summary, identity-matrix dry run against the live catalog, the report) lives in memory for
the run and is cheap to re-derive. This means: safe to run against a live LrC catalog copy
repeatedly, free to re-run after every prep fix (a missing file found, a straggler converted to
DNG), and nothing is lost by throwing a report away. `Commit` is the only step that writes, and it
writes through the existing ingest pipeline (impl/04) — no new write path.

**Rejected alternatives.**
- *Reimplementing the Lua Develop-settings decoder in Go* — undocumented format, breaks silently
  across LrC versions, and buys nothing: there's no destination for non-destructive Develop history
  once editing moves to Photoshop/Pixelmator. Baked pixels or ACR-compatible XMP is all either tool
  can consume.
- *A cloud/account step anywhere in the flow* — violates NFR-6, and doubly undermines trust for a
  feature whose entire pitch is "we don't touch your library."
- *In-app flattened-TIFF export as the primary edit-handoff format* — DNG dominates it: same
  non-destructive editability, smaller than a flattened master, and it's a menu Lightroom already
  has.

**Revisit trigger:** if the catalog-wide LrC prep pass proves impractical for very large libraries
(days-long Convert-to-DNG runs, disk-space doubling), consider a Lua-SDK LrC plugin that automates
prep incrementally instead of asking for one big pass — but this only reorders *when* Lightroom
does the translation, it does not revisit the core call of never decoding `crs:` ourselves.

## Delivery: engine first, wizard wraps it (per D1's seam discipline)

Two phases, same shape as every other engine feature in this project (`internal/importer` →
`cmd/dev` → eventually a UI):

**Phase 1 — `internal/lrcimport`, exercised by a standalone `cmd/lrcimport` CLI.** Three calls,
each a real Go function with a serializable return type — not stdout scraping, so the wizard can
call the identical function later through Wails bindings:

- `ParseCatalog(path string) (CatalogSummary, error)` — opens the `.lrcat` read-only, returns photo
  count, collection tree, virtual-copy/stack graph, per-photo DNG-conversion / XMP-freshness status,
  schema version, and any structural warnings (missing files on disk, untested version).
- `Preflight(summary CatalogSummary, source domain.Source) (Report, error)` — runs the existing
  identity matrix (impl/04) in dry-run mode against the live Alexandria catalog: what's new,
  what's a reimport, what's a probable duplicate; cross-references `CatalogSummary` for what's
  lossy (unconverted raws, smart collections, anything on an untested schema version). Writes
  nothing. This is the artifact a user reviews before committing to anything.
- `Commit(summary CatalogSummary, source domain.Source) (Manifest, error)` — runs
  `Importer.Run` (impl/04, unmodified) over the prepped file tree, then a post-import pass that
  creates `collections`/`collection_assets` and `asset_groups` (virtual-copy siblings, stacks;
  `origin='manual'`, per D8's classification so auto-grouping never touches these rows) by
  matching `CatalogSummary` paths to the asset IDs the pipeline just minted. Returns a `Manifest` —
  old LrC `id_local`/`id_global` → new asset ID, one row per photo — as a savable JSON artifact,
  not new schema (see "Traceability," below).

CLI is a thin wrapper: `parse`/`preflight` print the report and exit; `commit` requires an explicit
flag, matching the dry-run-by-default convention `cmd/dev` already uses. This phase is real enough
to dogfood on an actual library and is itself useful to the technically-comfortable slice of the
target audience before any UI exists.

**Phase 2 — Wails-bound wizard**, calling the same three functions through generated bindings
(IPC, per the UI-runtime leaning toward Wails — see `04-open-questions.md` #6, still formally
open). Steps: pick catalog file → LrC-side prep checklist (the two menu actions above, with
inline "have you done this?" hints derived from `CatalogSummary`'s per-photo conversion status) →
run `Preflight`, render the report with lossy items called out explicitly → confirm → `Commit`,
subscribed to the same `JobProgress` event stream (D17) the regular importer already emits, no
second progress mechanism → final summary reusing the existing duplicates/import-errors review
surfaces rather than new UI for this one feature.

Nothing in phase 2 contains migration logic — it is purely presentation over phase 1's contract.
This is the same seam discipline D1 already committed the whole engine to.

## What migrates, and how

| LrC source | Alexandria destination | Path | Notes |
|---|---|---|---|
| Rating, color label, keywords, caption, title, GPS | `assets.*` (observation), `asset_tags` | Prep-pass XMP → normal ingest EXTRACT (impl/04) → impl/06's field map, unmodified | No LrC-specific code; this is impl/06 reused as-is once its tag-repository dependency lands |
| Develop settings (final state only) | the DNG's embedded XMP `crs:` | LrC's own `Convert to DNG` | Not read or interpreted by Alexandria at all — it rides along inside the file for Photoshop/ACR |
| Flags (pick/reject) | `assets.flag` | `alexandria:Flag` custom namespace, written by the same prep pass if the user also runs impl/06's future write-back, otherwise dropped | LrC has no flag in standard XMP (documented asymmetry, same as impl/06's field map) |
| Collections + membership | `collections` / `collection_assets` | Read from `.lrcat` in `ParseCatalog`, materialized in `Commit`'s post-import pass | Smart collections migrate as a **static snapshot of current membership only** — the rule itself is not portable, and is never silently dropped without saying so in the report |
| Virtual copies / stacks | `asset_groups` (`origin='manual'`) | Read from `.lrcat`, linked in `Commit`'s post-import pass | Each virtual copy must exist as an independent file before import (LrC's DNG-per-virtual-copy behavior needs empirical verification during prep-guide authoring — flag, don't assume) |
| Rejected/deleted-in-LrC photos | *(excluded)* | User excludes them from the prep pass / export set | Simpler and safer than routing through a judgment writer during import; ingest structurally cannot write `is_deleted` (D8) |
| Face/person keyword tags (Lr6+) | plain hierarchical `asset_tags` | Same XMP `dc:subject`/`lr:hierarchicalSubject` path as any keyword | Face-region geometry is dropped — no consumer for it (no face UI) |

## Trust mechanics (load-bearing, not polish)

This feature's adoption *is* the trust question, so these are requirements, not nice-to-haves:

- **Provably read-only.** `.lrcat` opened with a read-only/immutable SQLite URI at the driver
  level — not "we don't call UPDATE" as a promise, a connection that structurally cannot write.
  Hash the file before and after a run and show the user the hashes match.
- **Never the original file.** The wizard's first screen instructs — and can perform — copying the
  catalog before anything touches it. `ParseCatalog`/`Preflight` never require write access to
  anywhere on disk.
- **Nothing is one-way.** Alexandria importing a copy doesn't touch Lightroom; message this
  explicitly ("keep using Lightroom in parallel for as long as you want, re-run this anytime").
  Re-running is safe by construction — `Commit` funnels through the same idempotent identity matrix
  every other import uses (impl/04, D9).
- **Alexandria-side reversibility.** The import lands as one `source`; removing that source and its
  assets is one traceable action (existing delete path), so the destination side isn't a one-way
  door either.
- **Traceability as a receipt.** The `Manifest` (old LrC ID → new asset ID → original path) is a
  savable artifact specifically so a skeptical user can spot-check N random photos against their
  old catalog and verify the claims in the report — falsifiable, not just asserted.
- **A demo catalog.** Ship a small sample `.lrcat` + fixture photos so the entire wizard flow
  (prep checklist → preflight → commit → report) can be exercised before a user risks their real
  library. Highest-leverage single trust-building item for a first-time user.
- **The field map is public.** What migrates cleanly, what flattens (smart collections), what's
  dropped (Develop history as editable state, face geometry) is documented up front — see the
  table above — not discovered after the fact.

## Open questions

1. **Virtual-copy → DNG behavior needs empirical verification.** Does `Convert to DNG` on a virtual
   copy produce an independent file, or does LrC refuse / silently operate on the master? Test
   before writing the user-facing prep guide.
2. **LrC schema version coverage.** Which `.lrcat` versions get full support vs. a "detected but
   untested, proceed with caution" warning? Needs a version matrix, likely built empirically
   against a few LrC release generations (mirrors the version-drift risk noted for the format
   itself — no Adobe documentation exists to consult instead).
3. **Where the `Manifest` artifact lives.** A file the user saves themselves (simplest, matches
   "derived, not durable" — nothing new in the catalog schema), vs. something Alexandria offers to
   keep alongside the catalog directory for later reference. Lean file-only until a real user asks
   for more.
4. **JPEG Develop-settings fidelity.** LrC can write Develop XMP for non-raw files too (no DNG
   conversion path exists for JPEG); ACR reads a JPEG's XMP `crs:` settings, but this is unverified
   in practice and less guaranteed than the DNG path. Needs the same empirical check as #1.
5. **Transport for phase 2.** Confirmed leaning Wails/IPC bindings per this conversation, but the
   formal UI-runtime decision (`04-open-questions.md` #6) is still open — this feature's wizard is
   blocked on that resolving, not the other way around.
