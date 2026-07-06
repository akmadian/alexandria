# Database Schema

Alexandria uses SQLite in WAL mode. The schema is managed by a versioned migration system (see [Schema Migrations](12-migrations.md)).

All timestamps are stored as ISO 8601 strings (e.g. `2024-07-01T10:30:00Z`). SQLite has no native datetime type; ISO 8601 strings sort correctly lexicographically and are human-readable when inspecting the database directly.

All primary keys are UUIDs (TEXT). Integer auto-increment PKs are avoided because UUIDs are stable across catalog merges, backups, and restores, and do not collide if two catalogs are ever combined.

---

## sources

A source is a watched root. Every asset's physical location belongs to a source.

```sql
CREATE TABLE sources (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    kind                TEXT NOT NULL CHECK(kind IN ('local', 'external_drive', 'smb', 'nfs')),
    base_path           TEXT NOT NULL,

    -- Drive identity (external_drive only).
    -- filesystem_uuid is the primary identifier — stable across plugging/unplugging.
    -- Changes on reformat. Sourced from: diskutil on macOS, blkid on Linux, volume serial on Windows.
    filesystem_uuid     TEXT,
    -- disk_serial is the physical drive serial from firmware.
    -- Survives reformats. Used as fallback if filesystem_uuid changes (reformat detected).
    -- Not always accessible (enclosures may not expose it). Never rely on as sole identifier.
    disk_serial         TEXT,
    -- volume_label is the human-readable name the user gave the drive.
    -- Stored for display only. Never used for identity matching.
    volume_label        TEXT,

    -- Network share identity (smb/nfs only).
    -- Identified by host + share_name. User is responsible for keeping these stable.
    -- This is an intentional simplicity tradeoff — see architecture docs.
    host                TEXT,
    share_name          TEXT,

    -- Scan behaviour.
    -- poll_interval_secs: NULL means use filesystem events (local/external_drive).
    -- Set to an integer for network sources where filesystem events are unreliable.
    poll_interval_secs  INTEGER,
    scan_recursively    INTEGER NOT NULL DEFAULT 1,

    -- State.
    status              TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'offline', 'removed')),
    last_scanned_at     TEXT,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX idx_sources_filesystem_uuid ON sources(filesystem_uuid);
CREATE INDEX idx_sources_status ON sources(status);
```

**On drive identity matching logic:** When a volume mounts, Alexandria reads its filesystem UUID and looks it up in this table. If found, the source is reconnected (base_path updated if the mount point changed). If not found by UUID, disk_serial is tried as a fallback — this handles the case where a drive was reformatted but is physically the same drive. If neither matches, the drive is treated as unknown and not auto-connected.

---

## assets

The canonical record for a file. One row per logical asset. Heavily denormalized for query performance: any field that will be filtered, sorted, or displayed in the grid is a dedicated column rather than buried in a JSON blob.

Each asset has exactly **one** physical location, stored directly on the asset row (`source_id` + `relative_path`). This matches the model used by Lightroom, Capture One, and digiKam. The same file content appearing at two paths is two assets plus a `duplicates` log entry — not one asset with two locations.

```sql
CREATE TABLE assets (
    id                  TEXT PRIMARY KEY,

    -- Physical location. Exactly one per asset.
    source_id           TEXT NOT NULL REFERENCES sources(id),
    -- relative_path is relative to source.base_path, so a source remounting
    -- at a different mount point only requires updating source.base_path.
    relative_path       TEXT NOT NULL,
    -- file_status reflects the last known state of the file on disk.
    -- 'online': verified, file exists and matches
    -- 'offline': source is offline, file not checked
    -- 'missing': source is online but file not found at this path
    --   (deleted or moved — user resolves via the relocate flow)
    file_status         TEXT NOT NULL DEFAULT 'online' CHECK(file_status IN ('online', 'offline', 'missing')),
    last_verified_at    TEXT,

    -- File identity.
    filename            TEXT NOT NULL,
    extension           TEXT NOT NULL,    -- lowercase, no leading dot: 'jpg', 'psd', 'mp4'
    mime_type           TEXT NOT NULL,
    -- file_type is a coarse category for fast filtering.
    -- Stored redundantly alongside mime_type because "show all images" is a constant query.
    file_type           TEXT NOT NULL CHECK(file_type IN ('image', 'video', 'raw', 'vector', 'document', 'audio')),

    -- File stats as observed at the last verification of the file on disk.
    size_bytes          INTEGER NOT NULL,
    mtime               TEXT NOT NULL,    -- ISO 8601, from filesystem
    -- partial_hash: xxHash of first 64KB of file contents concatenated with size_bytes.
    -- Used for: duplicate detection, integrity checks, move detection.
    -- Not a cryptographic hash — designed for speed, not collision resistance.
    -- Computed using cespare/xxhash. ~10-20x faster than MD5.
    -- "First 64KB" covers enough of any file to distinguish it reliably in a creative library.
    -- Full-file hashing is not done at ingest due to cost on large files over slow NAS connections.
    -- If full-file verification is ever needed, it can be done as a separate integrity pass.
    partial_hash        TEXT,

    -- Visual dimensions. NULL for non-visual file types.
    width               INTEGER,
    height              INTEGER,
    -- duration_secs: NULL for non-temporal files (images, documents).
    duration_secs       REAL,

    -- Colour profile information.
    color_space         TEXT,             -- 'sRGB', 'AdobeRGB', 'P3', 'CMYK', etc.
    bit_depth           INTEGER,

    -- Camera / capture metadata. Denormalized from EXIF.
    -- All nullable — not all files have EXIF, and not all EXIF has all fields.
    captured_at         TEXT,             -- ISO 8601, from EXIF DateTimeOriginal
    camera_make         TEXT,             -- e.g. 'Sony', 'Canon'
    camera_model        TEXT,             -- e.g. 'ILCE-7M4'
    lens_model          TEXT,
    focal_length_mm     REAL,
    aperture            REAL,             -- f-number, e.g. 2.8
    shutter_speed       TEXT,             -- stored as string: '1/250', '2', etc.
    iso                 INTEGER,
    gps_lat             REAL,
    gps_lon             REAL,

    -- Extended / format-specific metadata that is not commonly queried.
    -- Stored as JSON. Do not index into this field.
    -- Examples: video codec details, audio channel count, PDF page count,
    -- PSD layer count, full EXIF dump for less common fields.
    extended_metadata   TEXT,

    -- User organisation.
    rating              INTEGER CHECK(rating IS NULL OR (rating >= 0 AND rating <= 5)),
    color_label         TEXT CHECK(color_label IN ('red', 'orange', 'yellow', 'green', 'blue', 'purple') OR color_label IS NULL),
    flag                TEXT CHECK(flag IN ('pick', 'reject') OR flag IS NULL),
    -- Free-text note attached by the user. Included in full-text search.
    -- Synced to/from XMP dc:description (Lightroom's "Caption" field) when XMP sync is enabled.
    note                TEXT,

    -- XMP sync tracking.
    -- xmp_last_read_at: when Alexandria last read an XMP sidecar for this asset.
    -- xmp_last_written_at: when Alexandria last wrote an XMP sidecar for this asset.
    -- xmp_hash: hash of XMP content at last sync. Used to detect external changes
    --   (Lightroom edited the sidecar). If current XMP hash differs from stored hash,
    --   a conflict resolution check is triggered.
    xmp_last_read_at    TEXT,
    xmp_last_written_at TEXT,
    xmp_hash            TEXT,

    -- Thumbnail.
    -- thumbnail_path is relative to the Alexandria app data directory.
    -- Stored separately from source files so thumbnails survive source going offline.
    -- The thumbnail cache is rebuildable and is not a critical backup target.
    thumbnail_path      TEXT,
    thumbnail_at        TEXT,             -- when thumbnail was last generated

    -- Catalog lifecycle.
    is_deleted          INTEGER NOT NULL DEFAULT 0,   -- soft delete flag
    deleted_at          TEXT,
    ingested_at         TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

-- Grid view: non-deleted assets sorted by capture date (most common default sort)
CREATE INDEX idx_assets_captured_at     ON assets(captured_at) WHERE is_deleted = 0;
-- File type filter
CREATE INDEX idx_assets_file_type       ON assets(file_type) WHERE is_deleted = 0;
-- Rating filter
CREATE INDEX idx_assets_rating          ON assets(rating) WHERE is_deleted = 0;
-- Color label filter
CREATE INDEX idx_assets_color_label     ON assets(color_label) WHERE is_deleted = 0;
-- Flag filter
CREATE INDEX idx_assets_flag            ON assets(flag) WHERE is_deleted = 0;
-- Dedup and integrity checks
CREATE INDEX idx_assets_partial_hash    ON assets(partial_hash);
-- ingested_at for "recently added" views
CREATE INDEX idx_assets_ingested_at     ON assets(ingested_at) WHERE is_deleted = 0;
-- Scanner skip check and filesystem tree view: prefix queries on relative_path within a source.
-- Invariant: relative_path uses '/' separators on every platform (normalized at ingest) —
-- the derived folder tree (docs/frontend-architecture.md §3) groups byte-wise on this column.
-- If folder-scope queries or tree builds ever show up in profiles, the upgrade is an
-- ingest-written dir_path column + (source_id, dir_path) index; folders stay derived, never stored.
CREATE UNIQUE INDEX idx_assets_source_path ON assets(source_id, relative_path);
-- Per-source status queries (mark offline, find missing).
CREATE INDEX idx_assets_source_status   ON assets(source_id, file_status);

-- Full-text search. A standalone FTS5 table (NOT external-content) maintained
-- explicitly by application code. Since the catalog writer and tag/note commands
-- are the only write paths (single-writer design), keeping this in sync in app
-- code is simple and avoids the trigger machinery external-content tables need.
-- The 'tags' column holds a space-joined list of the asset's tag names,
-- rewritten whenever the asset's tags change. This is what makes typing a tag
-- name into the search box work.
CREATE VIRTUAL TABLE assets_fts USING fts5(
    asset_id UNINDEXED,
    filename,
    camera_make,
    camera_model,
    lens_model,
    tags,
    note
);
```

**On denormalization:** The decision to store `file_type` alongside `mime_type`, and to store EXIF fields as dedicated columns rather than only in `extended_metadata`, is a deliberate performance choice. Filtering by file type and sorting by capture date are the two most common grid view operations. Making these indexed columns rather than parsed JSON fields keeps query times fast at 500k assets.

**On soft delete:** Setting `is_deleted = 1` removes the asset from all normal views. The asset record and all its metadata are preserved. Permanent removal requires a separate explicit operation. This protects against accidental deletion and provides a recoverable trash.

---

## duplicates

Log of duplicate detections from import. When the dedup checker finds a new file whose content (partial hash + size) matches an existing asset that is still present on disk, the new file is still ingested as its own asset, and a row is written here linking the pair. Resolution UI (keep one, keep both, group) is deferred — this table exists from day one so detections are never lost between sessions.

```sql
CREATE TABLE duplicates (
    id                  TEXT PRIMARY KEY,
    -- The pre-existing asset that was matched.
    original_asset_id   TEXT NOT NULL REFERENCES assets(id),
    -- The newly ingested asset that duplicates it.
    duplicate_asset_id  TEXT NOT NULL REFERENCES assets(id),
    partial_hash        TEXT NOT NULL,
    detected_at         TEXT NOT NULL,
    -- 'pending' until the user resolves it (resolution UI is deferred).
    status              TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'resolved', 'ignored')),
    resolved_at         TEXT,

    UNIQUE(original_asset_id, duplicate_asset_id)
);

CREATE INDEX idx_duplicates_status ON duplicates(status);
```

**Move vs duplicate disambiguation:** a hash match against an existing asset whose file is `missing` (or whose source is offline and whose old path no longer resolves) is a **move**, not a duplicate — the existing asset is relinked to the new path and no duplicate row is written. Only a hash match against an asset that is still present at its recorded path is logged here. See the ingest pipeline doc.

---

## tags

Hierarchical tag tree. Tags can be nested to any depth via the `parent_id` self-reference.

```sql
CREATE TABLE tags (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    -- slug is a normalised, URL-safe version of the name used for programmatic reference.
    -- e.g. "Portrait Headshot" → "portrait-headshot"
    slug        TEXT NOT NULL,
    parent_id   TEXT REFERENCES tags(id),
    -- color is an optional UI hint for displaying this tag with a colour chip.
    color       TEXT,
    created_at  TEXT NOT NULL,

    -- Same slug is allowed under different parents (e.g. "raw" under "photography" and "file-type")
    UNIQUE(slug, parent_id)
);

CREATE INDEX idx_tags_parent ON tags(parent_id);
```

---

## asset_tags

Joins assets to tags. The `source` column tracks the provenance of each tag assignment — whether it was set by the user in Alexandria, synced in from an XMP sidecar, or synced from Lightroom. This is used during XMP conflict resolution and for knowing what to write back to XMP vs what to keep catalog-only.

```sql
CREATE TABLE asset_tags (
    asset_id    TEXT NOT NULL REFERENCES assets(id),
    tag_id      TEXT NOT NULL REFERENCES tags(id),
    source      TEXT NOT NULL DEFAULT 'user' CHECK(source IN ('user', 'xmp', 'lr')),
    created_at  TEXT NOT NULL,

    PRIMARY KEY (asset_id, tag_id)
);

CREATE INDEX idx_asset_tags_asset ON asset_tags(asset_id);
CREATE INDEX idx_asset_tags_tag   ON asset_tags(tag_id);
```

---

## collections

Manual and smart collections. Nestable. Smart collections store their query as JSON; membership is computed dynamically when the collection is opened (not pre-computed and cached). This is intentional — a pre-computed membership table would require synchronisation whenever assets change, adding complexity. SQLite queries with proper indexes are fast enough to evaluate smart collection queries on demand.

```sql
CREATE TABLE collections (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    -- parent_id enables nested collections (folders of collections).
    parent_id       TEXT REFERENCES collections(id),
    kind            TEXT NOT NULL DEFAULT 'manual' CHECK(kind IN ('manual', 'smart')),
    -- query is a JSON representation of the filter criteria for smart collections.
    -- NULL for manual collections.
    -- The query builder translates this JSON into a SQL SELECT at runtime.
    -- Schema of the JSON is defined in the query builder implementation.
    query           TEXT,
    -- cover_asset_id: the asset shown as the collection's cover thumbnail in the sidebar.
    cover_asset_id  TEXT REFERENCES assets(id),
    sort_field      TEXT,
    sort_dir        TEXT NOT NULL DEFAULT 'asc' CHECK(sort_dir IN ('asc', 'desc')),
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX idx_collections_parent ON collections(parent_id);
```

---

## collection_assets

Membership table for manual collections. Not used by smart collections (their membership is computed from `collections.query`).

```sql
CREATE TABLE collection_assets (
    collection_id   TEXT NOT NULL REFERENCES collections(id),
    asset_id        TEXT NOT NULL REFERENCES assets(id),
    -- position enables manual ordering of assets within a collection.
    -- NULL means unordered (use collection's sort_field/sort_dir instead).
    position        INTEGER,
    added_at        TEXT NOT NULL,

    PRIMARY KEY (collection_id, asset_id)
);

CREATE INDEX idx_collection_assets_collection ON collection_assets(collection_id, position);
CREATE INDEX idx_collection_assets_asset      ON collection_assets(asset_id);
```

---

## asset_groups

A group is a container for related assets (e.g. a RAW file and its exported JPEG, a PSD and its exported PNG). In the grid view, a group renders as a single card showing the cover asset. The group kind is inferred from member roles rather than stored explicitly — avoiding a proliferating list of kind values as new grouping scenarios are discovered.

This is a P1 feature — the schema is present from day one but the product functionality is deferred. See [Deferred Features](14-deferred.md).

```sql
CREATE TABLE asset_groups (
    id              TEXT PRIMARY KEY,
    -- cover_asset_id determines which member asset is shown in the grid card.
    -- Typically the JPEG for a RAW+JPEG pair (faster to render than RAW thumbnail).
    cover_asset_id  TEXT REFERENCES assets(id),
    created_at      TEXT NOT NULL
);
```

---

## asset_group_members

```sql
CREATE TABLE asset_group_members (
    group_id    TEXT NOT NULL REFERENCES asset_groups(id),
    asset_id    TEXT NOT NULL REFERENCES assets(id),
    -- role describes this asset's relationship within the group.
    -- An asset's role is relative to a specific group, not intrinsic to the asset itself.
    -- A hero.psd could be the 'source' in one group and just a 'member' in another.
    -- This is why role lives on the membership record, not on the asset.
    role        TEXT NOT NULL CHECK(role IN ('raw', 'jpeg_sidecar', 'source', 'export', 'member')),

    PRIMARY KEY (group_id, asset_id)
);

CREATE INDEX idx_group_members_group ON asset_group_members(group_id);
CREATE INDEX idx_group_members_asset ON asset_group_members(asset_id);
```

---

## settings

Key-value store for user preferences **that belong to the catalog** — organisational and behavioural settings that should travel with the catalog if it is restored on another machine. Values are JSON-encoded so complex settings (arrays, objects) don't require schema changes. The entire settings object is typically loaded at startup and held in memory.

**Machine-local settings live in a local config file, not here.** Worker pool sizes, memory limit, and log verbosity are properties of the machine, not the catalog — a catalog restored onto a laptop should not import a workstation's worker counts. These live in a small JSON file next to the catalog (`{catalog_dir}/machine.json`) with the same defaults listed below.

```sql
CREATE TABLE settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,    -- JSON-encoded value
    updated_at  TEXT NOT NULL
);
```

**Catalog settings keys (seeded at first launch with defaults):**

| Key | Type | Default | Description |
|---|---|---|---|
| `xmp_conflict_resolution` | string | `"xmp_wins"` | `"xmp_wins"` or `"catalog_wins"` |
| `thumbnail_quality` | integer | `85` | JPEG quality for thumbnails (1–100) |
| `import_batch_size` | integer | `50` | Catalog write batch size during import |
| `catalog_backup_count` | integer | `10` | Rolling backup retention count |
| `undo_stack_size` | integer | `50` | Maximum undo history depth |
| `update_check_enabled` | boolean | `true` | Whether to check GitHub for updates on launch |
| `default_sort_field` | string | `"captured_at"` | Default grid sort field |
| `default_sort_dir` | string | `"desc"` | Default grid sort direction |

**Machine-local settings (in `machine.json`, not in the catalog):**

| Key | Type | Default | Description |
|---|---|---|---|
| `hash_worker_count` | integer | `4` | Import pipeline hash workers |
| `extract_worker_count` | integer | `2` | Import pipeline metadata extraction workers |
| `thumb_worker_count` | integer | `2` | Import pipeline thumbnail workers |
| `memory_limit_mb` | integer | `512` | Go runtime memory limit (GOMEMLIMIT) |

(The `duplicate_handling` setting is removed: duplicates are always ingested and logged to the `duplicates` table; resolution is deferred.)

---

## keybindings

User keybinding **overrides only**. Default bindings live in code (`internal/keybindings/defaults.go`), not in the database. The effective binding set is defaults merged with overrides (override wins per action+context). This makes "reset to defaults" a simple `DELETE FROM keybindings`, means new actions added in app updates need no migration to seed bindings for existing users, and removes the ambiguity of a "modified default" row.

```sql
CREATE TABLE keybindings (
    -- action is a stable string constant identifying what this binding does.
    -- e.g. 'rate_1', 'flag_pick', 'nav_next', 'open_in_app'
    -- Action constants are defined in the Go domain package.
    action      TEXT NOT NULL,
    -- context scopes the binding. 'global' bindings apply everywhere.
    -- More specific context bindings take priority over global.
    context     TEXT NOT NULL CHECK(context IN ('global', 'grid', 'detail', 'import')),
    -- key_combo uses platform-neutral notation.
    -- 'primary' maps to Cmd on macOS, Ctrl on Windows/Linux.
    -- Examples: 'primary+z', 'shift+primary+z', '1', 'space', 'arrowleft'
    -- An empty string means "unbound" (user removed a default binding).
    key_combo   TEXT NOT NULL,
    updated_at  TEXT NOT NULL,

    PRIMARY KEY (action, context)
);
```

Uniqueness of key combos within a context is enforced at the application layer against the **merged** (defaults + overrides) set, since a DB constraint can't see the in-code defaults.

---

## schema_migrations

Tracks which migrations have been applied. Managed by the migration system; not touched by application code.

```sql
CREATE TABLE schema_migrations (
    version     INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    applied_at  TEXT NOT NULL
);
```

---

## Schema design principles (summary)

1. **UUIDs as primary keys.** Stable across catalog merges, backups, and restores. No collision risk.

2. **ISO 8601 strings for timestamps.** SQLite has no native datetime type. ISO 8601 strings sort correctly lexicographically and are human-readable.

3. **Nullable columns for optional data.** New columns added in future migrations must be nullable or have a constant default. This keeps `ALTER TABLE ADD COLUMN` instant even on large tables.

4. **Never remove columns, only add them.** Removing a column requires the expensive create-copy-drop migration pattern. Obsolete columns are deprecated in code but left in the schema.

5. **Denormalize for query performance.** Any field filtered or sorted on in the grid gets a dedicated column and index. Less common data goes in `extended_metadata` JSON.

6. **New features get new tables.** Asset grouping, face detection, and any future features should add new tables rather than modifying `assets`. The `assets` table is hot and wide; adding columns to it should be a deliberate decision.

7. **`PRAGMA user_version`** is used to track the current schema version (set by the migration system). This is SQLite's built-in mechanism and requires no extra table.
