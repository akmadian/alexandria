-- Alexandria catalog schema, v1 (pre-release; edited in place, not stacked).
--
-- Column classes (see docs/.../03-data-model.md §1) are annotated per column:
--   [obs]  observation  — copied from the filesystem; ingest/watcher write it, world wins
--   [jdg]  judgment     — user-declared; only the user-action path writes it, DB wins
--   [syn]  sync-state   — a reconciler's own cursor; only its owner writes it
--   [der]  derived      — computed from the above; jobs write it, rebuildable
--
-- FTS choice (impl/01 §15): assets_fts is a *standalone* FTS5 table (stores its own
-- text copy), NOT external-content. Asset-resident columns are kept in sync by
-- triggers below; the `tags` column is app-maintained (SetAssetTags rewrites it) and
-- the whole index is rebuildable via sqlite.RebuildFTS. This is deliberately the
-- boring option: each trigger is a trivial single-row statement, and there is no
-- external-content "old value" bookkeeping. The index is derived state — the
-- duplicated text is cheap and disposable.
--
-- Note: FTS rows key on assets.rowid. Back up with the SQLite backup API / VACUUM
-- INTO (which never renumbers rowids); do NOT run a plain in-place VACUUM. If one is
-- ever run, call sqlite.RebuildFTS afterwards.

CREATE TABLE IF NOT EXISTS sources (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,                              -- [jdg]
    kind                TEXT NOT NULL CHECK(kind IN ('local', 'external_drive', 'smb', 'nfs')),
    base_path           TEXT NOT NULL,                             -- [jdg]
    filesystem_uuid     TEXT,                                      -- [obs]
    disk_serial         TEXT,                                      -- [obs]
    volume_label        TEXT,                                      -- [obs]
    host                TEXT,                                      -- [jdg]
    share_name          TEXT,                                      -- [jdg]
    poll_interval_secs  INTEGER,                                   -- [jdg]
    scan_recursively    INTEGER NOT NULL DEFAULT 1,                -- [jdg]
    enabled             INTEGER NOT NULL DEFAULT 1,                -- [jdg] user activates/deactivates
    connectivity        TEXT NOT NULL DEFAULT 'online'             -- [obs] volume monitor / reconciler
                            CHECK(connectivity IN ('online', 'offline')),
    last_scanned_at     TEXT,                                      -- [syn]
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sources_filesystem_uuid ON sources(filesystem_uuid);

CREATE TABLE IF NOT EXISTS assets (
    id                  TEXT PRIMARY KEY,
    source_id           TEXT NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    -- [obs] file identity & facts ------------------------------------------------
    relative_path       TEXT NOT NULL,
    file_status         TEXT NOT NULL DEFAULT 'online' CHECK(file_status IN ('online', 'offline', 'missing')),
    filename            TEXT NOT NULL,
    extension           TEXT NOT NULL,
    mime_type           TEXT NOT NULL,
    file_type           TEXT NOT NULL,                             -- no CHECK: file types grow (P3); validated in assettype.Classify
    size_bytes          INTEGER NOT NULL,
    mtime               TEXT NOT NULL,
    partial_hash        TEXT NOT NULL,                             -- always written by the hasher
    -- [obs] extracted metadata ---------------------------------------------------
    width               INTEGER,
    height              INTEGER,
    duration_secs       REAL,
    color_space         TEXT,
    bit_depth           INTEGER,
    captured_at         TEXT,
    camera_make         TEXT,
    camera_model        TEXT,
    lens_model          TEXT,
    focal_length_mm     REAL,
    aperture            REAL,
    shutter_speed       TEXT,
    iso                 INTEGER,
    gps_lat             REAL,
    gps_lon             REAL,
    creator             TEXT,
    copyright           TEXT,
    title               TEXT,                                      -- IPTC/XMP dc:title (FTS target)
    caption             TEXT,                                      -- IPTC/XMP dc:description (distinct from note)
    extended_metadata   TEXT,                                      -- JSON, keyed by exiftool Group:Tag
    -- [der] computed -------------------------------------------------------------
    aspect_ratio        REAL GENERATED ALWAYS AS (CASE WHEN width > 0 AND height > 0 THEN 1.0 * width / height END) VIRTUAL,
    -- [jdg] user judgment --------------------------------------------------------
    rating              INTEGER CHECK(rating IS NULL OR (rating >= 0 AND rating <= 5)),
    color_label         TEXT,                                      -- no CHECK: custom labels (P2); validated in app
    flag                TEXT CHECK(flag IN ('pick', 'reject') OR flag IS NULL),
    note                TEXT,
    is_deleted          INTEGER NOT NULL DEFAULT 0,
    deleted_at          TEXT,
    judgment_modified_at TEXT,                                     -- bumped ONLY by the judgment writer (impl/02)
    -- [syn] reconciler cursors ---------------------------------------------------
    last_verified_at    TEXT,
    xmp_last_read_at    TEXT,
    xmp_last_written_at TEXT,
    xmp_hash            TEXT,
    -- [der] thumbnail marker -----------------------------------------------------
    thumbnail_at        TEXT,
    ingested_at         TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_assets_captured_at   ON assets(COALESCE(captured_at, mtime)) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_file_type     ON assets(file_type) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_rating        ON assets(rating) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_color_label   ON assets(color_label) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_flag          ON assets(flag) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_ingested_at   ON assets(ingested_at) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_filename      ON assets(filename) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_size_bytes    ON assets(size_bytes) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_hash          ON assets(partial_hash, size_bytes);
CREATE INDEX IF NOT EXISTS idx_assets_source_status ON assets(source_id, file_status);
-- Soft-delete safe: a removed row keeps its path, so a later re-import at the same
-- path must not collide. Only live rows are constrained unique.
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_source_path ON assets(source_id, relative_path) WHERE is_deleted = 0;

-- Full-text search. Standalone FTS5 (see header). asset_id is stored UNINDEXED so a
-- MATCH result maps back to the asset without a join; the rowid mirrors assets.rowid
-- so triggers address rows by rowid.
CREATE VIRTUAL TABLE IF NOT EXISTS assets_fts USING fts5(
    asset_id UNINDEXED,
    filename,
    camera_make,
    camera_model,
    lens_model,
    title,
    caption,
    note,
    tags
);

CREATE TRIGGER IF NOT EXISTS assets_fts_ai AFTER INSERT ON assets BEGIN
    INSERT INTO assets_fts (rowid, asset_id, filename, camera_make, camera_model, lens_model, title, caption, note, tags)
    VALUES (new.rowid, new.id, new.filename, new.camera_make, new.camera_model, new.lens_model, new.title, new.caption, new.note, '');
END;

-- Fires only when an indexed text column changes (not on file_status/thumbnail_at/etc.);
-- leaves `tags` untouched (app-maintained).
CREATE TRIGGER IF NOT EXISTS assets_fts_au
AFTER UPDATE OF filename, camera_make, camera_model, lens_model, title, caption, note ON assets BEGIN
    UPDATE assets_fts SET
        filename = new.filename, camera_make = new.camera_make, camera_model = new.camera_model,
        lens_model = new.lens_model, title = new.title, caption = new.caption, note = new.note
    WHERE rowid = new.rowid;
END;

CREATE TRIGGER IF NOT EXISTS assets_fts_ad AFTER DELETE ON assets BEGIN
    DELETE FROM assets_fts WHERE rowid = old.rowid;
END;

CREATE TABLE IF NOT EXISTS sidecar_files (
    id                  TEXT PRIMARY KEY,
    source_id           TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    dir                 TEXT NOT NULL,                             -- [obs] relative dir, '' = source root
    stem                TEXT NOT NULL,                             -- [obs] lowercase basename sans final ext
    ext                 TEXT NOT NULL,                             -- [obs] 'xmp', 'aae', 'thm', ...
    relative_path       TEXT NOT NULL,                             -- [obs] full relative path (convenience)
    size_bytes          INTEGER NOT NULL,                         -- [obs]
    mtime               TEXT NOT NULL,                            -- [obs]
    partial_hash        TEXT NOT NULL,                           -- [obs]
    attached_asset_id   TEXT REFERENCES assets(id) ON DELETE SET NULL,  -- [der] grouping engine writes it
    first_seen_at       TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    UNIQUE(source_id, relative_path)
);

CREATE INDEX IF NOT EXISTS idx_sidecars_key ON sidecar_files(source_id, dir, stem);

CREATE TABLE IF NOT EXISTS duplicates (
    id                  TEXT PRIMARY KEY,
    original_asset_id   TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    duplicate_asset_id  TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    partial_hash        TEXT NOT NULL,
    detected_at         TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'resolved', 'ignored')),
    resolved_at         TEXT,
    UNIQUE(original_asset_id, duplicate_asset_id)
);

CREATE INDEX IF NOT EXISTS idx_duplicates_status ON duplicates(status);

CREATE TABLE IF NOT EXISTS tags (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,                             -- display form; first-seen wins
    slug        TEXT NOT NULL,                             -- normalized match key
    parent_id   TEXT REFERENCES tags(id) ON DELETE CASCADE,-- adjacency: structural truth
    color       TEXT,                                      -- [jdg] hex; meaningful only when color_mode='custom'
    color_mode  TEXT NOT NULL DEFAULT 'inherit'            -- [jdg] tri-state a bare nullable color can't express
                    CHECK(color_mode IN ('inherit', 'custom', 'none')),
    path        TEXT NOT NULL,                             -- [der] materialized ancestry '/rootId/…/selfId/'
    created_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tags_parent ON tags(parent_id);
-- SQLite treats NULLs as distinct in a UNIQUE index, so UNIQUE(slug, parent_id) would
-- admit two root tags with the same slug. Collapse NULL parents to '' to constrain them.
CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_slug_parent ON tags(slug, IFNULL(parent_id, ''));
-- Subtree queries are an indexed prefix scan: `path GLOB parent.path || '*'`. GLOB (not
-- LIKE) so SQLite uses this index; tag IDs are UUIDs with no GLOB metacharacters.
CREATE INDEX IF NOT EXISTS idx_tags_path ON tags(path);

CREATE TABLE IF NOT EXISTS asset_tags (
    asset_id    TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    tag_id      TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    source      TEXT NOT NULL DEFAULT 'user' CHECK(source IN ('user', 'xmp', 'lr')),  -- [jdg|obs] per-row class
    removed_at  TEXT,                                     -- [jdg] tombstone: user-suppressed an imported tag; null = active
    created_at  TEXT NOT NULL,
    PRIMARY KEY (asset_id, tag_id)
);

-- Reverse (tag → assets) hot path, tombstone-aware so suppressed rows never bloat it.
CREATE INDEX IF NOT EXISTS idx_asset_tags_tag ON asset_tags(tag_id) WHERE removed_at IS NULL;

CREATE TABLE IF NOT EXISTS collections (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    parent_id       TEXT REFERENCES collections(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL DEFAULT 'manual' CHECK(kind IN ('manual', 'smart')),
    query           TEXT,
    cover_asset_id  TEXT REFERENCES assets(id) ON DELETE SET NULL,
    sort_field      TEXT,
    sort_dir        TEXT NOT NULL DEFAULT 'asc' CHECK(sort_dir IN ('asc', 'desc')),
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_collections_parent ON collections(parent_id);

CREATE TABLE IF NOT EXISTS collection_assets (
    collection_id   TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    asset_id        TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    position        INTEGER,
    added_at        TEXT NOT NULL,
    PRIMARY KEY (collection_id, asset_id)
);

CREATE INDEX IF NOT EXISTS idx_collection_assets_collection ON collection_assets(collection_id, position);
CREATE INDEX IF NOT EXISTS idx_collection_assets_asset      ON collection_assets(asset_id);

CREATE TABLE IF NOT EXISTS import_sessions (
    id                   TEXT PRIMARY KEY,
    source_id            TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    kind                 TEXT NOT NULL CHECK(kind IN ('import', 'reconcile', 'watch')),
    started_at           TEXT NOT NULL,
    finished_at          TEXT,
    added                INTEGER NOT NULL DEFAULT 0,
    updated              INTEGER NOT NULL DEFAULT 0,
    moved                INTEGER NOT NULL DEFAULT 0,
    skipped              INTEGER NOT NULL DEFAULT 0,
    dups                 INTEGER NOT NULL DEFAULT 0,
    errors               INTEGER NOT NULL DEFAULT 0,
    skipped_unknown_json TEXT,   -- {"braw": 3100, ...} per-extension tallies
    skipped_ignored_json TEXT    -- same shape, ignore-list hits
);

CREATE INDEX IF NOT EXISTS idx_sessions_started ON import_sessions(started_at);

CREATE TABLE IF NOT EXISTS import_errors (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    stage       TEXT NOT NULL,          -- scan|hash|match|extract|thumb|write
    reason_code TEXT NOT NULL,          -- machine-readable taxonomy, e.g. 'decode_failed'
    message     TEXT NOT NULL,          -- raw error
    attempts    INTEGER NOT NULL DEFAULT 1,
    occurred_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_import_errors_session ON import_errors(session_id);

-- Settings are NOT stored in the catalog DB. They live as plain JSON files
-- (settings.json in the catalog dir, machine.json/keybindings.json in the app
-- config dir) — see internal/settings and impl/11. The old `settings` KV table
-- was dropped here per this repo's pre-1.0 edit-in-place migration convention.
