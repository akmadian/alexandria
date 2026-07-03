# Schema Migrations

## Overview

The migration system manages changes to the SQLite schema over time. Every schema change — adding a table, adding a column, adding an index — is expressed as a numbered migration file. Migrations are applied in order, tracked in the database, and never re-applied.

The migration system is designed around one principle: **migrations should be boring**. A migration should be a small, safe, mechanical change. The schema design principles (described below) make most future migrations a simple `CREATE TABLE` or `ALTER TABLE ADD COLUMN`.

---

## How it works

### Migration files

Each migration is a numbered SQL file in `internal/migrations/`:

```
internal/migrations/
  0001_initial_schema.sql
  0002_add_extended_metadata.sql
  0003_add_xmp_fields.sql
  0004_add_asset_groups.sql
  ...
```

Files are named `{version}_{description}.sql`. The version number is a zero-padded integer. Descriptions use underscores. Files are sorted lexicographically — the zero-padding ensures correct ordering.

Each file contains complete, self-contained SQL. It should be runnable independently (given the state produced by all prior migrations). It must be idempotent where possible (e.g. `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`).

### Tracking table

The `schema_migrations` table tracks which migrations have been applied:

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    applied_at  TEXT NOT NULL
);
```

This table is created as part of the migration system's initialisation, before any migrations run.

### Migrator behaviour

On each startup:

1. Ensure `schema_migrations` table exists
2. Read all applied version numbers from the table
3. Compare against the set of migration files embedded in the binary
4. For each unapplied migration (in version order):
   - Begin a SQLite transaction
   - Execute the migration SQL
   - Insert a record into `schema_migrations`
   - Commit
5. If any migration fails: the transaction rolls back, the error propagates, startup aborts

Each migration runs in its own transaction. If migration 7 fails, migrations 1–6 are committed and migration 7 is rolled back. The catalog is in the state produced by migration 6 — consistent, but not fully migrated.

### Migration SQL is embedded in the binary

Migration files are embedded in the Go binary at compile time using Go's `embed` package. This means:

- The binary is self-contained — no external migration files to deploy
- Migrations cannot be accidentally deleted or modified on disk
- The exact set of migrations for a given binary version is fixed

---

## Backup before migration

Before running any pending migrations, Alexandria takes an automatic backup:

```
backups/catalog-{ISO8601-timestamp}.db
```

This uses SQLite's online backup API, which copies the database without locking it for the duration. The backup is complete and consistent.

If the backup fails, startup aborts — do not run migrations without a safety net.

The number of backups retained is configured by `settings.catalog_backup_count` (default: 10). Older backups are pruned on startup after a successful migration.

The error message on migration failure explicitly tells the user the path to their backup:

```
Migration 0007_add_face_regions failed: [error details]
Your catalog has not been modified. A backup is available at:
/Users/name/Library/Application Support/alexandria/backups/catalog-2024-07-01T10-30-00.db
```

---

## Schema version tracking

Alexandria uses SQLite's built-in `PRAGMA user_version` to store the current schema version number. This is set by the migration system after each successful migration:

```sql
PRAGMA user_version = 7;
```

The app binary has a constant `MinSchemaVersion` — the minimum schema version required to run. On startup, after migrations:

- If `PRAGMA user_version` < `MinSchemaVersion` → migration failed silently (should not happen) → abort
- If `PRAGMA user_version` > app's latest known version → catalog was created by a newer Alexandria → "please update the app" → abort

This handles the case where a user accidentally opens a new catalog with an old binary (e.g. they have two Alexandria versions installed and open the wrong one).

---

## Schema design principles for cheap migrations

The way you design the schema today determines how painful future migrations will be. These principles are followed throughout the schema design to keep future migrations cheap:

### 1. Never remove columns — only add them

Dropping a column in SQLite requires the expensive create-copy-drop dance:

```sql
-- expensive: requires full table rewrite
CREATE TABLE assets_new AS SELECT (all columns except removed_col) FROM assets;
DROP TABLE assets;
ALTER TABLE assets_new RENAME TO assets;
-- plus recreate all indexes
```

On a 500k row table over a network drive, this could take minutes and carries data loss risk.

**Policy:** Columns are never removed. If a column becomes obsolete, it is deprecated in application code (the app stops reading/writing it) but left in the schema. The wasted bytes are negligible (SQLite stores NULLs efficiently).

### 2. New columns must be nullable or have a constant default

SQLite can add a column to an existing table instantly with `ALTER TABLE ADD COLUMN` — but only if the column is nullable (`NULL` default) or has a constant literal default. If it's `NOT NULL` without a default, SQLite must rewrite the entire table to populate the new column.

```sql
-- fast, instant on any table size
ALTER TABLE assets ADD COLUMN ai_tags TEXT;
ALTER TABLE assets ADD COLUMN grouped INTEGER NOT NULL DEFAULT 0;

-- slow on large tables — full table rewrite
ALTER TABLE assets ADD COLUMN new_required_field TEXT NOT NULL;
```

**Policy:** All new columns added in migrations must be nullable or have a constant default.

### 3. Prefer new tables over modifying existing ones

Adding a new feature? Model it as a new table. Don't add columns to `assets` unless there is a strong reason to. New tables are zero-cost migrations.

This is why asset grouping, collections, and tags all live in their own tables rather than as columns on `assets`. Adding face detection in a future migration is `CREATE TABLE face_regions` — not `ALTER TABLE assets ADD COLUMN face_data TEXT`.

### 4. JSON columns for flexible, non-queried data

For data that will evolve over time but is not commonly filtered or sorted on, store it as JSON in a TEXT column. Changing the JSON schema requires no database migration — only application code changes.

```sql
ALTER TABLE assets ADD COLUMN extended_metadata TEXT;  -- JSON, instant
```

**Constraint:** Do not store data in JSON that needs to be indexed, filtered, or sorted. JSON fields cannot be indexed efficiently in SQLite without JSONPath expressions, which add query complexity. Only use JSON for display-only data.

### 5. Check constraints as lightweight enums

SQLite `CHECK` constraints enforce valid values without the overhead of a separate lookup table:

```sql
file_type TEXT NOT NULL CHECK(file_type IN ('image', 'video', 'raw', 'vector', 'document', 'audio'))
```

Adding a new valid value is an `ALTER TABLE` constraint change. In SQLite, changing a constraint currently requires the create-copy-drop pattern. However, these enums change rarely (adding a new file type category is an infrequent event) and the constraint provides valuable data integrity guarantees.

**Alternative:** Drop the `CHECK` constraint and enforce valid values in application code only. This is simpler to migrate but less safe (invalid values can reach the database). Given that Alexandria is a single-writer application, application-level enforcement may be sufficient. This decision can be revisited.

---

## Writing a migration

Example: adding face detection tables in a future version.

```sql
-- 0008_add_face_detection.sql

-- New tables for face detection (future P2 feature).
-- persons: named individuals that can be associated with face regions.
-- face_regions: bounding boxes within an asset image where a face was detected.

CREATE TABLE IF NOT EXISTS persons (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    cover_asset_id TEXT REFERENCES assets(id),
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS face_regions (
    id          TEXT PRIMARY KEY,
    asset_id    TEXT NOT NULL REFERENCES assets(id),
    person_id   TEXT REFERENCES persons(id),
    -- Bounding box as fractions of image dimensions (0.0–1.0).
    -- Stored as fractions rather than pixels so they remain valid if
    -- thumbnail dimensions change.
    x           REAL NOT NULL,
    y           REAL NOT NULL,
    width       REAL NOT NULL,
    height      REAL NOT NULL,
    confidence  REAL,           -- ML model confidence score, 0.0–1.0
    created_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_face_regions_asset  ON face_regions(asset_id);
CREATE INDEX IF NOT EXISTS idx_face_regions_person ON face_regions(person_id);

PRAGMA user_version = 8;
```

This migration is:
- Two new tables, no existing table changes
- Instant — no data movement
- `CREATE TABLE IF NOT EXISTS` — idempotent
- Sets `user_version` at the end

This is what most migrations should look like.

---

## Manual catalog operations

For users who are comfortable with SQLite, the catalog can be inspected and queried directly using any SQLite browser (DB Browser for SQLite, TablePlus, etc.). The schema is human-readable. No data is encrypted or obfuscated.

**Warning:** Writing to the catalog outside of Alexandria can corrupt the application state. Reads are always safe; writes should only be done for recovery purposes and with a backup in place.
