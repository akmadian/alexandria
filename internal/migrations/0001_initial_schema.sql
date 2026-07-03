CREATE TABLE IF NOT EXISTS sources (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    kind                TEXT NOT NULL CHECK(kind IN ('local', 'external_drive', 'smb', 'nfs')),
    base_path           TEXT NOT NULL,
    filesystem_uuid     TEXT,
    disk_serial         TEXT,
    volume_label        TEXT,
    host                TEXT,
    share_name          TEXT,
    poll_interval_secs  INTEGER,
    scan_recursively    INTEGER NOT NULL DEFAULT 1,
    status              TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'offline', 'removed')),
    last_scanned_at     TEXT,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sources_filesystem_uuid ON sources(filesystem_uuid);
CREATE INDEX IF NOT EXISTS idx_sources_status ON sources(status);

CREATE TABLE IF NOT EXISTS assets (
    id                  TEXT PRIMARY KEY,
    source_id           TEXT NOT NULL REFERENCES sources(id),
    relative_path       TEXT NOT NULL,
    file_status         TEXT NOT NULL DEFAULT 'online' CHECK(file_status IN ('online', 'offline', 'missing')),
    last_verified_at    TEXT,
    filename            TEXT NOT NULL,
    extension           TEXT NOT NULL,
    mime_type           TEXT NOT NULL,
    file_type           TEXT NOT NULL CHECK(file_type IN ('image', 'video', 'raw', 'vector', 'document', 'audio')),
    size_bytes          INTEGER NOT NULL,
    mtime               TEXT NOT NULL,
    partial_hash        TEXT,
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
    extended_metadata   TEXT,
    rating              INTEGER CHECK(rating IS NULL OR (rating >= 0 AND rating <= 5)),
    color_label         TEXT CHECK(color_label IN ('red', 'orange', 'yellow', 'green', 'blue', 'purple') OR color_label IS NULL),
    flag                TEXT CHECK(flag IN ('pick', 'reject') OR flag IS NULL),
    note                TEXT,
    xmp_last_read_at    TEXT,
    xmp_last_written_at TEXT,
    xmp_hash            TEXT,
    thumbnail_path      TEXT,
    thumbnail_at        TEXT,
    is_deleted          INTEGER NOT NULL DEFAULT 0,
    deleted_at          TEXT,
    ingested_at         TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_assets_captured_at   ON assets(captured_at) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_file_type     ON assets(file_type) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_rating        ON assets(rating) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_color_label   ON assets(color_label) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_flag          ON assets(flag) WHERE is_deleted = 0;
CREATE INDEX IF NOT EXISTS idx_assets_partial_hash  ON assets(partial_hash);
CREATE INDEX IF NOT EXISTS idx_assets_ingested_at   ON assets(ingested_at) WHERE is_deleted = 0;
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_source_path ON assets(source_id, relative_path);
CREATE INDEX IF NOT EXISTS idx_assets_source_status ON assets(source_id, file_status);

CREATE VIRTUAL TABLE IF NOT EXISTS assets_fts USING fts5(
    asset_id UNINDEXED,
    filename,
    camera_make,
    camera_model,
    lens_model,
    tags,
    note
);

CREATE TABLE IF NOT EXISTS duplicates (
    id                  TEXT PRIMARY KEY,
    original_asset_id   TEXT NOT NULL REFERENCES assets(id),
    duplicate_asset_id  TEXT NOT NULL REFERENCES assets(id),
    partial_hash        TEXT NOT NULL,
    detected_at         TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'resolved', 'ignored')),
    resolved_at         TEXT,
    UNIQUE(original_asset_id, duplicate_asset_id)
);

CREATE INDEX IF NOT EXISTS idx_duplicates_status ON duplicates(status);

CREATE TABLE IF NOT EXISTS tags (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL,
    parent_id   TEXT REFERENCES tags(id),
    color       TEXT,
    created_at  TEXT NOT NULL,
    UNIQUE(slug, parent_id)
);

CREATE INDEX IF NOT EXISTS idx_tags_parent ON tags(parent_id);

CREATE TABLE IF NOT EXISTS asset_tags (
    asset_id    TEXT NOT NULL REFERENCES assets(id),
    tag_id      TEXT NOT NULL REFERENCES tags(id),
    source      TEXT NOT NULL DEFAULT 'user' CHECK(source IN ('user', 'xmp', 'lr')),
    created_at  TEXT NOT NULL,
    PRIMARY KEY (asset_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_asset_tags_asset ON asset_tags(asset_id);
CREATE INDEX IF NOT EXISTS idx_asset_tags_tag   ON asset_tags(tag_id);

CREATE TABLE IF NOT EXISTS collections (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    parent_id       TEXT REFERENCES collections(id),
    kind            TEXT NOT NULL DEFAULT 'manual' CHECK(kind IN ('manual', 'smart')),
    query           TEXT,
    cover_asset_id  TEXT REFERENCES assets(id),
    sort_field      TEXT,
    sort_dir        TEXT NOT NULL DEFAULT 'asc' CHECK(sort_dir IN ('asc', 'desc')),
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_collections_parent ON collections(parent_id);

CREATE TABLE IF NOT EXISTS collection_assets (
    collection_id   TEXT NOT NULL REFERENCES collections(id),
    asset_id        TEXT NOT NULL REFERENCES assets(id),
    position        INTEGER,
    added_at        TEXT NOT NULL,
    PRIMARY KEY (collection_id, asset_id)
);

CREATE INDEX IF NOT EXISTS idx_collection_assets_collection ON collection_assets(collection_id, position);
CREATE INDEX IF NOT EXISTS idx_collection_assets_asset      ON collection_assets(asset_id);

CREATE TABLE IF NOT EXISTS asset_groups (
    id              TEXT PRIMARY KEY,
    cover_asset_id  TEXT REFERENCES assets(id),
    created_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS asset_group_members (
    group_id    TEXT NOT NULL REFERENCES asset_groups(id),
    asset_id    TEXT NOT NULL REFERENCES assets(id),
    role        TEXT NOT NULL CHECK(role IN ('raw', 'jpeg_sidecar', 'source', 'export', 'member')),
    PRIMARY KEY (group_id, asset_id)
);

CREATE INDEX IF NOT EXISTS idx_group_members_group ON asset_group_members(group_id);
CREATE INDEX IF NOT EXISTS idx_group_members_asset ON asset_group_members(asset_id);

CREATE TABLE IF NOT EXISTS settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS keybindings (
    action      TEXT NOT NULL,
    context     TEXT NOT NULL CHECK(context IN ('global', 'grid', 'detail', 'import')),
    key_combo   TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (action, context)
);

PRAGMA user_version = 1;
