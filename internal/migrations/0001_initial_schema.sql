-- Alexandria catalog schema, v1 (pre-release; edited in place, not stacked).
--
-- Column classes (see docs/data-model.md §1) are annotated per column:
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

-- The D24 split (D41): `sources` becomes `volumes` (identity/portability anchor,
-- matched by filesystem UUID — the mount point is resolved live, never stored)
-- plus `folders` (tracked roots + sync scope; one volume, many disjoint folders).
-- The split principle is identity vs. tracking scope, NOT writer class — both
-- tables carry mixed classes, enforced per-column by the catalog writer interfaces.
CREATE TABLE IF NOT EXISTS volumes (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,                              -- [jdg]
    kind                TEXT NOT NULL CHECK(kind IN ('local', 'external_drive', 'smb', 'nfs')),
    host                TEXT,                                      -- [jdg]
    share_name          TEXT,                                      -- [jdg]
    filesystem_uuid     TEXT,                                      -- [obs] identity key: real fs UUID (local/external), 'smb://host/share' / 'nfs://host/export' (network), or residual session-scoped 'dev:N' (exotic fs only — internal/volume prober)
    disk_serial         TEXT,                                      -- [obs]
    volume_label        TEXT,                                      -- [obs]
    connectivity        TEXT NOT NULL DEFAULT 'online'             -- [obs] volume monitor / reconciler
                            CHECK(connectivity IN ('online', 'offline')),
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

-- A volume is found-or-created by filesystem_uuid, so the identity lookup is a
-- unique index (NULLs are distinct in SQLite, so an as-yet-unidentified volume
-- never collides).
CREATE UNIQUE INDEX IF NOT EXISTS idx_volumes_filesystem_uuid ON volumes(filesystem_uuid);

CREATE TABLE IF NOT EXISTS folders (
    id                  TEXT PRIMARY KEY,
    volume_id           TEXT NOT NULL REFERENCES volumes(id) ON DELETE RESTRICT,
    path                TEXT NOT NULL,                             -- [jdg] volume-relative; '' = volume root
    name                TEXT NOT NULL,                             -- [jdg]
    sync_mode           TEXT NOT NULL DEFAULT 'manual'             -- [jdg]
                            CHECK(sync_mode IN ('manual', 'watched', 'scheduled')),
    scan_recursively    INTEGER NOT NULL DEFAULT 1,                -- [jdg]
    enabled             INTEGER NOT NULL DEFAULT 1,                -- [jdg] user activates/deactivates
    poll_interval_secs  INTEGER,                                   -- [jdg]
    last_scanned_at     TEXT,                                      -- [syn]
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_folders_volume ON folders(volume_id);
-- Tracked roots on one volume are disjoint by invariant (D41); a volume never
-- carries two folders at the same path.
CREATE UNIQUE INDEX IF NOT EXISTS idx_folders_volume_path ON folders(volume_id, path);

CREATE TABLE IF NOT EXISTS assets (
    id                  TEXT PRIMARY KEY,
    volume_id           TEXT NOT NULL REFERENCES volumes(id) ON DELETE RESTRICT,
    -- [obs] file identity & facts ------------------------------------------------
    relative_path       TEXT NOT NULL,                             -- volume-relative path; on-disk bytes, the I/O truth
    path_key            TEXT NOT NULL,                             -- [der] NFC(relative_path); "compare keys, open bytes" (D24) — rebuildable
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
    -- [der] enrichment artifacts (NULL = not computed; the missing-artifact scan IS
    -- the queue, D25/D28; all cleared by the reimport staleness path, task 19) -----
    thumbnail_at        TEXT,
    sharpness           REAL,    -- raw variance of Laplacian on the 512px thumb; ranking is the contract, not the absolute value
    clipping_highlights REAL,    -- % of thumb pixels at/near pure white
    clipping_shadows    REAL,    -- % of thumb pixels at/near pure black
    phash               TEXT,    -- 64-bit perceptual hash (dHash), hex; the near-dup query surface is deferred (DEFERRED §12)
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
CREATE INDEX IF NOT EXISTS idx_assets_volume_status ON assets(volume_id, file_status);
-- Identity is (volume_id, path_key): the NFC key, not the raw bytes, so an
-- NFD-stored macOS name and its NFC query form are one identity (D24 — never a
-- phantom new asset). Soft-delete safe: a removed row keeps its key, so a later
-- re-import at the same path must not collide; only live rows are constrained.
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_volume_path_key ON assets(volume_id, path_key) WHERE is_deleted = 0;
-- Folder-scope prefix scans and the importer's per-subtree known-file load walk
-- path_key ranges (GLOB prefix), so key it for the index.
CREATE INDEX IF NOT EXISTS idx_assets_volume_path_key_all ON assets(volume_id, path_key);

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
    volume_id           TEXT NOT NULL REFERENCES volumes(id) ON DELETE CASCADE,
    dir                 TEXT NOT NULL,                             -- [obs] volume-relative dir, '' = volume root
    stem                TEXT NOT NULL,                             -- [obs] lowercase basename sans final ext
    ext                 TEXT NOT NULL,                             -- [obs] 'xmp', 'aae', 'thm', ...
    relative_path       TEXT NOT NULL,                             -- [obs] full volume-relative path (I/O bytes)
    path_key            TEXT NOT NULL,                             -- [der] NFC(relative_path); compare keys, open bytes
    size_bytes          INTEGER NOT NULL,                         -- [obs]
    mtime               TEXT NOT NULL,                            -- [obs]
    partial_hash        TEXT NOT NULL,                           -- [obs]
    attached_asset_id   TEXT REFERENCES assets(id) ON DELETE SET NULL,  -- [der] grouping engine writes it
    first_seen_at       TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    UNIQUE(volume_id, path_key)
);

CREATE INDEX IF NOT EXISTS idx_sidecars_key ON sidecar_files(volume_id, dir, stem);

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
    volume_id            TEXT NOT NULL REFERENCES volumes(id) ON DELETE CASCADE,
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

-- The enrichment DLQ (D28): one row per (asset, kind) failure. Deliberately NOT
-- an import_errors extension — import failures are path-keyed (pre-identity),
-- enrichment failures are (asset, kind)-keyed (post-identity). "Absence is
-- ambiguous" is why this table exists: a missing artifact means "not yet"
-- UNLESS a row here says "tried and failed"; the missing-artifact scan skips
-- attempt-exhausted rows so a corrupt file never spins forever, and the UI
-- renders failed instead of an eternal spinner.
CREATE TABLE IF NOT EXISTS enrichment_errors (
    asset_id        TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL,          -- job-kind registry key, e.g. 'thumbnail'
    reason_code     TEXT NOT NULL,          -- machine-readable taxonomy, e.g. 'decode_failed'
    message         TEXT NOT NULL,          -- raw error
    attempts        INTEGER NOT NULL DEFAULT 1,
    last_attempt_at TEXT NOT NULL,
    PRIMARY KEY (asset_id, kind)
);

CREATE INDEX IF NOT EXISTS idx_enrichment_errors_kind ON enrichment_errors(kind);

-- Settings are NOT stored in the catalog DB. They live as plain JSON files
-- (settings.json in the catalog dir, machine.json/keybindings.json in the app
-- config dir) — see internal/settings and impl/11. The old `settings` KV table
-- was dropped here per this repo's pre-1.0 edit-in-place migration convention.
