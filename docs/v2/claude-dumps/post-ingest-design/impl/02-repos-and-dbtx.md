# impl/02 — DBTX Seam + Writer-Scoped Repositories (Blocker 2)

**Scope:** `internal/sqlite/`, `internal/catalog/`. **Blocked by:** impl/01. **Blocks:** impl/04.
**References:** D8, `03-data-model.md` §1.

## 1. The DBTX seam

Repos currently hold `*sql.DB` — nothing can run inside a transaction, so the pipeline's batched
writes (50/txn) and multi-statement actions (relink = 2 UPDATEs; duplicate = 2 INSERTs) are
non-atomic. Fix:

```go
// internal/sqlite/db.go
type DBTX interface {
    ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
    QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}
```

Both `*sql.DB` and `*sql.Tx` satisfy it. Repos hold `DBTX`. Provide a helper on the store root:

```go
func (s *Store) InTx(ctx context.Context, fn func(r Repos) error) error
// BEGIN IMMEDIATE; build Repos bound to the tx; fn; COMMIT/ROLLBACK.
```

`BEGIN IMMEDIATE` (not DEFERRED) — the writer goroutine is the only writer, but IMMEDIATE fails
fast on lock contention instead of failing at first write.

## 2. Writer-scoped interfaces (the classification, made structural)

Replace the single fat `AssetRepository` with class-scoped interfaces in `internal/catalog`:

```go
type AssetReader interface {
    Get(ctx, id) (*domain.Asset, error)
    List(ctx, AssetFilter) ([]*domain.Asset, error)
    FindByHash(ctx, hash string, size int64) (*domain.Asset, error)  // is_deleted=0 only
    FindBySourcePath(ctx, sourceID, relPath string) (*domain.Asset, error)
    ListKnownFiles(ctx, sourceID) (map[string]domain.FileStat, error)
    ListPathsStatus(ctx, sourceID) ([]PathStatus, error) // slim projection for reconcile (id, relPath, file_status)
}

// Observation writer — ingest/watcher/reconciler ONLY. No judgment columns reachable.
type AssetObservationWriter interface {
    Create(ctx, *domain.Asset) error                    // minting (judgment fields must be zero)
    ApplyFilePatch(ctx, id string, p FilePatch) error   // reimport: file facts + extracted metadata
    UpdatePath(ctx, id, sourceID, relPath string) error // relink
    SetFileStatus(ctx, id string, s domain.FileStatus) error
    MarkConnectivityBySource(ctx, sourceID string, online bool) error
}

// Judgment writer — user-action service ONLY. Bumps judgment_modified_at on every write.
type AssetJudgmentWriter interface {
    ApplyTriagePatch(ctx, ids []string, p TriagePatch) error // rating/label/flag/note; bulk-capable
    SoftDelete(ctx, ids []string) error
}

// Sync writer — XMP sync ONLY. May set judgment VALUES per conflict policy but NEVER bumps
// judgment_modified_at; owns xmp_* cursor columns.
type AssetSyncWriter interface {
    ApplyXMPInbound(ctx, id string, p TriagePatch, readAt time.Time, xmpHash string) error
    RecordXMPWritten(ctx, id string, writtenAt time.Time, xmpHash string) error
}

// Derived writer — jobs ONLY.
type AssetDerivedWriter interface {
    SetThumbnailAt(ctx, id string, t time.Time) error
}
```

`FilePatch` = filename/ext/mime/file_type/size/mtime/hash/file_status + extracted-metadata fields
(width…copyright, title, caption, extended-merge). `TriagePatch` = rating/label/flag/note using
`domain.Opt[T]`. The existing `AssetPatch` splits into these two; delete it.

**The invariant this buys:** `judgment_modified_at` is bumped in exactly one code path
(`ApplyTriagePatch`/`SoftDelete`) and physically unreachable from ingest. The XMP oscillator
(D15 loop level 2) and the reimport-clobber bug are now uncompilable.

One concrete implementation struct can satisfy all interfaces; the *scoping happens at injection*:
the Importer receives `AssetObservationWriter + AssetReader`, nothing else.

## 3. Repo fixes riding along (from the original audit)

- `List`: whitelist map for sort fields `{"captured": "captured_at", "added": "ingested_at",
  "rating": "rating", "filename": "filename", "size": "size_bytes"}`; reject unknown. Kills the
  ORDER-BY injection.
- `Create` must reject non-zero judgment fields (defense in depth; minting is observation-only).
- New `sidecar_files` repo: `UpsertObservation`, `DeleteByPath`, `ListByKey(source, dir, stem)`.
- New `import_sessions` repo: `Start`, `UpdateCounts`, `Finish`, `LogError` (attempts upsert:
  same session+path+stage increments `attempts`).
- `SetAssetTags` (tag repo): updates the asset's FTS tags text in the same tx (impl/01 §15).

## Acceptance
- Compile-time check: the importer package imports only Reader + ObservationWriter types.
- Test: `ApplyFilePatch` on an asset with rating=5 leaves rating and judgment_modified_at untouched.
- Test: `ApplyTriagePatch` bumps judgment_modified_at; `ApplyXMPInbound` does not.
- Test: InTx rollback on error leaves no partial writes (relink two-statement case).
- Test: List with sort field `"filename; DROP TABLE"` returns an error.
