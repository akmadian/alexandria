package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
)

func TestQueryAssets_BasicFilter(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "a.jpg")
	testutil.NewTestAsset(t, db, src.ID, "b.jpg")
	testutil.NewTestAsset(t, db, src.ID, "c.jpg")

	if err := repo.ApplyTriagePatch(ctx, []string{"asset-a.jpg"}, catalog.TriagePatch{Rating: domain.SetOpt(5)}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ApplyTriagePatch(ctx, []string{"asset-b.jpg"}, catalog.TriagePatch{Rating: domain.SetOpt(3)}); err != nil {
		t.Fatal(err)
	}

	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpGte, Value: float64(4)},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 {
		t.Fatalf("total: got %d, want 1", total)
	}
	if len(rows) != 1 || rows[0].ID != "asset-a.jpg" {
		t.Fatalf("rows: got %v", rows)
	}
}

func TestQueryAssets_ExcludesDeleted(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "live.jpg")
	testutil.NewTestAsset(t, db, src.ID, "dead.jpg")
	if err := repo.SoftDelete(ctx, []string{"asset-dead.jpg"}); err != nil {
		t.Fatal(err)
	}

	query := ast.Query{Version: ast.Version}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 {
		t.Fatalf("total: got %d, want 1", total)
	}
	if rows[0].ID != "asset-live.jpg" {
		t.Fatalf("expected live.jpg, got %s", rows[0].ID)
	}
}

func TestQueryAssets_SourceScope(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()

	src1 := testutil.NewTestSource(t, db, "src1")
	src2 := testutil.NewTestSource(t, db, "src2")
	testutil.NewTestAsset(t, db, src1.ID, "a.jpg")
	testutil.NewTestAsset(t, db, src2.ID, "b.jpg")

	query := ast.Query{
		Version: ast.Version,
		Scope:   &ast.Scope{Kind: ast.ScopeFolder, SourceID: src1.ID, Recursive: true},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || rows[0].SourceID != src1.ID {
		t.Fatalf("expected source src1, got total=%d rows=%v", total, rows)
	}
}

func TestQueryAssets_NestedBooleanLogic(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "a.jpg")
	testutil.NewTestAsset(t, db, src.ID, "b.jpg")
	testutil.NewTestAsset(t, db, src.ID, "c.jpg")

	if err := repo.ApplyTriagePatch(ctx, []string{"asset-a.jpg"}, catalog.TriagePatch{
		Rating: domain.SetOpt(5), Flag: domain.SetOpt(domain.FlagPick),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ApplyTriagePatch(ctx, []string{"asset-b.jpg"}, catalog.TriagePatch{
		Rating: domain.SetOpt(3), Flag: domain.SetOpt(domain.FlagPick),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ApplyTriagePatch(ctx, []string{"asset-c.jpg"}, catalog.TriagePatch{
		Rating: domain.SetOpt(5), Flag: domain.SetOpt(domain.FlagReject),
	}); err != nil {
		t.Fatal(err)
	}

	// (rating=5 OR flag=pick) AND NOT(flag=reject)
	query := ast.Query{
		Version: ast.Version,
		Where: ast.Group{
			Op: ast.GroupAnd,
			Children: []ast.Node{
				ast.Group{
					Op: ast.GroupOr,
					Children: []ast.Node{
						ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpEq, Value: float64(5)},
						ast.Leaf{Field: ast.FieldFlag, Cmp: ast.OpIn, Value: []string{"pick"}},
					},
				},
				ast.Group{
					Op: ast.GroupNot,
					Children: []ast.Node{
						ast.Group{
							Op: ast.GroupAnd,
							Children: []ast.Node{
								ast.Leaf{Field: ast.FieldFlag, Cmp: ast.OpIn, Value: []string{"reject"}},
							},
						},
					},
				},
			},
		},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	// a.jpg (rating=5, pick) and b.jpg (rating=3, pick) match. c.jpg (rating=5, reject) excluded.
	if total != 2 {
		t.Fatalf("total: got %d, want 2", total)
	}
	ids := make(map[string]bool)
	for _, r := range rows {
		ids[r.ID] = true
	}
	if !ids["asset-a.jpg"] || !ids["asset-b.jpg"] {
		t.Fatalf("expected a.jpg and b.jpg, got %v", ids)
	}
}

func TestQueryAssets_EmptyOperator(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "rated.jpg")
	testutil.NewTestAsset(t, db, src.ID, "unrated.jpg")
	if err := repo.ApplyTriagePatch(ctx, []string{"asset-rated.jpg"}, catalog.TriagePatch{Rating: domain.SetOpt(3)}); err != nil {
		t.Fatal(err)
	}

	// Unrated: rating IS NULL (not eq 0)
	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpEmpty},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-unrated.jpg" {
		t.Fatalf("expected unrated.jpg, got total=%d rows=%v", total, rows)
	}
}

func TestAssetIDSlice_MatchesQueryAssets(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg", "e.jpg"} {
		testutil.NewTestAsset(t, db, src.ID, name)
	}

	query := ast.Query{Version: ast.Version}
	arrangement := ast.Arrangement{SortField: ast.SortFilename, SortDir: ast.SortAsc}

	rows, _, err := repo.QueryAssets(ctx, query, arrangement, ast.Page{Limit: 100})
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	ids, err := repo.AssetIDSlice(ctx, query, arrangement, 1, 3)
	if err != nil {
		t.Fatalf("id slice: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
	// IDs [1,3) should match rows[1] and rows[2].
	if ids[0] != rows[1].ID || ids[1] != rows[2].ID {
		t.Fatalf("id slice mismatch: got %v, want [%s, %s]", ids, rows[1].ID, rows[2].ID)
	}
}

func TestIndexOfAsset_InvertsIDSlice(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "a.jpg")
	testutil.NewTestAsset(t, db, src.ID, "b.jpg")
	testutil.NewTestAsset(t, db, src.ID, "c.jpg")

	query := ast.Query{Version: ast.Version}
	arrangement := ast.Arrangement{SortField: ast.SortFilename, SortDir: ast.SortAsc}

	rows, _, _ := repo.QueryAssets(ctx, query, arrangement, ast.Page{Limit: 100})
	for expectedIndex, row := range rows {
		position, err := repo.IndexOfAsset(ctx, query, arrangement, row.ID)
		if err != nil {
			t.Fatalf("index of %s: %v", row.ID, err)
		}
		if position == nil {
			t.Fatalf("index of %s: nil", row.ID)
		}
		if *position != expectedIndex {
			t.Errorf("index of %s: got %d, want %d", row.ID, *position, expectedIndex)
		}
	}

	// Non-existent asset returns nil.
	position, err := repo.IndexOfAsset(ctx, query, arrangement, "nonexistent")
	if err != nil {
		t.Fatalf("index of nonexistent: %v", err)
	}
	if position != nil {
		t.Fatalf("expected nil for nonexistent, got %d", *position)
	}
}

func TestApplyTriagePatchByQuery_SingleStatement(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"} {
		testutil.NewTestAsset(t, db, src.ID, name)
	}

	// Rate all except c.jpg via query.
	query := ast.Query{Version: ast.Version}
	affected, err := repo.ApplyTriagePatchByQuery(ctx, query, []string{"asset-c.jpg"},
		catalog.TriagePatch{Rating: domain.SetOpt(4)})
	if err != nil {
		t.Fatalf("apply by query: %v", err)
	}
	if len(affected) != 3 {
		t.Fatalf("expected 3 affected, got %d: %v", len(affected), affected)
	}

	// c.jpg should be unaffected.
	got, _ := repo.Get(ctx, "asset-c.jpg")
	if got.Rating != nil {
		t.Fatalf("c.jpg should be unaffected, got rating=%v", got.Rating)
	}
	// Others should have rating=4.
	for _, id := range []string{"asset-a.jpg", "asset-b.jpg", "asset-d.jpg"} {
		got, _ := repo.Get(ctx, id)
		if got.Rating == nil || *got.Rating != 4 {
			t.Errorf("%s: rating=%v, want 4", id, got.Rating)
		}
	}
}

func TestReadTriageStates(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "a.jpg")
	testutil.NewTestAsset(t, db, src.ID, "b.jpg")

	if err := repo.ApplyTriagePatch(ctx, []string{"asset-a.jpg"}, catalog.TriagePatch{
		Rating: domain.SetOpt(5), Flag: domain.SetOpt(domain.FlagPick),
	}); err != nil {
		t.Fatal(err)
	}

	states, err := repo.ReadTriageStates(ctx, []string{"asset-a.jpg", "asset-b.jpg"})
	if err != nil {
		t.Fatalf("read triage: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}

	stateMap := make(map[string]catalog.TriageState)
	for _, s := range states {
		stateMap[s.ID] = s
	}
	a := stateMap["asset-a.jpg"]
	if a.Rating == nil || *a.Rating != 5 {
		t.Errorf("a.jpg rating: got %v, want 5", a.Rating)
	}
	b := stateMap["asset-b.jpg"]
	if b.Rating != nil {
		t.Errorf("b.jpg rating: got %v, want nil", b.Rating)
	}
}

func TestDistinctValues(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	now := time.Now().UTC().Truncate(time.Second)
	cameraMake := "Canon"
	asset := &domain.Asset{
		ID: "a1", SourceID: src.ID, RelativePath: "a.jpg",
		FileStatus: domain.FileStatusOnline, Filename: "a.jpg",
		Extension: "jpg", MIMEType: "image/jpeg", FileType: domain.FileTypeImage,
		SizeBytes: 1024, MTime: now, PartialHash: "h1",
		CameraMake: &cameraMake, IngestedAt: now, UpdatedAt: now,
	}
	repo := &sqlite.AssetRepo{DB: db}
	if err := repo.Create(ctx, asset); err != nil {
		t.Fatalf("create: %v", err)
	}

	values, err := repo.DistinctValues(ctx, ast.FieldCameraMake)
	if err != nil {
		t.Fatalf("distinct: %v", err)
	}
	if len(values) != 1 || values[0] != "Canon" {
		t.Fatalf("expected [Canon], got %v", values)
	}
}

func TestQueryAssets_TagFilter(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "tagged.jpg")
	testutil.NewTestAsset(t, db, src.ID, "untagged.jpg")

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO tags (id, name, slug, path, created_at) VALUES ('t1', 'Beach', 'beach', '/t1/', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO asset_tags (asset_id, tag_id, source, created_at) VALUES ('asset-tagged.jpg', 't1', 'user', ?)`, now); err != nil {
		t.Fatal(err)
	}

	// has tag
	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldTag, Cmp: ast.OpHas, Value: "t1"},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query has tag: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-tagged.jpg" {
		t.Fatalf("expected tagged.jpg, got total=%d", total)
	}

	// empty tag (untagged)
	query = ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldTag, Cmp: ast.OpEmpty},
	}
	rows, total, err = repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query untagged: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-untagged.jpg" {
		t.Fatalf("expected untagged.jpg, got total=%d", total)
	}
}

func TestQueryAssets_TagUnderHierarchy(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "deep.jpg")
	testutil.NewTestAsset(t, db, src.ID, "other.jpg")

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO tags (id, name, slug, path, created_at) VALUES ('root', 'Travel', 'travel', '/root/', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tags (id, name, slug, parent_id, path, created_at) VALUES ('child', 'Japan', 'japan', 'root', '/root/child/', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO asset_tags (asset_id, tag_id, source, created_at) VALUES ('asset-deep.jpg', 'child', 'user', ?)`, now); err != nil {
		t.Fatal(err)
	}

	// tag under "root" should find deep.jpg (tagged with child of root)
	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldTag, Cmp: ast.OpUnder, Value: "root"},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-deep.jpg" {
		t.Fatalf("expected deep.jpg, got total=%d rows=%v", total, rows)
	}
}

func TestQueryAssets_TagTombstoneRespected(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "photo.jpg")
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO tags (id, name, slug, path, created_at) VALUES ('t1', 'Beach', 'beach', '/t1/', ?)`, now); err != nil {
		t.Fatal(err)
	}
	// Tombstoned tag — should NOT match.
	if _, err := db.ExecContext(ctx, `INSERT INTO asset_tags (asset_id, tag_id, source, removed_at, created_at) VALUES ('asset-photo.jpg', 't1', 'user', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}

	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldTag, Cmp: ast.OpHas, Value: "t1"},
	}
	_, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 0 {
		t.Fatalf("tombstoned tag should not match, got total=%d", total)
	}
}

func TestQueryAssets_FTSMatchesTags(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "photo.jpg")

	tagRepo := &sqlite.TagRepo{DB: db}
	if err := tagRepo.ImportKeywords(ctx, "asset-photo.jpg", []string{"landscape"}, nil, "user"); err != nil {
		t.Fatalf("import keywords: %v", err)
	}

	// Text search should find the asset by tag name.
	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldText, Cmp: ast.OpMatches, Value: "landscape"},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-photo.jpg" {
		t.Fatalf("expected photo.jpg via tag search, got total=%d", total)
	}
}

func TestQueryAssets_FTSAncestorTagName(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "event.jpg")

	tagRepo := &sqlite.TagRepo{DB: db}
	if err := tagRepo.ImportKeywords(ctx, "asset-event.jpg", nil, [][]string{{"Wedding", "2026"}}, "user"); err != nil {
		t.Fatalf("import keywords: %v", err)
	}

	// Searching for parent tag name should find the asset tagged with child.
	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldText, Cmp: ast.OpMatches, Value: "Wedding"},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-event.jpg" {
		t.Fatalf("expected event.jpg via ancestor tag search, got total=%d", total)
	}
}

func TestQueryAssets_Pagination(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	for i := 0; i < 10; i++ {
		testutil.NewTestAsset(t, db, src.ID, string(rune('a'+i))+".jpg")
	}

	query := ast.Query{Version: ast.Version}
	arrangement := ast.Arrangement{SortField: ast.SortFilename, SortDir: ast.SortAsc}

	page1, total, err := repo.QueryAssets(ctx, query, arrangement, ast.Page{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if total != 10 {
		t.Fatalf("total: got %d, want 10", total)
	}
	if len(page1) != 3 {
		t.Fatalf("page1 len: got %d, want 3", len(page1))
	}

	page2, _, err := repo.QueryAssets(ctx, query, arrangement, ast.Page{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 3 {
		t.Fatalf("page2 len: got %d, want 3", len(page2))
	}
	// Pages should not overlap.
	if page1[0].ID == page2[0].ID {
		t.Fatal("pages overlap")
	}
}

// --- helpers ---

func defaultArrangement() ast.Arrangement {
	return ast.Arrangement{SortField: ast.SortIngestedAt, SortDir: ast.SortDesc}
}

// The AssetRow projection's newest columns (duration_secs, camera_model — D24)
// must round-trip with real values: the assetRowColumns list and the scan's
// variable order are a hand-maintained pairing (C15 rung 4), and fixtures that
// leave both NULL would let a column swap pass silently.
func TestQueryAssets_RoundTripsDurationAndCameraModel(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")
	asset := testutil.NewTestAsset(t, db, src.ID, "clip.mp4")

	if _, err := db.Exec(
		`UPDATE assets SET duration_secs = 12.5, camera_model = 'A7 IV' WHERE id = ?`, asset.ID); err != nil {
		t.Fatal(err)
	}

	rows, _, err := repo.QueryAssets(ctx, ast.Query{Version: ast.Version},
		ast.Arrangement{SortField: ast.SortFilename, SortDir: ast.SortAsc}, ast.Page{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].DurationSecs == nil || *rows[0].DurationSecs != 12.5 {
		t.Fatalf("DurationSecs did not round-trip: %+v", rows[0].DurationSecs)
	}
	if rows[0].CameraModel == nil || *rows[0].CameraModel != "A7 IV" {
		t.Fatalf("CameraModel did not round-trip: %+v", rows[0].CameraModel)
	}
}
