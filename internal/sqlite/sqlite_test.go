package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
)

// FTS is populated by the assets triggers, not by direct writes: insert an asset
// and it must become searchable by filename.
func TestMigration_FTSTriggerIndexesAssets(t *testing.T) {
	db := testutil.NewTestDB(t)
	src := testutil.NewTestSource(t, db, "s")
	testutil.NewTestAsset(t, db, src.ID, "sunset-beach.jpg")

	var id string
	err := db.QueryRow("SELECT asset_id FROM assets_fts WHERE assets_fts MATCH 'sunset'").Scan(&id)
	if err != nil {
		t.Fatalf("FTS query failed: %v", err)
	}
	if id != "asset-sunset-beach.jpg" {
		t.Fatalf("got %q, want asset-sunset-beach.jpg", id)
	}
}

func TestMigration_ForeignKeysEnforced(t *testing.T) {
	db := testutil.NewTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO assets
		(id, source_id, relative_path, file_status, filename, extension, mime_type, file_type,
		 size_bytes, mtime, partial_hash, ingested_at, updated_at)
		VALUES ('a1', 'nonexistent-source', 'f.jpg', 'online', 'f.jpg', 'jpg', 'image/jpeg', 'image',
		 1024, ?, 'hash', ?, ?)`, now, now, now)
	if err == nil {
		t.Fatal("expected foreign key violation, got nil")
	}
}

// --- Source repository tests ---

func TestSourceRepo_CreateAndGet(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.SourceRepo{DB: db}
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	fsUUID := "uuid-123"
	source := &domain.Source{
		ID: "s1", Name: "Photos", Kind: domain.SourceKindExternalDrive,
		BasePath: "/Volumes/Photos", FilesystemUUID: &fsUUID,
		ScanRecursively: true, Enabled: true, Connectivity: domain.SourceOnline,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, source); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Photos" || got.Kind != domain.SourceKindExternalDrive {
		t.Fatalf("got %+v", got)
	}
	if got.FilesystemUUID == nil || *got.FilesystemUUID != "uuid-123" {
		t.Fatalf("filesystem_uuid: got %v", got.FilesystemUUID)
	}
	if !got.Enabled || got.Connectivity != domain.SourceOnline {
		t.Fatalf("enabled/connectivity: got enabled=%v connectivity=%q", got.Enabled, got.Connectivity)
	}
}

func TestSourceRepo_GetNotFound(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.SourceRepo{DB: db}
	_, err := repo.Get(context.Background(), "nope")
	var nf *domain.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
}

func TestSourceRepo_List(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.SourceRepo{DB: db}
	ctx := context.Background()

	testutil.NewTestSource(t, db, "one")
	testutil.NewTestSource(t, db, "two")

	sources, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("got %d sources, want 2", len(sources))
	}
}

func TestSourceRepo_SetConnectivity(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.SourceRepo{DB: db}
	ctx := context.Background()
	testutil.NewTestSource(t, db, "drive")

	if err := repo.SetConnectivity(ctx, "src-drive", domain.SourceOffline); err != nil {
		t.Fatalf("set connectivity: %v", err)
	}
	got, _ := repo.Get(ctx, "src-drive")
	if got.Connectivity != domain.SourceOffline {
		t.Fatalf("got connectivity %q, want offline", got.Connectivity)
	}
	// Connectivity (observation) must not disturb Enabled (judgment).
	if !got.Enabled {
		t.Fatal("SetConnectivity clobbered Enabled")
	}
}

func TestSourceRepo_FindByFilesystemUUID(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.SourceRepo{DB: db}
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	fsUUID := "fs-abc"
	if err := repo.Create(ctx, &domain.Source{
		ID: "s1", Name: "ext", Kind: domain.SourceKindExternalDrive,
		BasePath: "/mnt/ext", FilesystemUUID: &fsUUID,
		ScanRecursively: true, Enabled: true, Connectivity: domain.SourceOnline,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.FindByFilesystemUUID(ctx, "fs-abc")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil || got.ID != "s1" {
		t.Fatalf("expected s1, got %v", got)
	}

	got, err = repo.FindByFilesystemUUID(ctx, "no-such")
	if err != nil {
		t.Fatalf("find miss: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

// --- Asset repository tests ---

func TestAssetRepo_CreateAndGet(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()

	src := testutil.NewTestSource(t, db, "photos")
	now := time.Now().UTC().Truncate(time.Second)
	asset := &domain.Asset{
		ID: "a1", SourceID: src.ID, RelativePath: "vacation/beach.jpg",
		FileStatus: domain.FileStatusOnline, Filename: "beach.jpg",
		Extension: "jpg", MIMEType: "image/jpeg", FileType: domain.FileTypeImage,
		SizeBytes: 4096, MTime: now, PartialHash: "hash123",
		IngestedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, asset); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Judgments are set through the judgment writer, never at minting.
	if err := repo.ApplyTriagePatch(ctx, []string{"a1"},
		catalog.TriagePatch{Rating: domain.SetOpt(4), ColorLabel: domain.SetOpt(domain.ColorLabelBlue)}); err != nil {
		t.Fatalf("triage: %v", err)
	}

	got, err := repo.Get(ctx, "a1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Filename != "beach.jpg" {
		t.Fatalf("filename: got %q", got.Filename)
	}
	if got.Rating == nil || *got.Rating != 4 {
		t.Fatalf("rating: got %v", got.Rating)
	}
	if got.ColorLabel == nil || *got.ColorLabel != domain.ColorLabelBlue {
		t.Fatalf("color_label: got %v", got.ColorLabel)
	}
}

// Minting is observation-only: Create must reject an asset carrying judgment.
func TestAssetRepo_Create_RejectsJudgmentFields(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")
	now := time.Now().UTC()
	rating := 3
	a := &domain.Asset{
		ID: "bad", SourceID: src.ID, RelativePath: "x.jpg", FileStatus: domain.FileStatusOnline,
		Filename: "x.jpg", Extension: "jpg", MIMEType: "image/jpeg", FileType: domain.FileTypeImage,
		SizeBytes: 1, MTime: now, PartialHash: "h", Rating: &rating, IngestedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, a); err == nil {
		t.Fatal("Create must reject a minted asset that carries a rating")
	}
}

func TestAssetRepo_GetNotFound(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	_, err := repo.Get(context.Background(), "nope")
	var nf *domain.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
}

func TestAssetRepo_SoftDelete(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()

	src := testutil.NewTestSource(t, db, "s")
	testutil.NewTestAsset(t, db, src.ID, "photo.jpg")

	if err := repo.SoftDelete(ctx, []string{"asset-photo.jpg"}); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	got, err := repo.Get(ctx, "asset-photo.jpg")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if !got.IsDeleted {
		t.Fatal("expected is_deleted=true")
	}
	if got.DeletedAt == nil {
		t.Fatal("expected deleted_at to be set")
	}
}

func TestAssetRepo_ApplyTriagePatch_BulkSetsOnlyJudgment(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	for _, name := range []string{"r1.jpg", "r2.jpg", "r3.jpg"} {
		testutil.NewTestAsset(t, db, src.ID, name)
	}
	ids := []string{"asset-r1.jpg", "asset-r2.jpg", "asset-r3.jpg"}
	if err := repo.ApplyTriagePatch(ctx, ids, catalog.TriagePatch{Rating: domain.SetOpt(4)}); err != nil {
		t.Fatalf("bulk triage: %v", err)
	}

	for _, id := range ids {
		got, _ := repo.Get(ctx, id)
		if got.Rating == nil || *got.Rating != 4 {
			t.Errorf("%s: rating=%v, want 4", id, got.Rating)
		}
		// A judgment write must not disturb observation columns.
		if got.FileStatus != domain.FileStatusOnline {
			t.Errorf("%s: file_status=%q, want online", id, got.FileStatus)
		}
	}
}

func TestAssetRepo_ApplyTriagePatch_ClearField(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")
	testutil.NewTestAsset(t, db, src.ID, "x.jpg")

	if err := repo.ApplyTriagePatch(ctx, []string{"asset-x.jpg"}, catalog.TriagePatch{Rating: domain.SetOpt(3)}); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(ctx, "asset-x.jpg")
	if got.Rating == nil || *got.Rating != 3 {
		t.Fatalf("setup: rating=%v", got.Rating)
	}

	if err := repo.ApplyTriagePatch(ctx, []string{"asset-x.jpg"}, catalog.TriagePatch{Rating: domain.ClearOpt[int]()}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(ctx, "asset-x.jpg")
	if got.Rating != nil {
		t.Fatalf("after clear: rating=%v, want nil", got.Rating)
	}
}

func TestAssetRepo_FindByHash(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")
	testutil.NewTestAsset(t, db, src.ID, "dup.jpg")

	got, err := repo.FindByHash(ctx, "testhash-dup.jpg", 1024)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil || got.ID != "asset-dup.jpg" {
		t.Fatalf("expected asset-dup.jpg, got %v", got)
	}

	// Miss
	got, err = repo.FindByHash(ctx, "nohash", 999)
	if err != nil {
		t.Fatalf("find miss: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestAssetRepo_FindBySourcePath(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")
	testutil.NewTestAsset(t, db, src.ID, "sub/photo.jpg")

	got, err := repo.FindBySourcePath(ctx, src.ID, "sub/photo.jpg")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil || got.ID != "asset-sub/photo.jpg" {
		t.Fatalf("expected asset, got %v", got)
	}
}

func TestAssetRepo_MarkConnectivityBySource(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "drive")
	testutil.NewTestAsset(t, db, src.ID, "a.jpg")
	testutil.NewTestAsset(t, db, src.ID, "b.jpg")

	if err := repo.MarkConnectivityBySource(ctx, src.ID, false); err != nil {
		t.Fatalf("mark offline: %v", err)
	}
	got, _ := repo.Get(ctx, "asset-a.jpg")
	if got.FileStatus != domain.FileStatusOffline {
		t.Fatalf("got %q, want offline", got.FileStatus)
	}

	if err := repo.MarkConnectivityBySource(ctx, src.ID, true); err != nil {
		t.Fatalf("mark online: %v", err)
	}
	got, _ = repo.Get(ctx, "asset-a.jpg")
	if got.FileStatus != domain.FileStatusOnline {
		t.Fatalf("got %q, want online", got.FileStatus)
	}
}

// --- Schema constraint / trap tests (impl/01 acceptance) ---

// The unique (source_id, relative_path) index is partial on is_deleted=0, so a
// soft-deleted asset must not block re-importing a fresh asset at the same path.
func TestSchema_SoftDeleteThenReimportSamePath(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	a1 := testutil.NewTestAsset(t, db, src.ID, "photo.jpg")
	if err := repo.SoftDelete(ctx, []string{a1.ID}); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	now := time.Now().UTC()
	reimported := &domain.Asset{
		ID: domain.NewID(), SourceID: src.ID, RelativePath: "photo.jpg",
		FileStatus: domain.FileStatusOnline, Filename: "photo.jpg", Extension: "jpg",
		MIMEType: "image/jpeg", FileType: domain.FileTypeImage, SizeBytes: 2048,
		MTime: now, PartialHash: "newhash", IngestedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, reimported); err != nil {
		t.Fatalf("re-import at same path after soft delete should succeed: %v", err)
	}
}

// SQLite treats NULLs as distinct in a UNIQUE index; the IFNULL(parent_id,”)
// expression index must still reject two root tags with the same slug.
func TestSchema_RootTagSlugConflict(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	insert := func(id string) error {
		_, err := db.ExecContext(ctx,
			`INSERT INTO tags (id, name, slug, parent_id, path, created_at) VALUES (?, 'Travel', 'travel', NULL, '/'||?||'/', ?)`,
			id, id, now)
		return err
	}
	if err := insert("t1"); err != nil {
		t.Fatalf("first root tag: %v", err)
	}
	if err := insert("t2"); err == nil {
		t.Fatal("expected slug conflict on second root tag, got nil")
	}
}

func TestSchema_FKCascadeOnTagDelete(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")
	asset := testutil.NewTestAsset(t, db, src.ID, "x.jpg")
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := db.ExecContext(ctx, `INSERT INTO tags (id, name, slug, path, created_at) VALUES ('tag1', 'Beach', 'beach', '/tag1/', ?)`, now); err != nil {
		t.Fatalf("tag: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO asset_tags (asset_id, tag_id, source, created_at) VALUES (?, 'tag1', 'user', ?)`, asset.ID, now); err != nil {
		t.Fatalf("asset_tag: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM tags WHERE id = 'tag1'`); err != nil {
		t.Fatalf("delete tag: %v", err)
	}

	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM asset_tags WHERE tag_id = 'tag1'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected CASCADE to remove asset_tags, found %d", n)
	}
}

// RebuildFTS composes the tags column from the asset_tags join — the one FTS
// column triggers can't maintain. Before rebuild the tags cell is empty; after,
// the asset is searchable by tag name.
func TestRebuildFTS_IncludesTags(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")
	asset := testutil.NewTestAsset(t, db, src.ID, "img.jpg")
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := db.ExecContext(ctx, `INSERT INTO tags (id, name, slug, path, created_at) VALUES ('t1', 'Landscape', 'landscape', '/t1/', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO asset_tags (asset_id, tag_id, source, created_at) VALUES (?, 't1', 'user', ?)`, asset.ID, now); err != nil {
		t.Fatal(err)
	}

	if err := sqlite.RebuildFTS(ctx, db); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	var id string
	if err := db.QueryRowContext(ctx, `SELECT asset_id FROM assets_fts WHERE assets_fts MATCH 'landscape'`).Scan(&id); err != nil {
		t.Fatalf("tag search after rebuild: %v", err)
	}
	if id != asset.ID {
		t.Fatalf("got %q, want %q", id, asset.ID)
	}
}

// --- Writer-class invariants (impl/02 acceptance) ---

// The reimport clobber bug, made impossible: an observation write (ApplyFilePatch)
// changes file facts but must leave rating AND judgment_modified_at untouched.
func TestAssetRepo_ApplyFilePatch_PreservesJudgment(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")
	testutil.NewTestAsset(t, db, src.ID, "p.jpg")

	// User rates it (judgment write → stamps judgment_modified_at).
	if err := repo.ApplyTriagePatch(ctx, []string{"asset-p.jpg"}, catalog.TriagePatch{Rating: domain.SetOpt(5)}); err != nil {
		t.Fatalf("triage: %v", err)
	}
	before, _ := repo.Get(ctx, "asset-p.jpg")
	if before.Rating == nil || *before.Rating != 5 || before.JudgmentModifiedAt == nil {
		t.Fatalf("setup: rating=%v judgmentModifiedAt=%v", before.Rating, before.JudgmentModifiedAt)
	}

	// A reimport changes file facts.
	const newSize = int64(9999)
	if err := repo.ApplyFilePatch(ctx, "asset-p.jpg", &catalog.FilePatch{
		Filename: "p.jpg", Extension: "jpg", MIMEType: "image/jpeg",
		FileType: domain.FileTypeImage, SizeBytes: newSize,
		MTime: time.Now().UTC(), PartialHash: "newhash", FileStatus: domain.FileStatusOnline,
	}); err != nil {
		t.Fatalf("file patch: %v", err)
	}

	after, _ := repo.Get(ctx, "asset-p.jpg")
	if after.SizeBytes != newSize {
		t.Fatalf("observation not updated: size=%d", after.SizeBytes)
	}
	if after.Rating == nil || *after.Rating != 5 {
		t.Fatalf("reimport clobbered rating: %v", after.Rating)
	}
	if after.JudgmentModifiedAt == nil || !after.JudgmentModifiedAt.Equal(*before.JudgmentModifiedAt) {
		t.Fatalf("reimport must not touch judgment_modified_at: before=%v after=%v",
			before.JudgmentModifiedAt, after.JudgmentModifiedAt)
	}
}

// The XMP oscillator guard: ApplyXMPInbound applies judgment VALUES but must not
// bump judgment_modified_at, whereas a user triage does.
func TestAssetRepo_XMPInbound_DoesNotBumpJudgmentModified(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")
	testutil.NewTestAsset(t, db, src.ID, "x.jpg")

	a0, _ := repo.Get(ctx, "asset-x.jpg")
	if a0.JudgmentModifiedAt != nil {
		t.Fatalf("fresh asset already has judgment_modified_at: %v", a0.JudgmentModifiedAt)
	}

	if err := repo.ApplyXMPInbound(ctx, "asset-x.jpg",
		catalog.TriagePatch{Rating: domain.SetOpt(3)}, time.Now().UTC(), "xmphash1"); err != nil {
		t.Fatalf("xmp inbound: %v", err)
	}
	asset, _ := repo.Get(ctx, "asset-x.jpg")
	if asset.Rating == nil || *asset.Rating != 3 {
		t.Fatalf("xmp value not applied: %v", asset.Rating)
	}
	if asset.JudgmentModifiedAt != nil {
		t.Fatalf("xmp inbound must not bump judgment_modified_at, got %v", asset.JudgmentModifiedAt)
	}
	if asset.XMPHash == nil || *asset.XMPHash != "xmphash1" {
		t.Fatalf("xmp cursor not recorded: %v", asset.XMPHash)
	}

	if err := repo.ApplyTriagePatch(ctx, []string{"asset-x.jpg"}, catalog.TriagePatch{Flag: domain.SetOpt(domain.FlagPick)}); err != nil {
		t.Fatalf("triage: %v", err)
	}
	a2, _ := repo.Get(ctx, "asset-x.jpg")
	if a2.JudgmentModifiedAt == nil {
		t.Fatal("user triage must bump judgment_modified_at")
	}
}

// InTx rolls back every statement on a returned error (the multi-statement relink
// case: UpdatePath then fail).
func TestStore_InTxRollback(t *testing.T) {
	db := testutil.NewTestDB(t)
	store := sqlite.NewStore(db)
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")
	testutil.NewTestAsset(t, db, src.ID, "m.jpg")

	sentinel := errors.New("boom")
	err := store.InTx(ctx, func(r sqlite.Repos) error {
		if err := r.Assets.UpdatePath(ctx, "asset-m.jpg", src.ID, "moved.jpg"); err != nil {
			return err
		}
		return sentinel // force rollback after a successful write
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("InTx should surface fn's error, got %v", err)
	}
	got, _ := (&sqlite.AssetRepo{DB: db}).Get(ctx, "asset-m.jpg")
	if got.RelativePath != "m.jpg" {
		t.Fatalf("rollback failed: path=%q, want m.jpg", got.RelativePath)
	}
}
