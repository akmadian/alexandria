-- catalog-stats.sql — one-screen health check for a catalog DB.
--
--   sqlite3 -box <catalog-dir>/catalog.db < cmd/dev/sql/catalog-stats.sql

.mode box

.print
.print == row counts ==
SELECT 'assets'          AS "table", COUNT(*) AS rows FROM assets
UNION ALL SELECT 'sources',           COUNT(*) FROM sources
UNION ALL SELECT 'sidecar_files',     COUNT(*) FROM sidecar_files
UNION ALL SELECT 'duplicates',        COUNT(*) FROM duplicates
UNION ALL SELECT 'tags',              COUNT(*) FROM tags
UNION ALL SELECT 'collections',       COUNT(*) FROM collections
UNION ALL SELECT 'import_sessions',   COUNT(*) FROM import_sessions
UNION ALL SELECT 'import_errors',     COUNT(*) FROM import_errors;

.print
.print == assets by status ==
SELECT file_status, COUNT(*) AS assets,
       SUM(thumbnail_at IS NOT NULL) AS thumbnailed,
       SUM(width IS NULL)            AS no_metadata
FROM assets WHERE is_deleted = 0 GROUP BY file_status;

.print
.print == pending review pairs (duplicates ledger) ==
SELECT status, COUNT(*) AS pairs FROM duplicates GROUP BY status;

.print
.print == DLQ by reason (last 500 rows) ==
SELECT reason_code, stage, COUNT(*) AS errors
FROM (SELECT * FROM import_errors ORDER BY rowid DESC LIMIT 500)
GROUP BY reason_code, stage ORDER BY errors DESC;

.print
.print == recent import sessions ==
SELECT id, started_at, finished_at, added, updated, skipped, dups, errors
FROM import_sessions ORDER BY started_at DESC LIMIT 8;
