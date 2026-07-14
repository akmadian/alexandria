# Data Model

## 1. The classification system (D8) — read this first

Every column group in the schema belongs to exactly one class. The class determines who may write
it, what wins in conflict, how it's backed up, and how it's repaired.

| Class | Meaning | Writer | Conflict | Backup | Repair |
|---|---|---|---|---|---|
| **Judgment** | user-declared (k8s `spec`) | user-action service only | DB wins | paranoid | restore |
| **Observation** | world-state copied from FS (k8s `status`) | ingest/watcher only | world wins | nice-to-have | re-scan |
| **Derived** | computed from the other two | jobs only | n/a | never | recompute |
| **Sync-state** | reconcilers' own cursors | owning reconciler only | n/a | with judgments | reset+resync |

Enforcement is structural: repositories expose **writer-scoped interfaces** (`impl/02`) so an
observation writer has no method that touches a judgment column. When one table holds rows of
different classes, the class is an explicit column (`asset_tags.source`).

Class assignments by column group:

- `assets`: path/size/mtime/hash/EXIF/extended JSON/`file_status`/`caption`/`title` = observation ·
  rating/label/flag/note/is_deleted = judgment · `xmp_*`, `last_verified_at` = sync-state ·
  `thumbnail_at`, `aspect_ratio`, `phash`(P3) = derived
- `sources`: name/base_path/scan-config/`enabled` = judgment · fs UUID/serial/label/`connectivity` = observation ·
  `last_scanned_at` = sync-state
- `tags`, `collections`, `collection_assets` = judgment. `settings`/`keybindings` are no longer
  tables (impl/11 — plain JSON files instead), so they carry no column-class here.
- `asset_tags` = judgment or observation per-row (`source`: user|xmp|lr); `removed_at` (judgment
  tombstone — user deletion of an imported tag, respected over sync, D22/impl/10)
- ~~`asset_groups`~~ — DELETED (D24, 2026-07-10): drifted zero-consumer stub. The grouping
  design round (open question #7) re-derives the noun; the class split it sketched (derived
  `auto` rows freely rebuilt, `manual` never touched) carries forward as the design intent.
- `sidecar_files` = observation, EXCEPT `attached_asset_id` = derived (grouping engine writes it)
- `duplicates` = derived detections carrying judgment columns (`status`) → rebuild must UPSERT,
  never truncate
- `assets_fts`, thumbnails-on-disk = derived · smart-collection membership = derived-unmaterialized
  (computed at query time; the best kind)
- `import_sessions`/`import_errors` = system log (importer writes; losable)
- `enrichment_errors` = system log (the enrichment DLQ, D28 — the engine's writer goroutine
  writes it; losable: a lost row costs one wasted retry, nothing else). Keyed (asset_id, kind) —
  post-identity, which is why it is not an `import_errors` extension (that DLQ is path-keyed)

Special cases resolved by the classification:

- `updated_at` conflates writers → **`judgment_modified_at`** exists, bumped ONLY by the judgment
  writer. XMP conflict detection reads it; XMP sync (a distinct sync-writer class) applies inbound
  judgment *values* WITHOUT bumping it (loop prevention, D15).
- `file_status` is observation-only. The relocate flow is a user-*triggered re-observation*
  (`relocate(folder)` engine verb → match → observe), never a `setFileStatus`.
- `note` (judgment, ours) vs `caption` (observation, from IPTC/XMP `dc:description`) are distinct
  columns on purpose.

## 2. Keys

- PKs: **UUIDv7** as TEXT (time-ordered → b-tree locality; switch the helper from v4).
  UUIDs are load-bearing: bundle merge-back and multi-catalog require collision-free cross-catalog IDs.
- SQLite has no clustered sort key; "sort keys" = composite/partial secondary indexes shaped
  `(filter…, sort)` for hot grid queries.

## 3. Table roster

| Table | PK | FKs (ON DELETE) | Critical indexes / constraints |
|---|---|---|---|
| `sources` | uuid | — | fs_uuid |
| `assets` | uuid | source_id (RESTRICT) | `UNIQUE(source_id, relative_path) WHERE is_deleted=0` ← soft-delete trap fix · (partial_hash, size_bytes) · partial sort idx: captured_at, ingested_at, rating, filename, size_bytes |
| `sidecar_files` | uuid | source_id (CASCADE), attached_asset_id (SET NULL) | (source_id, dir, stem) · ext |
| `tags` | uuid | parent_id (CASCADE) | `UNIQUE(slug, IFNULL(parent_id,''))` ← NULL-parent trap fix · `path` (derived materialized ancestry, GLOB-prefix idx) · `color_mode` (D22/impl/10) |
| `asset_tags` | (asset_id, tag_id) | both CASCADE | tag_id reverse **partial** `WHERE removed_at IS NULL` (D22/impl/10) |
| `collections` | uuid | parent_id (CASCADE), cover (SET NULL) | parent_id |
| `collection_assets` | (collection_id, asset_id) | both CASCADE | (collection_id, position) · asset_id reverse |
| ~~`asset_groups` / `_members`~~ | — | — | DELETED (D24) — re-derived by the grouping round |
| `duplicates` | uuid | asset ids (CASCADE) | status · UNIQUE(original, duplicate) |
| `import_sessions` / `import_errors` | uuid | session_id (CASCADE) | started_at |
| `enrichment_errors` | (asset_id, kind) | asset_id (CASCADE) | kind · attempts gate the missing-artifact scan (D28) |
| `assets_fts` | — | external-content on `assets` | trigger-maintained |

**No `settings` or `keybindings` table** (impl/11, supersedes D16's storage mechanism). Both are
plain JSON files instead — `<catalog-dir>/settings.json` (catalog-scoped: `ui.*`, ignore list D18,
`xmpWriteBack`/`xmpConflictResolution`) and `<app-config-dir>/keybindings.json` (user-scoped
overrides, outside any catalog). The `settings` table that migration 0001 originally shipped gets
dropped in place when impl/11 lands (pre-1.0, edited-not-stacked). Dropped constraints: CHECKs on
`color_label` and `file_type` (guaranteed to change: custom labels P2, new file types P3; SQLite
CHECKs can't be altered without a 500k-row table rebuild — validation moves to `assettype.Classify`, the type registry — realized in impl/03).
CHECKs KEPT on stable enums: flag, file_status, sort_dir, sources.kind, asset_tags.source.

## 4. Column promotion rule (D11)

Column iff: (a) FR filter/sort/group consumes it, OR (b) FTS indexes it, OR (c) the engine consumes
it — AND cross-format normalizable. Else → `extended_metadata` JSON keyed by exiftool `Group:Tag`.
Current first-class set: dimensions, duration, captured_at, camera make/model, lens, focal length,
aperture, shutter, ISO, GPS lat/lon, color space, bit depth, size, mtime, hash, creator, copyright,
**title, caption** (promoted this session). `aspect_ratio` = generated VIRTUAL column from
width/height. Promotion later = ALTER + backfill from blob; never re-reads files (D24/C15 —
operators/column/compile all derive from one vocabulary row).

### 4a. Unrated = NULL, end to end (D24)

The catalog stores NULL for unrated; the wire carries `null`; the `empty` operator is the ONLY
query form for "unrated". **0 is not a rating** — `rating eq 0` matches nothing (no 0 is ever
stored). No layer coerces NULL→0.

### 4b. Path comparison: compare keys, open bytes (D24)

`domain.PathKey` (Unicode NFC, no case folding) is THE comparison form for path/filename
equality, matching, and dedup — macOS emits NFD, everything else NFC, and byte comparison mints
phantom identities. It is one-way: on-disk bytes stay the truth for file I/O and are never
replaced by the normalized form. **Status:** the helper + tests exist (D24); threading it
through the identity matrix, the scanner skip-map, and folder-scope path comparison is owned by
the source-management round (DEFERRED §8 — likely needs a stored normalized key column, since
normalizing only the query side of a LIKE breaks against NFD-stored rows).

## 5. FTS5 spec

> **Realized in impl/01 — chose STANDALONE FTS5, not external-content.** The plan below leaned
> external-content; implementation went standalone (the table stores its own text copy) because
> external-content's old-value bookkeeping for a non-content `tags` column was more code than
> trivial per-row triggers. Non-negotiables all met: asset-resident columns trigger-maintained
> (the UPDATE trigger scoped `AFTER UPDATE OF` the text columns so status/thumb churn doesn't
> reindex), `tags` app-maintained, rebuild via `sqlite.RebuildFTS`. FTS keys on `assets.rowid` →
> no plain in-place VACUUM (use VACUUM INTO; RebuildFTS is the escape hatch). Columns: filename,
> camera_make, camera_model, lens_model, title, caption, note, tags (+ `asset_id UNINDEXED`).

Original plan (superseded by the note above): external-content table over `assets` (no duplicated
text storage). The `tags` column CANNOT be trigger-maintained (it's a join through `asset_tags`) →
`SetAssetTags` rewrites that asset's FTS row (kept over dropping the column: FR P0 requires search
over tag names). Rebuild path: drop + re-populate from `assets` ⋈ `asset_tags`.

## 6. Identity & the reconciliation matrix (D9)

Identity is minted (UUIDv7) at ingest; afterwards every scan event is *matching*. Signals: path,
content fingerprint (xxhash of first 64KB + size), filename.

Precedence (order matters — this IS the policy). **Revised by D20 (2026-07-07):** the matrix
never auto-changes identity — it acts on a known *path* and otherwise detects-and-flags. The old
**Relink** rule (content+name vs a missing asset → adopt new path) and the **delete-side merge**
are **removed**; a file that reappears at a new path is a new asset + a pending review row.

1. **Unchanged**: path known + size exact + mtime within 2s tolerance → skip.
2. **Reimport**: path match → same asset; refresh observations ONLY (FilePatch — judgments
   untouched); regenerate thumbnail; restore online if it was missing and reappeared at its
   **original** path. Path identity wins for a known address.
3. **Duplicate**: content match vs another asset (present **or** missing, any source) → new
   identity + a `pending` duplicates row. A detection FLAG only, never a mutation of the matched
   asset — the review queue derives duplicate-vs-probable-move from live status (DEFERRED §5).
4. **New**: no match → mint.

Duplicate detection runs catalog-wide (cross-source content matches surface as duplicates). MATCH
stage also consults an **in-run hash map** (this import's minted hashes) or first-import duplicate
pairs are invisible.

Accepted failure modes (named, documented, all leave visible residue): a move → missing original +
new asset + a pending review pair the user confirms (D20 — no auto-relink, so no cross-attach or
swap hazard); partial-hash dedup ambiguity (→ full-hash verify before UI claims "identical");
soft-deleted tombstones excluded from matching (removal was a judgment; revisit-cheap).

Sidecars deliberately get NONE of this: matched purely by (source, dir, stem) filesystem identity.
Assets carry identity; sidecars follow.
