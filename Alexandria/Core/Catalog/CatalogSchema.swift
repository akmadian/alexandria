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
/// Absent by design, arriving with their feature rounds: stacks, keywords,
/// FTS, XMP cursors, duplicate detection, the long-tail metadata store.
nonisolated enum CatalogSchema {
	static let v0 = """
	CREATE TABLE volumes (
	    id       TEXT PRIMARY KEY, -- [] Who the volume is to the catalog
	    -- Identity ladder: filesystem UUID | 'smb://host/share' (case-folded) |
	    -- 'nfs://host/export'. NULL = not yet identified.
	    identity TEXT,                                                            -- [obs] How the catalog recognizes the volume when it shows up
	    name     TEXT NOT NULL,                                                   -- [jdg] seeded from the volume label
	    kind     TEXT NOT NULL CHECK (kind IN ('local', 'external', 'network')) -- [obs]
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
	    finished_at TEXT,  -- when the run ended; chronology only, outcome carries the semantics
		outcome     TEXT CHECK (outcome IN ('completed', 'canceled', 'failed')) -- NULL = still running; NULL with no live run = interrupted
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
	    -- NULL = formation pending (formation round, 2026-09-11): asset
	    -- formation is a distinct pass after commit, the thumbnail_at idiom.
	    -- Only orphan sidecars stay pending past their import.
	    asset_id       TEXT REFERENCES assets(id) ON DELETE RESTRICT,
	    import_id      TEXT NOT NULL REFERENCES imports(id) ON DELETE RESTRICT,  -- [obs] the import that added this file
	    name           TEXT NOT NULL,     -- [obs] on-disk bytes, extension included
	    name_key       TEXT NOT NULL,     -- [der] NFC(name): identity compare, case preserved
	    file_stem           TEXT NOT NULL,     -- [der] lowercase+NFC, final-dot rule; the formation collision key
	    file_extension      TEXT NOT NULL,     -- [der] lowercase+NFC final-dot tail; '' = none
	    kind           TEXT NOT NULL,     -- [der] filetype-registry verdict ('sidecar' included)
	    size_bytes     INTEGER NOT NULL,  -- [obs] staleness gate, with modified_at
	    modified_at    TEXT NOT NULL,     -- [obs] disk mtime; ±2s tolerance applies at compare, never at storage
	    content_hash   TEXT,              -- [obs] partial hash of the first 64KB
	    missing        INTEGER NOT NULL DEFAULT 0 CHECK (missing IN (0, 1)),  -- [obs] rescan verdict
	    metadata       TEXT,              -- [obs] sectioned facet JSON (FileMetadata); promotion mints generated columns
	    -- [der] The facet-roster version the metadata blob was written under
	    -- (metadata round, 2026-09-17): the re-extraction worklist key — a
	    -- roster bump queries `metadata_version < ?` for files needing a
	    -- re-read, the thumbnail_at idiom for a marker the blob itself can't
	    -- carry (an old blob and an evidence-less new one are byte-alike).
	    -- 0 = no extraction attempted or pre-facet writer.
	    metadata_version INTEGER NOT NULL DEFAULT 0,
	    -- [der] The capture-time sort key (grid sorting round, 2026-09-15): the
	    -- first metadata field promoted per the blob's design (database.md) —
	    -- capture time lifted out of the JSON at its facet path (metadata
	    -- round, 2026-09-17), COALESCEd to disk mtime so the key is total
	    -- (every file has an mtime; NOT NULL). VIRTUAL: no row storage, the
	    -- value is materialized by its index — so json_extract runs at WRITE
	    -- (index maintenance), and a non-JSON metadata blob would throw on
	    -- INSERT. Safe while databaseJSON() is the only writer: the column
	    -- must hold valid JSON or NULL — nothing else parses ('' is malformed
	    -- JSON and would fail the INSERT); a future raw-blob metadata lane
	    -- must keep that invariant. Blob dates share catalogDateFormatter's ms ISO
	    -- 8601 with the columns, so captured and mtime values sort on one
	    -- scale with no cross-format fuzz.
	    capture_sort   TEXT GENERATED ALWAYS AS (COALESCE(json_extract(metadata, '$.capture.captured_at'), modified_at)) VIRTUAL,
	    thumbnail_at   TEXT,              -- [der] NULL = pending; the missing artifact IS the queue
	    -- [der] the rule that admitted this file to its asset — a historical
	    -- fact, never "the rule that would match now". NULL = not yet formed
	    -- (or a future manual admission).
	    formation_rule TEXT
	);

	CREATE UNIQUE INDEX idx_files_identity ON files(folder_id, name_key);
	CREATE INDEX idx_files_asset  ON files(asset_id);
	CREATE INDEX idx_files_stem   ON files(file_stem);  -- catalog-wide formation collision address
	CREATE INDEX idx_files_import ON files(import_id);
	-- The whole-library files lens rides this: ORDER BY capture_sort is
	-- index-served there, not a scan-and-sort. A narrowed files source (WHERE
	-- import_id = ? … ORDER BY capture_sort) is filter-then-sort, and the asset
	-- lens sorts on a computed join key the index can't reach (grid sorting
	-- round, 2026-09-15; see WorkingSetQuery's PERF note).
	CREATE INDEX idx_files_capture_sort ON files(capture_sort);
	-- The thumbnail worklist (thumbnails round, ratified 2026-09-11): pending
	-- rows only, ordered — each drain pull is O(log n + batch) and the index
	-- empties as stamps land. Errored files stay in it (thumbnail_at NULL);
	-- the worklist's NOT EXISTS rejects them per pull, fine at realistic
	-- error counts.
	CREATE INDEX idx_files_thumbnail_pending ON files(import_id, id) WHERE thumbnail_at IS NULL;

	-- Collections (collections round, 2026-09-12): authored, named, nestable
	-- sets of assets — the user's work product, so every column here and in
	-- collection_members is judgment-class. ONE noun: a collection holds
	-- member assets AND child collections alike; there is no set/group type
	-- (the LrC/Photos container wall is documented user pain). Names are
	-- free-form and may duplicate, siblings included — identity is the id,
	-- and nothing looks a collection up by name; emptiness is rejected at
	-- the verb, not here. A smart collection is a predicate on this same
	-- table (smart-collection round, 2026-09-18), never a second kind of
	-- thing.
	CREATE TABLE collections (
	    id        TEXT PRIMARY KEY,
	    -- NULL = a root (multiple roots allowed). RESTRICT: deleting a
	    -- subtree is a designed verb walking bottom-up, never a cascade.
	    parent_id TEXT REFERENCES collections(id) ON DELETE RESTRICT,
	    name      TEXT NOT NULL,  -- [jdg] authored, as typed; never an identity
	    -- [jdg] NULL = manual. Non-NULL = a smart collection: the versioned
	    -- filter envelope (FilterGroup.serialized()), decoded lazily per use
	    -- so one corrupt predicate disables one collection, never a fetch
	    -- that merely lists rows (sidebar, tree assembly stay row-blind).
	    predicate TEXT
	);

	-- The subtree walk's join key (the folder tree gets this via its child
	-- identity index; collections have no such index, so it's explicit).
	CREATE INDEX idx_collections_parent ON collections(parent_id);

	-- Manual membership ONLY, by ruling: a smart collection computes
	-- membership from its predicate and never writes here, and takes no
	-- manual adds (the verbs refuse). The composite key makes duplicate
	-- membership structurally
	-- impossible (bulk add is INSERT OR IGNORE, idempotent). CASCADE both
	-- ways: a membership is meaningless without either parent.
	CREATE TABLE collection_members (
	    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
	    asset_id      TEXT NOT NULL REFERENCES assets(id)      ON DELETE CASCADE,
	    -- [jdg] the manual order: a fractional index (base-62 TEXT, binary
	    -- compare), minted append-at-end on add so added-order IS the manual
	    -- order until the first drag — there is no unordered state. An
	    -- insert-between touches one row, never the tail (the LrC renumber
	    -- lesson); floats were rejected outright (LrC's 52-reorder mantissa
	    -- bug). See _design/collections.md.
	    order_key     TEXT NOT NULL,
	    PRIMARY KEY (collection_id, asset_id)
	);

	-- The ordered read rides this index — and UNIQUE turns a key collision
	-- (a minting bug under the single writer) into a loud transaction
	-- failure instead of LrC's silent identical-position rot.
	CREATE UNIQUE INDEX idx_collection_members_order ON collection_members(collection_id, order_key);
	-- The reverse verb: which collections hold this asset.
	CREATE INDEX idx_collection_members_asset ON collection_members(asset_id);

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
	    task        TEXT NOT NULL,  -- v0 members: 'thumbnail', 'metadata'
	    reason_code TEXT NOT NULL,  -- 'decode_failed' (terminal) vs retryable classes
	    message     TEXT NOT NULL,
	    attempts    INTEGER NOT NULL DEFAULT 1,
	    PRIMARY KEY (file_id, task)
	);
	"""
}
