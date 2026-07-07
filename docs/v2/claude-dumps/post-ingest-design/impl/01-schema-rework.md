# impl/01 — Schema Rework (Blocker 1)

> **STATUS: DONE (2026-07-06).** Implemented as specified; notes capture where it refined this spec.
>
> - **FTS (§15) → standalone FTS5, not external-content.** Asset-resident columns are trigger-
>   maintained (`assets_fts_ai/au/ad`; the `au` trigger is scoped `AFTER UPDATE OF` the text
>   columns so file_status/thumbnail_at churn never reindexes), `tags` is app-maintained,
>   rebuildable via `sqlite.RebuildFTS`. Rationale in the 0001 header: external-content's
>   old-value bookkeeping for a non-content `tags` column is more code than trivial per-row
>   triggers. FTS keys on `assets.rowid` → documented "no plain VACUUM" caveat (use VACUUM INTO;
>   RebuildFTS is the escape hatch).
> - **openCatalog:** `sqlite.Open(dir) (*Catalog, error)` — WAL, synchronous(FULL), foreign_keys,
>   busy_timeout(5000). Instance lock via flock in `lock_unix.go` (returns
>   `domain.CatalogLockedError`); `lock_windows.go` is a `ponytail:` no-op stub (Windows third-
>   priority; needs LockFileEx). `:memory:` mode still available for the smoke harness.
> - **aspect_ratio:** VIRTUAL generated column is in the schema (indexable, ready) but deliberately
>   NOT on `domain.Asset` / the repo SELECT — nothing consumes it yet (YAGNI; promotion is free).
> - **UUIDv7** via `domain.NewID()`; all mint sites use it. `file_type`/`color_label` CHECKs
>   dropped; validation lives in `assettype.Classify` (renamed from `domain.Classify` in impl/03).
> - Acceptance tests green: soft-delete→reimport-same-path, root-tag slug conflict, FTS trigger
>   indexing, FK cascade, plus RebuildFTS and the Open instance-lock test.

**Scope:** rewrite `internal/migrations/0001_initial_schema.sql` **in place**. Pre-release, zero
real catalogs exist — do NOT stack a migration 0002. Also touch: the UUID helper, marshal helpers.
**Blocked by:** nothing. **Blocks:** impl/02, impl/04.
**References:** `03-data-model.md` for the full roster and classification; D3/D8/D9/D11 in the decision log.

## Changes to 0001, exhaustively

### assets table
1. DROP the CHECK on `color_label` and the CHECK on `file_type` (doomed constraints — custom labels
   P2, new types P3; SQLite CHECKs are unalterable). Validation lives in `assettype.Classify` (the
   type registry) / the label registry. KEEP CHECKs on `flag`, `file_status`, `rating` range.
2. `partial_hash` → `NOT NULL` (the importer always writes it).
3. ADD `judgment_modified_at TEXT` (nullable; bumped ONLY by the judgment writer — see impl/02).
4. ADD `title TEXT`, `caption TEXT` (observation class; FTS targets).
5. ADD `aspect_ratio REAL GENERATED ALWAYS AS (CASE WHEN width > 0 AND height > 0 THEN 1.0 * width / height END) VIRTUAL`.
6. REPLACE `UNIQUE INDEX idx_assets_source_path` with a partial unique:
   `CREATE UNIQUE INDEX idx_assets_source_path ON assets(source_id, relative_path) WHERE is_deleted = 0;`
   (soft-delete then re-import at same path must not conflict).
7. ADD partial sort indexes: `filename WHERE is_deleted=0`, `size_bytes WHERE is_deleted=0`.
   (captured_at, ingested_at, rating, color_label, flag, file_type already exist — keep.)
8. Composite dedup index: replace the bare `partial_hash` index with `(partial_hash, size_bytes)`.

### sources table
9. REPLACE `status TEXT CHECK(active|offline|removed)` with:
   - `enabled INTEGER NOT NULL DEFAULT 1` (judgment: user activates/deactivates)
   - `connectivity TEXT NOT NULL DEFAULT 'online' CHECK(connectivity IN ('online','offline'))`
     (observation: volume monitor / reconciler writes)
   "Removed" is modeled by deletion flow, not a status. Update `domain.Source`, repo, and the
   frontend model/mock to match (frontend `SourceStatus` splits the same way).

### tags table
10. REPLACE `UNIQUE(slug, parent_id)` with expression index:
    `CREATE UNIQUE INDEX idx_tags_slug_parent ON tags(slug, IFNULL(parent_id,''));`
    (SQLite treats NULLs as distinct — two root "travel" tags were admissible).

### New tables
11. `sidecar_files`:
    ```sql
    CREATE TABLE sidecar_files (
        id                TEXT PRIMARY KEY,
        source_id         TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
        dir               TEXT NOT NULL,      -- relative directory, '' = source root
        stem              TEXT NOT NULL,      -- lowercase basename without final extension
        ext               TEXT NOT NULL,      -- 'xmp', 'aae', 'thm', ...
        relative_path     TEXT NOT NULL,      -- full relative path (convenience; derivable)
        size_bytes        INTEGER NOT NULL,
        mtime             TEXT NOT NULL,
        partial_hash      TEXT NOT NULL,
        attached_asset_id TEXT REFERENCES assets(id) ON DELETE SET NULL,  -- derived; grouping writes
        first_seen_at     TEXT NOT NULL,
        updated_at        TEXT NOT NULL,
        UNIQUE(source_id, relative_path)
    );
    CREATE INDEX idx_sidecars_key ON sidecar_files(source_id, dir, stem);
    ```
12. `import_sessions` + `import_errors` (the DLQ, D13):
    ```sql
    CREATE TABLE import_sessions (
        id            TEXT PRIMARY KEY,
        source_id     TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
        kind          TEXT NOT NULL CHECK(kind IN ('import','reconcile','watch')),
        started_at    TEXT NOT NULL,
        finished_at   TEXT,
        added         INTEGER NOT NULL DEFAULT 0,
        updated       INTEGER NOT NULL DEFAULT 0,
        moved         INTEGER NOT NULL DEFAULT 0,
        skipped       INTEGER NOT NULL DEFAULT 0,
        dups          INTEGER NOT NULL DEFAULT 0,
        errors        INTEGER NOT NULL DEFAULT 0,
        skipped_unknown_json TEXT,   -- {"braw": 3100, "cube": 312} per-extension tallies
        skipped_ignored_json TEXT    -- same shape, ignore-list hits
    );
    CREATE INDEX idx_sessions_started ON import_sessions(started_at);
    CREATE TABLE import_errors (
        id          TEXT PRIMARY KEY,
        session_id  TEXT NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
        path        TEXT NOT NULL,
        stage       TEXT NOT NULL,          -- scan|hash|match|extract|thumb|write
        reason_code TEXT NOT NULL,          -- machine-readable taxonomy, e.g. 'decode_failed'
        message     TEXT NOT NULL,          -- raw error
        attempts    INTEGER NOT NULL DEFAULT 1,
        occurred_at TEXT NOT NULL
    );
    CREATE INDEX idx_import_errors_session ON import_errors(session_id);
    ```

### FK delete rules (currently absent entirely)
13. `asset_tags`, `collection_assets`, `asset_group_members`: CASCADE both directions.
    `tags.parent_id`, `collections.parent_id`: CASCADE. `collections.cover_asset_id`,
    `asset_groups.cover_asset_id`: SET NULL. `duplicates.*_asset_id`: CASCADE.
    `assets.source_id`: RESTRICT (remove-source flow must be explicit).

### asset_groups
14. ADD `origin TEXT NOT NULL DEFAULT 'auto' CHECK(origin IN ('auto','manual'))` (stable enum, CHECK ok).

### FTS5 rebuild (D3, `03-data-model.md` §5)
15. REPLACE the standalone `assets_fts` with external-content:
    ```sql
    CREATE VIRTUAL TABLE assets_fts USING fts5(
        filename, camera_make, camera_model, lens_model, note, title, caption, tags,
        content='assets', content_rowid='rowid'
    );
    ```
    Wait — `tags` is not an assets column. Two options; the session chose: keep `tags` in the FTS
    table as an app-maintained column. With external-content that's awkward (content table lacks
    the column). RESOLUTION for the implementer: use external-content for the seven asset-resident
    columns and maintain `tags` via a **contentless-delete companion** OR simply keep `assets_fts`
    standalone but trigger-maintained for asset columns + app-maintained for tags. Choose whichever
    is less code; the non-negotiables are: (a) triggers keep asset-resident text in sync so no
    writer can forget, (b) `SetAssetTags` updates the tags text, (c) a registered rebuild function
    repopulates from scratch. Document the choice in the schema file header.
16. Write the INSERT/UPDATE/DELETE triggers for the asset-resident columns.

### Removals
17. DROP the `keybindings` table (D16 — overrides live at settings key `ui.keybindings`).
18. DROP `PRAGMA user_version = 1;` (schema_migrations is the single version authority).

### Code-side
19. UUID helper: switch `uuid.NewString()` (v4) call sites to UUIDv7 (`uuid.Must(uuid.NewV7()).String()`
    with github.com/google/uuid). One helper func `domain.NewID()`; all call sites use it.
20. Update `domain.Asset` / `domain.Source` structs + `internal/sqlite` scan/marshal for the new
    and changed columns. Update frontend `models/` + `mock-api` for the sources split (small).

## openCatalog (pull-in item)
Add a real opener alongside the `:memory:` smoke path (the smoke harness in `internal/main.go`
stays — see `05-code-disposition.md`; it may use either mode) (~30 lines):
file path in catalog dir, DSN pragmas: `_pragma=journal_mode(WAL)`, `synchronous(FULL)`,
`foreign_keys(1)`, `busy_timeout(5000)`. Acquire an advisory lock file (`catalog.lock`,
flock-style; clear error if held — instance lock, one per catalog).

## Acceptance
- `go test ./...` green; existing repo tests updated for new columns.
- A test that soft-deletes an asset then re-creates at the same path (must succeed).
- A test that two root tags with the same slug conflict (must fail to insert).
- A test inserting an asset and FTS-searching its filename (triggers work).
- A test that FK cascade removes asset_tags when a tag is deleted.
