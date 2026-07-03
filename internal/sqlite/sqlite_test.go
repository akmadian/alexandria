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

func TestMigration_FTS5Works(t *testing.T) {
	db := testutil.NewTestDB(t)
	_, err := db.Exec("INSERT INTO assets_fts (asset_id, filename, tags, note) VALUES ('a1', 'sunset.jpg', 'landscape golden-hour', 'beautiful sunset')")
	if err != nil {
		t.Fatalf("FTS5 insert failed: %v", err)
	}
	var id string
	err = db.QueryRow("SELECT asset_id FROM assets_fts WHERE assets_fts MATCH 'sunset'").Scan(&id)
	if err != nil {
		t.Fatalf("FTS5 query failed: %v", err)
	}
	if id != "a1" {
		t.Fatalf("got %q, want a1", id)
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
	s := &domain.Source{
		ID: "s1", Name: "Photos", Kind: domain.SourceKindExternalDrive,
		BasePath: "/Volumes/Photos", FilesystemUUID: &fsUUID,
		ScanRecursively: true, Status: domain.SourceStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, s); err != nil {
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

func TestSourceRepo_UpdateStatus(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.SourceRepo{DB: db}
	ctx := context.Background()
	testutil.NewTestSource(t, db, "drive")

	if err := repo.UpdateStatus(ctx, "src-drive", domain.SourceStatusOffline); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got, _ := repo.Get(ctx, "src-drive")
	if got.Status != domain.SourceStatusOffline {
		t.Fatalf("got status %q, want offline", got.Status)
	}
}

func TestSourceRepo_FindByFilesystemUUID(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.SourceRepo{DB: db}
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	fsUUID := "fs-abc"
	repo.Create(ctx, &domain.Source{
		ID: "s1", Name: "ext", Kind: domain.SourceKindExternalDrive,
		BasePath: "/mnt/ext", FilesystemUUID: &fsUUID,
		ScanRecursively: true, Status: domain.SourceStatusActive,
		CreatedAt: now, UpdatedAt: now,
	})

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
	rating := 4
	cl := domain.ColorLabelBlue
	a := &domain.Asset{
		ID: "a1", SourceID: src.ID, RelativePath: "vacation/beach.jpg",
		FileStatus: domain.FileStatusOnline, Filename: "beach.jpg",
		Extension: "jpg", MIMEType: "image/jpeg", FileType: domain.FileTypeImage,
		SizeBytes: 4096, MTime: now, PartialHash: "hash123",
		Rating: &rating, ColorLabel: &cl,
		IngestedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("create: %v", err)
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

	if err := repo.SoftDelete(ctx, "asset-photo.jpg"); err != nil {
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

func TestAssetRepo_ListWithFilters(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "a.jpg")
	testutil.NewTestAsset(t, db, src.ID, "b.jpg")
	testutil.NewTestAsset(t, db, src.ID, "c.jpg")

	// Set one to deleted
	repo.SoftDelete(ctx, "asset-c.jpg")

	// List non-deleted (default)
	assets, err := repo.List(ctx, catalog.AssetFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("got %d, want 2", len(assets))
	}

	// List including deleted
	assets, err = repo.List(ctx, catalog.AssetFilter{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(assets) != 3 {
		t.Fatalf("got %d, want 3", len(assets))
	}

	// Filter by file type
	assets, err = repo.List(ctx, catalog.AssetFilter{FileTypes: []domain.FileType{domain.FileTypeVideo}})
	if err != nil {
		t.Fatalf("list video: %v", err)
	}
	if len(assets) != 0 {
		t.Fatalf("got %d, want 0", len(assets))
	}
}

func TestAssetRepo_Patch_SparseSetsOnlyRequestedFields(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	// Insert three assets with different initial ratings
	for _, name := range []string{"r1.jpg", "r2.jpg", "r3.jpg"} {
		testutil.NewTestAsset(t, db, src.ID, name)
	}
	// Set initial ratings: r1=1, r2=3, r3=5
	repo.Patch(ctx, "asset-r1.jpg", catalog.AssetPatch{Rating: domain.SetOpt(1)})
	repo.Patch(ctx, "asset-r2.jpg", catalog.AssetPatch{Rating: domain.SetOpt(3)})
	repo.Patch(ctx, "asset-r3.jpg", catalog.AssetPatch{Rating: domain.SetOpt(5)})

	// Now BulkPatch all to rating=4 — only rating should change
	err := repo.BulkPatch(ctx,
		[]string{"asset-r1.jpg", "asset-r2.jpg", "asset-r3.jpg"},
		catalog.AssetPatch{Rating: domain.SetOpt(4)})
	if err != nil {
		t.Fatalf("bulk patch: %v", err)
	}

	for _, name := range []string{"r1.jpg", "r2.jpg", "r3.jpg"} {
		got, _ := repo.Get(ctx, "asset-"+name)
		if got.Rating == nil || *got.Rating != 4 {
			t.Errorf("asset-%s: rating=%v, want 4", name, got.Rating)
		}
		// FileStatus should be unchanged
		if got.FileStatus != domain.FileStatusOnline {
			t.Errorf("asset-%s: file_status=%q, want online", name, got.FileStatus)
		}
	}
}

func TestAssetRepo_Patch_ClearField(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")
	testutil.NewTestAsset(t, db, src.ID, "x.jpg")

	// Set a rating
	repo.Patch(ctx, "asset-x.jpg", catalog.AssetPatch{Rating: domain.SetOpt(3)})
	got, _ := repo.Get(ctx, "asset-x.jpg")
	if got.Rating == nil || *got.Rating != 3 {
		t.Fatalf("setup: rating=%v", got.Rating)
	}

	// Clear it
	repo.Patch(ctx, "asset-x.jpg", catalog.AssetPatch{Rating: domain.ClearOpt[int]()})
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

func TestAssetRepo_MarkOfflineOnlineBySource(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "drive")
	testutil.NewTestAsset(t, db, src.ID, "a.jpg")
	testutil.NewTestAsset(t, db, src.ID, "b.jpg")

	if err := repo.MarkOfflineBySource(ctx, src.ID); err != nil {
		t.Fatalf("mark offline: %v", err)
	}
	got, _ := repo.Get(ctx, "asset-a.jpg")
	if got.FileStatus != domain.FileStatusOffline {
		t.Fatalf("got %q, want offline", got.FileStatus)
	}

	if err := repo.MarkOnlineBySource(ctx, src.ID); err != nil {
		t.Fatalf("mark online: %v", err)
	}
	got, _ = repo.Get(ctx, "asset-a.jpg")
	if got.FileStatus != domain.FileStatusOnline {
		t.Fatalf("got %q, want online", got.FileStatus)
	}
}
