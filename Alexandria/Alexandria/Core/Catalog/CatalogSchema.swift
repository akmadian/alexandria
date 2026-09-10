//
//  CatalogSchema.swift
//  Alexandria
//

/// The v0 catalog schema, one annotated SQL document (schema round, ratified
/// 2026-09-10). Pre-1.0 this migration is edited in place and
/// `eraseDatabaseOnSchemaChange` rebuilds dev catalogs on drift; stacked
/// migrations begin at 1.0.
///
/// Conventions — ids are TEXT UUIDv7 (readable in any sqlite3 shell);
/// timestamps are ISO 8601 UTC, millisecond precision, 'Z' suffix, so
/// lexicographic order is chronological order. Column classes: [obs]
/// filesystem truth · [jdg] user-declared · [der] rebuildable. Foreign keys:
/// RESTRICT where deletion must be a designed verb, CASCADE only for rows
/// meaningless without their parent, SET NULL only where a computed default
/// catches the fall.
///
/// Absent by design, arriving with their feature rounds: stacks, collections,
/// keywords, FTS, XMP cursors, duplicate detection, the long-tail metadata
/// store.
nonisolated enum CatalogSchema {
	static let v0 = """
	CREATE TABLE volumes (
	    id       TEXT PRIMARY KEY,
	    -- Identity ladder: filesystem UUID | 'smb://host/share' (case-folded) |
	    -- 'nfs://host/export'. NULL = not yet identified.
	    identity TEXT,                                                            -- [obs]
	    name     TEXT NOT NULL,                                                   -- [jdg] seeded from the volume label
	    kind     TEXT NOT NULL CHECK (kind IN ('internal', 'external', 'network')) -- [obs]
	);

	-- Found-or-created by identity; SQLite keeps NULLs distinct in unique
	-- indexes, so unidentified volumes never collide.
	CREATE UNIQUE INDEX idx_volumes_identity ON volumes(identity);

	CREATE TABLE folders (
	    id        TEXT PRIMARY KEY,
	    volume_id TEXT NOT NULL REFERENCES volumes(id) ON DELETE RESTRICT,
	    parent_id TEXT REFERENCES folders(id) ON DELETE RESTRICT,  -- NULL = a tracked root
	    name      TEXT NOT NULL,                                   -- [obs] on-disk bytes
	    name_key  TEXT NOT NULL,                                   -- [der] NFC(name): compare keys, open bytes
	    -- Tracked roots only: volume-relative path. Relocation repair = update
	    -- this one value; descendant paths recompose through the tree.
	    root_path TEXT,                                            -- [jdg]
	    CHECK ((parent_id IS NULL) = (root_path IS NOT NULL))
	);

	CREATE UNIQUE INDEX idx_folders_root  ON folders(volume_id, root_path) WHERE parent_id IS NULL;
	CREATE UNIQUE INDEX idx_folders_child ON folders(parent_id, name_key)  WHERE parent_id IS NOT NULL;

	CREATE TABLE imports (
	    id          TEXT PRIMARY KEY,
	    -- NULL = a file-picked import. Membership is derived either way
	    -- (files.import_id); counts are computed, never recorded.
	    folder_id   TEXT REFERENCES folders(id) ON DELETE RESTRICT,
	    started_at  TEXT NOT NULL,
	    finished_at TEXT  -- NULL = never completed: the interrupted-import signal
	);

	-- assets and files reference each other; SQLite resolves foreign keys at
	-- write time, not CREATE time, so the cycle is legal.
	CREATE TABLE assets (
	    id   TEXT PRIMARY KEY,
	    kind TEXT NOT NULL,  -- [der] open set (image/video/audio/…) via the kind registry
	    -- Judgments: NULL = unrated / unflagged; 0 is not a rating.
	    rating INTEGER CHECK (rating BETWEEN 1 AND 5),             -- [jdg]
	    flag   TEXT CHECK (flag IN ('pick', 'reject')),            -- [jdg]
	    -- Override only; the default election is computed at read, never stored.
	    representative_file_id TEXT REFERENCES files(id) ON DELETE SET NULL  -- [jdg]
	);

	CREATE TABLE files (
	    id             TEXT PRIMARY KEY,
	    folder_id      TEXT NOT NULL REFERENCES folders(id) ON DELETE RESTRICT,
	    asset_id       TEXT REFERENCES assets(id) ON DELETE RESTRICT,
	    import_id      TEXT NOT NULL REFERENCES imports(id) ON DELETE RESTRICT,  -- [obs] the import that added this file
	    name           TEXT NOT NULL,     -- [obs] on-disk bytes, extension included
	    name_key       TEXT NOT NULL,     -- [der] NFC(name): identity compare, case preserved
	    stem           TEXT NOT NULL,     -- [der] lowercase+NFC, final-dot rule; the formation collision key
	    extension      TEXT NOT NULL,     -- [der] lowercase+NFC final-dot tail; '' = none
	    kind           TEXT NOT NULL,     -- [der] filetype-registry verdict ('sidecar' included)
	    size_bytes     INTEGER NOT NULL,  -- [obs] staleness gate, with modified_at
	    modified_at    TEXT NOT NULL,     -- [obs] disk mtime; ±2s tolerance applies at compare, never at storage
	    content_hash   TEXT,              -- [obs] partial hash of the first 64KB
	    missing        INTEGER NOT NULL DEFAULT 0 CHECK (missing IN (0, 1)),  -- [obs] rescan verdict
	    metadata       TEXT,              -- [obs] JSON, field-catalog keys; promotion mints generated columns
	    thumbnail_at   TEXT,              -- [der] NULL = pending; the missing artifact IS the queue
	    formation_rule TEXT,              -- [der] the named rule that admitted this file to its asset; NULL = manual
	    -- Universal minting at statement time: a non-sidecar file cannot commit
	    -- unassetted, so formation must run inside the import's own transaction.
	    CHECK (kind = 'sidecar' OR asset_id IS NOT NULL)
	);

	CREATE UNIQUE INDEX idx_files_identity ON files(folder_id, name_key);
	CREATE INDEX idx_files_asset  ON files(asset_id);
	CREATE INDEX idx_files_stem   ON files(stem);  -- catalog-wide formation collision address
	CREATE INDEX idx_files_import ON files(import_id);

	-- The import DLQ: pre-identity failures, path-keyed, so a file that never
	-- became a row still leaves visible residue.
	CREATE TABLE import_errors (
	    id          TEXT PRIMARY KEY,
	    import_id   TEXT NOT NULL REFERENCES imports(id) ON DELETE CASCADE,
	    path        TEXT NOT NULL,  -- volume-relative walked path
	    reason_code TEXT NOT NULL,  -- machine taxonomy: 'read_failed', 'permission_denied', …
	    message     TEXT NOT NULL,
	    attempts    INTEGER NOT NULL DEFAULT 1
	);

	CREATE INDEX idx_import_errors_import ON import_errors(import_id);

	-- Post-identity failures, (file, task)-keyed. Absence is ambiguous: a NULL
	-- artifact marker means not-yet UNLESS a row here says tried-and-failed.
	CREATE TABLE file_errors (
	    file_id     TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	    task        TEXT NOT NULL,  -- 'thumbnail' is v0's only member
	    reason_code TEXT NOT NULL,  -- 'decode_failed' (terminal) vs retryable classes
	    message     TEXT NOT NULL,
	    attempts    INTEGER NOT NULL DEFAULT 1,
	    PRIMARY KEY (file_id, task)
	);
	"""
}
