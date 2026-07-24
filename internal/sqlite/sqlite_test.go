package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
)

// FTS is populated by the assets triggers, not by direct writes: insert an asset
// and it must become searchable by filename.
func TestMigration_FTSTriggerIndexesAssets(t *testing.T) {
	db := testutil.NewTestDB(t)
	src := testutil.NewTestVolume(t, db, "s")
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
		(id, volume_id, relative_path, path_key, file_status, filename, extension, mime_type, file_type,
		 size_bytes, mtime, partial_hash, ingested_at, updated_at)
		VALUES ('a1', 'nonexistent-volume', 'f.jpg', 'f.jpg', 'online', 'f.jpg', 'jpg', 'image/jpeg', 'image',
		 1024, ?, 'hash', ?, ?)`, now, now, now)
	if err == nil {
		t.Fatal("expected foreign key violation, got nil")
	}
}

// --- Volume repository tests ---

func TestVolumeRepo_CreateAndGet(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.VolumeRepo{DB: db}
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	fsUUID := "uuid-123"
	volume := &domain.Volume{
		ID: "v1", Name: "Photos", Kind: domain.VolumeKindExternalDrive,
		FilesystemUUID: &fsUUID, Connectivity: domain.VolumeOnline,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, volume); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.Get(ctx, "v1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Photos" || got.Kind != domain.VolumeKindExternalDrive {
		t.Fatalf("got %+v", got)
	}
	if got.FilesystemUUID == nil || *got.FilesystemUUID != "uuid-123" {
		t.Fatalf("filesystem_uuid: got %v", got.FilesystemUUID)
	}
	if got.Connectivity != domain.VolumeOnline {
		t.Fatalf("connectivity: got %q", got.Connectivity)
	}
}

func TestVolumeRepo_GetNotFound(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.VolumeRepo{DB: db}
	_, err := repo.Get(context.Background(), "nope")
	var nf *domain.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
}

func TestVolumeRepo_List(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.VolumeRepo{DB: db}
	ctx := context.Background()

	testutil.NewTestVolume(t, db, "one")
	testutil.NewTestVolume(t, db, "two")

	volumes, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(volumes) != 2 {
		t.Fatalf("got %d volumes, want 2", len(volumes))
	}
}

func TestVolumeRepo_SetConnectivity(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.VolumeRepo{DB: db}
	ctx := context.Background()
	testutil.NewTestVolume(t, db, "drive")

	if err := repo.SetConnectivity(ctx, "vol-drive", domain.VolumeOffline); err != nil {
		t.Fatalf("set connectivity: %v", err)
	}
	got, _ := repo.Get(ctx, "vol-drive")
	if got.Connectivity != domain.VolumeOffline {
		t.Fatalf("got connectivity %q, want offline", got.Connectivity)
	}
}

func TestVolumeRepo_FindByFilesystemUUID(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.VolumeRepo{DB: db}
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	fsUUID := "fs-abc"
	if err := repo.Create(ctx, &domain.Volume{
		ID: "v1", Name: "ext", Kind: domain.VolumeKindExternalDrive,
		FilesystemUUID: &fsUUID, Connectivity: domain.VolumeOnline,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.FindByFilesystemUUID(ctx, "fs-abc")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil || got.ID != "v1" {
		t.Fatalf("expected v1, got %v", got)
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

	src := testutil.NewTestVolume(t, db, "photos")
	now := time.Now().UTC().Truncate(time.Second)
	asset := &domain.Asset{
		ID: "a1", VolumeID: src.ID, RelativePath: "vacation/beach.jpg",
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
	src := testutil.NewTestVolume(t, db, "s")
	now := time.Now().UTC()
	rating := 3
	a := &domain.Asset{
		ID: "bad", VolumeID: src.ID, RelativePath: "x.jpg", FileStatus: domain.FileStatusOnline,
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

	src := testutil.NewTestVolume(t, db, "s")
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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")
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
	src := testutil.NewTestVolume(t, db, "s")
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

func TestAssetRepo_FindByVolumePath(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")
	testutil.NewTestAsset(t, db, src.ID, "sub/photo.jpg")

	got, err := repo.FindByVolumePath(ctx, src.ID, "sub/photo.jpg")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil || got.ID != "asset-sub/photo.jpg" {
		t.Fatalf("expected asset, got %v", got)
	}
}

func TestAssetRepo_MarkConnectivityByVolume(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "drive")
	testutil.NewTestAsset(t, db, src.ID, "a.jpg")
	testutil.NewTestAsset(t, db, src.ID, "b.jpg")

	if err := repo.MarkConnectivityByVolume(ctx, src.ID, false); err != nil {
		t.Fatalf("mark offline: %v", err)
	}
	got, _ := repo.Get(ctx, "asset-a.jpg")
	if got.FileStatus != domain.FileStatusOffline {
		t.Fatalf("got %q, want offline", got.FileStatus)
	}

	if err := repo.MarkConnectivityByVolume(ctx, src.ID, true); err != nil {
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
	src := testutil.NewTestVolume(t, db, "s")

	a1 := testutil.NewTestAsset(t, db, src.ID, "photo.jpg")
	if err := repo.SoftDelete(ctx, []string{a1.ID}); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	now := time.Now().UTC()
	reimported := &domain.Asset{
		ID: domain.NewID(), VolumeID: src.ID, RelativePath: "photo.jpg",
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
	src := testutil.NewTestVolume(t, db, "s")
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
	src := testutil.NewTestVolume(t, db, "s")
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
	src := testutil.NewTestVolume(t, db, "s")
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
	src := testutil.NewTestVolume(t, db, "s")
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
	src := testutil.NewTestVolume(t, db, "s")
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

// --- path_key: derived NFC identity form (D24) ---

// TestRebuildPathKeys proves path_key is derived state with a registered rebuild
// path: corrupt the keys, rebuild, and every key must equal NFC(relative_path).
func TestRebuildPathKeys(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	vol := testutil.NewTestVolume(t, db, "v")
	assets := &sqlite.AssetRepo{DB: db}

	// An NFD-composed name (macOS style): "cafe" + combining acute.
	nfdName := "cafe\u0301.jpg" // decomposed (e + combining acute)
	asset := &domain.Asset{
		ID: "a1", VolumeID: vol.ID, RelativePath: nfdName, FileStatus: domain.FileStatusOnline,
		Filename: nfdName, Extension: "jpg", MIMEType: "image/jpeg", FileType: domain.FileTypeImage,
		SizeBytes: 10, MTime: time.Now().UTC(), PartialHash: "h",
		IngestedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := assets.Create(ctx, asset); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Corrupt the stored key out from under the rows.
	if _, err := db.ExecContext(ctx, "UPDATE assets SET path_key = 'bogus'"); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.RebuildPathKeys(ctx, db); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	var key string
	if err := db.QueryRowContext(ctx, "SELECT path_key FROM assets WHERE id = 'a1'").Scan(&key); err != nil {
		t.Fatal(err)
	}
	if key != domain.PathKey(nfdName) {
		t.Fatalf("rebuilt path_key = %q, want NFC form %q", key, domain.PathKey(nfdName))
	}
}

// TestFindByVolumePath_NFDMatchesNFC is the §8 phantom-identity closure at the
// repo seam: a file stored with an NFD (decomposed) name matches when queried by
// its NFC (composed) form, because the comparison is key-to-key (path_key), not
// byte-to-byte — so the identity matrix reimports rather than minting a phantom.
func TestFindByVolumePath_NFDMatchesNFC(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	vol := testutil.NewTestVolume(t, db, "v")
	assets := &sqlite.AssetRepo{DB: db}

	nfdName := "cafe\u0301.jpg" // decomposed (e + combining acute), as macOS hands it out
	nfcName := "caf\u00e9.jpg"  // precomposed (é), the query form
	if nfdName == nfcName {
		t.Fatal("test bug: the two forms must differ at the byte level")
	}

	asset := &domain.Asset{
		ID: "a1", VolumeID: vol.ID, RelativePath: nfdName, FileStatus: domain.FileStatusOnline,
		Filename: nfdName, Extension: "jpg", MIMEType: "image/jpeg", FileType: domain.FileTypeImage,
		SizeBytes: 10, MTime: time.Now().UTC(), PartialHash: "h",
		IngestedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := assets.Create(ctx, asset); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := assets.FindByVolumePath(ctx, vol.ID, nfcName)
	if err != nil {
		t.Fatalf("find by NFC form: %v", err)
	}
	if got == nil || got.ID != "a1" {
		t.Fatalf("NFC query did not match the NFD-stored asset (got %v) — a phantom identity would be minted", got)
	}
}

// TestQueryAssets_FolderScope_NFDStoredNFCScope is the execution-level closure
// of the folder-scope half of DEFERRED §8: an asset stored under an NFD
// (decomposed) directory name IS found by a folder scope carrying the NFC
// (composed) prefix, because both sides compile to path_key (compare keys, open
// bytes — normalizing only the query side against raw NFD rows would miss).
func TestQueryAssets_FolderScope_NFDStoredNFCScope(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	vol := testutil.NewTestVolume(t, db, "v")
	assets := &sqlite.AssetRepo{DB: db}

	nfdDirectory := "cafe\u0301" // decomposed (e + combining acute), as macOS stores it
	nfcDirectory := "caf\u00e9"  // precomposed (é), the scope's query form
	if nfdDirectory == nfcDirectory {
		t.Fatal("test bug: the two forms must differ at the byte level")
	}
	asset := &domain.Asset{
		ID: "a1", VolumeID: vol.ID, RelativePath: nfdDirectory + "/shot.jpg", FileStatus: domain.FileStatusOnline,
		Filename: "shot.jpg", Extension: "jpg", MIMEType: "image/jpeg", FileType: domain.FileTypeImage,
		SizeBytes: 10, MTime: time.Now().UTC(), PartialHash: "h",
		IngestedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := assets.Create(ctx, asset); err != nil {
		t.Fatalf("create: %v", err)
	}

	query := ast.Query{
		Version: ast.Version,
		Scope:   &ast.Scope{Kind: ast.ScopeFolder, VolumeID: vol.ID, Path: nfcDirectory, Recursive: true},
	}
	rows, total, err := assets.QueryAssets(ctx, query,
		ast.Arrangement{SortField: ast.SortFilename, SortDir: ast.SortAsc}, ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != "a1" {
		t.Fatalf("NFC folder scope missed the NFD-stored asset: total=%d rows=%+v", total, rows)
	}
}

// --- Folder repository: the [syn] writer split ---

// TestFolderRepo_SetLastScannedAt proves the writer split on the folder table:
// the narrow sync-state writer records the scan cursor without touching judgment
// columns, and the user-action Update never clobbers the cursor back.
func TestFolderRepo_SetLastScannedAt(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	vol := testutil.NewTestVolume(t, db, "v")
	folder := testutil.NewTestFolder(t, db, vol.ID, "Photos")
	repo := &sqlite.FolderRepo{DB: db}

	scannedAt := time.Now().UTC().Truncate(time.Second)
	if err := repo.SetLastScannedAt(ctx, folder.ID, scannedAt); err != nil {
		t.Fatalf("set last scanned: %v", err)
	}
	got, err := repo.Get(ctx, folder.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastScannedAt == nil || !got.LastScannedAt.Equal(scannedAt) {
		t.Fatalf("last_scanned_at = %v, want %v", got.LastScannedAt, scannedAt)
	}
	// The [syn] write must not disturb judgment columns.
	if got.SyncMode != domain.SyncModeManual || !got.Enabled {
		t.Fatalf("sync-state write disturbed judgment columns: %+v", got)
	}

	// The user-action Update (a judgment write) must NOT clobber the cursor:
	// a stale in-memory LastScannedAt on the updated struct is ignored.
	got.Name = "Renamed"
	got.LastScannedAt = nil
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	after, err := repo.Get(ctx, folder.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Name != "Renamed" {
		t.Fatalf("update did not apply: %+v", after)
	}
	if after.LastScannedAt == nil || !after.LastScannedAt.Equal(scannedAt) {
		t.Fatalf("user Update clobbered the [syn] cursor: %v", after.LastScannedAt)
	}
}
