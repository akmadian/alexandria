-- catalog-wipe.sql — empty every content table for a fresh-slate catalog.
-- The schema, settings files, and DB file itself survive; only rows go.
--
--   sqlite3 <catalog-dir>/catalog.db < cmd/dev/sql/catalog-wipe.sql
--
-- SQL cannot delete files: thumbnails/ and traces/ next to catalog.db keep
-- their contents — remove those directories by hand if you want them gone too.
-- (The FTS index empties itself via the assets_fts_ad trigger.)

PRAGMA foreign_keys = OFF;

DELETE FROM collection_assets;
DELETE FROM collections;
DELETE FROM asset_tags;
DELETE FROM tags;
DELETE FROM duplicates;
DELETE FROM sidecar_files;
DELETE FROM import_errors;
DELETE FROM import_sessions;
DELETE FROM assets;
DELETE FROM sources;

PRAGMA foreign_keys = ON;
VACUUM;

SELECT 'catalog wiped — ' || (SELECT COUNT(*) FROM assets) || ' assets remain (want 0)';
