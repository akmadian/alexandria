package sqlite_test

import (
	"context"
	"fmt"
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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")

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

	src1 := testutil.NewTestVolume(t, db, "src1")
	src2 := testutil.NewTestVolume(t, db, "src2")
	testutil.NewTestAsset(t, db, src1.ID, "a.jpg")
	testutil.NewTestAsset(t, db, src2.ID, "b.jpg")

	query := ast.Query{
		Version: ast.Version,
		Scope:   &ast.Scope{Kind: ast.ScopeFolder, VolumeID: src1.ID, Recursive: true},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || rows[0].VolumeID != src1.ID {
		t.Fatalf("expected source src1, got total=%d rows=%v", total, rows)
	}
}

func TestQueryAssets_NestedBooleanLogic(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")

	now := time.Now().UTC().Truncate(time.Second)
	cameraMake := "Canon"
	asset := &domain.Asset{
		ID: "a1", VolumeID: src.ID, RelativePath: "a.jpg",
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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")

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
	src := testutil.NewTestVolume(t, db, "s")
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

// Task 17 gap 1 — date-range predicates (within / notWithin) against real rows.
// The compile tests fix the SQL shape and args for a given `now`; these execute
// the resolved half-open [start, end) interval against stored captured_at values
// to confirm rolling-now resolution, calendar arithmetic, and the negated form
// (which, on a nullable column, includes absent rows) actually select correctly.
func TestQueryAssets_DateWithinRolling(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "recent.jpg")
	testutil.NewTestAsset(t, db, src.ID, "old.jpg")
	// recent is inside a rolling 30-day window; old is well outside it. Margins are
	// wide so the microsecond gap between the fixture `now` and the query-time `now`
	// (QueryAssets calls time.Now internally) cannot flip a boundary.
	recentAt := time.Now().Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)
	oldAt := time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `UPDATE assets SET captured_at = ? WHERE id = ?`, recentAt, "asset-recent.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE assets SET captured_at = ? WHERE id = ?`, oldAt, "asset-old.jpg"); err != nil {
		t.Fatal(err)
	}

	// captured within the last 30 days (anchor "now", duration -30d).
	query := ast.Query{
		Version: ast.Version,
		Where: ast.Leaf{Field: ast.FieldCapturedAt, Cmp: ast.OpWithin, Value: ast.DateValue{
			Anchor:   ast.DateAnchor{Now: true},
			Duration: ast.DateDuration{Days: -30},
		}},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-recent.jpg" {
		t.Fatalf("expected recent.jpg within last 30 days, got total=%d rows=%v", total, rows)
	}
}

func TestQueryAssets_DateWithinConcreteAnchor(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "inside.jpg")
	testutil.NewTestAsset(t, db, src.ID, "outside.jpg")
	// Concrete-anchor window: [2026-01-01, 2026-01-08). inside falls in it; outside
	// (a month later) does not. A concrete anchor never rolls with `now`.
	if _, err := db.ExecContext(ctx, `UPDATE assets SET captured_at = ? WHERE id = ?`,
		"2026-01-03T12:00:00Z", "asset-inside.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE assets SET captured_at = ? WHERE id = ?`,
		"2026-02-01T12:00:00Z", "asset-outside.jpg"); err != nil {
		t.Fatal(err)
	}

	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	query := ast.Query{
		Version: ast.Version,
		Where: ast.Leaf{Field: ast.FieldCapturedAt, Cmp: ast.OpWithin, Value: ast.DateValue{
			Anchor:   ast.DateAnchor{Date: anchor},
			Duration: ast.DateDuration{Days: 7},
		}},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-inside.jpg" {
		t.Fatalf("expected inside.jpg within concrete window, got total=%d rows=%v", total, rows)
	}
}

func TestQueryAssets_DateNotWithin(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "recent.jpg")
	testutil.NewTestAsset(t, db, src.ID, "old.jpg")
	testutil.NewTestAsset(t, db, src.ID, "undated.jpg") // captured_at stays NULL
	recentAt := time.Now().Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)
	oldAt := time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `UPDATE assets SET captured_at = ? WHERE id = ?`, recentAt, "asset-recent.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE assets SET captured_at = ? WHERE id = ?`, oldAt, "asset-old.jpg"); err != nil {
		t.Fatal(err)
	}

	// NOT captured within the last 30 days. capturedAt is nullable, so the
	// NULL-negation policy makes the negated predicate include the undated row.
	query := ast.Query{
		Version: ast.Version,
		Where: ast.Leaf{Field: ast.FieldCapturedAt, Cmp: ast.OpNotWithin, Value: ast.DateValue{
			Anchor:   ast.DateAnchor{Now: true},
			Duration: ast.DateDuration{Days: -30},
		}},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	matched := make(map[string]bool)
	for _, row := range rows {
		matched[row.ID] = true
	}
	if total != 2 || !matched["asset-old.jpg"] || !matched["asset-undated.jpg"] {
		t.Fatalf("expected old.jpg + undated.jpg, got total=%d matched=%v", total, matched)
	}
	if matched["asset-recent.jpg"] {
		t.Fatal("recent.jpg is within the window and must not match notWithin")
	}
}

// Task 17 gap 2 — LIKE operators (contains / startsWith) against real filenames.
// CompileLIKEEscaping proves the args backslash-escape % and _; this proves the
// escaping is load-bearing at query time: a filename holding LIKE metacharacters
// matches only literally, and control rows that would match an unescaped pattern
// do not.
func TestQueryAssets_LikeContainsEscapesMetacharacters(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "photo_100%.jpg") // the metacharacter-bearing name
	testutil.NewTestAsset(t, db, src.ID, "photo1000.jpg")  // would match "100%" if % were a wildcard
	testutil.NewTestAsset(t, db, src.ID, "photoX100Y.jpg") // would match "photo_" if _ were a wildcard

	// contains "100%" — the % must be literal, so only photo_100%.jpg qualifies.
	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldFilename, Cmp: ast.OpContains, Value: "100%"},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("contains query: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-photo_100%.jpg" {
		t.Fatalf(`expected only photo_100%%.jpg to contain "100%%", got total=%d rows=%v`, total, rows)
	}
}

func TestQueryAssets_LikeStartsWithEscapesUnderscore(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "photo_100%.jpg")
	testutil.NewTestAsset(t, db, src.ID, "photo1000.jpg")
	testutil.NewTestAsset(t, db, src.ID, "photoX100Y.jpg")

	// startsWith "photo_" — the _ must be literal, so photo1000/photoX100Y (which
	// an unescaped single-char wildcard would catch) are excluded.
	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldFilename, Cmp: ast.OpStartsWith, Value: "photo_"},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("startsWith query: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-photo_100%.jpg" {
		t.Fatalf(`expected only photo_100%%.jpg to start with "photo_", got total=%d rows=%v`, total, rows)
	}
}

// Task 17 gap 3 — FTS quoting edge cases against real rows. quoteFTS wraps the
// search text so FTS5 operators (*, -, ", NEAR) are literal tokens; unquoted, a
// bare `"` is an unbalanced-phrase syntax error and NEAR/-/* change the query's
// meaning. These search text that holds those characters and confirm the intended
// row matches with no FTS syntax error.
func TestQueryAssets_FTSFilenameWithOperators(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, `report*NEAR"final.jpg`) // tokens: report, near, final
	testutil.NewTestAsset(t, db, src.ID, "plain.jpg")

	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldText, Cmp: ast.OpMatches, Value: `report*NEAR"final`},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("fts query with operators errored: %v", err)
	}
	if total != 1 || rows[0].ID != `asset-report*NEAR"final.jpg` {
		t.Fatalf("expected the operator-bearing filename to match, got total=%d rows=%v", total, rows)
	}
}

func TestQueryAssets_FTSTagWithOperators(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "genre.jpg")
	testutil.NewTestAsset(t, db, src.ID, "other.jpg")

	tagRepo := &sqlite.TagRepo{DB: db}
	// ImportKeywords maintains the FTS tags column. The hyphen is an FTS operator
	// unless the search term is quoted.
	if err := tagRepo.ImportKeywords(ctx, "asset-genre.jpg", []string{"sci-fi"}, nil, "user"); err != nil {
		t.Fatalf("import keywords: %v", err)
	}
	if err := tagRepo.ImportKeywords(ctx, "asset-other.jpg", []string{"documentary"}, nil, "user"); err != nil {
		t.Fatalf("import keywords: %v", err)
	}

	query := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldText, Cmp: ast.OpMatches, Value: "sci-fi"},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("fts tag query with operator errored: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-genre.jpg" {
		t.Fatalf("expected the sci-fi tagged asset to match, got total=%d rows=%v", total, rows)
	}
}

// Task 17 gap 4 — sort tiebreaker correctness under value collisions. When every
// row shares the sort-column value, the always-ascending id tiebreaker must yield
// one deterministic total order so paging neither duplicates nor skips a row.
func TestQueryAssets_PaginationStableUnderTiedSortValues(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")

	const assetCount = 12
	ids := make([]string, 0, assetCount)
	for i := 0; i < assetCount; i++ {
		name := fmt.Sprintf("asset%02d.jpg", i)
		asset := testutil.NewTestAsset(t, db, src.ID, name)
		ids = append(ids, asset.ID)
	}
	// Every asset gets the SAME rating, so rating cannot distinguish rows — the id
	// tiebreaker is the only thing imposing a total order.
	if err := repo.ApplyTriagePatch(ctx, ids, catalog.TriagePatch{Rating: domain.SetOpt(3)}); err != nil {
		t.Fatal(err)
	}

	arrangement := ast.Arrangement{SortField: ast.SortRating, SortDir: ast.SortDesc}
	query := ast.Query{Version: ast.Version}

	const pageSize = 5
	seen := make(map[string]int)
	for offset := 0; offset < assetCount; offset += pageSize {
		page, total, err := repo.QueryAssets(ctx, query, arrangement, ast.Page{Limit: pageSize, Offset: offset})
		if err != nil {
			t.Fatalf("page at offset %d: %v", offset, err)
		}
		if total != assetCount {
			t.Fatalf("total: got %d, want %d", total, assetCount)
		}
		for _, row := range page {
			seen[row.ID]++
		}
	}

	if len(seen) != assetCount {
		t.Fatalf("paged union covered %d distinct rows, want %d (rows skipped or duplicated)", len(seen), assetCount)
	}
	for _, id := range ids {
		if seen[id] != 1 {
			t.Errorf("asset %s appeared %d times across pages, want exactly 1", id, seen[id])
		}
	}
}

// Task 17 gap 5 — tag-scoped queries (ScopeTag) against real data. The compile
// test proves ScopeTag emits an asset_tags subquery; this runs it: only assets in
// the tag's subtree return, and an additional predicate composes on top.
func TestQueryAssets_ScopeTag(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "tagged-a.jpg")
	testutil.NewTestAsset(t, db, src.ID, "tagged-b.jpg")
	testutil.NewTestAsset(t, db, src.ID, "untagged.jpg")

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO tags (id, name, slug, path, created_at) VALUES ('t1', 'Beach', 'beach', '/t1/', ?)`, now); err != nil {
		t.Fatal(err)
	}
	for _, assetID := range []string{"asset-tagged-a.jpg", "asset-tagged-b.jpg"} {
		if _, err := db.ExecContext(ctx, `INSERT INTO asset_tags (asset_id, tag_id, source, created_at) VALUES (?, 't1', 'user', ?)`, assetID, now); err != nil {
			t.Fatal(err)
		}
	}

	// Scope-only: exactly the two tagged assets.
	scoped := ast.Query{
		Version: ast.Version,
		Scope:   &ast.Scope{Kind: ast.ScopeTag, ID: "t1"},
	}
	rows, total, err := repo.QueryAssets(ctx, scoped, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("scope query: %v", err)
	}
	scopedIDs := make(map[string]bool)
	for _, row := range rows {
		scopedIDs[row.ID] = true
	}
	if total != 2 || !scopedIDs["asset-tagged-a.jpg"] || !scopedIDs["asset-tagged-b.jpg"] {
		t.Fatalf("expected both tagged assets in scope, got total=%d rows=%v", total, rows)
	}

	// Scope + predicate: rate only one, filter to rating>=4 — the tag scope and the
	// predicate must both bind.
	if err := repo.ApplyTriagePatch(ctx, []string{"asset-tagged-a.jpg"}, catalog.TriagePatch{Rating: domain.SetOpt(5)}); err != nil {
		t.Fatal(err)
	}
	composed := ast.Query{
		Version: ast.Version,
		Scope:   &ast.Scope{Kind: ast.ScopeTag, ID: "t1"},
		Where:   ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpGte, Value: float64(4)},
	}
	rows, total, err = repo.QueryAssets(ctx, composed, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("composed query: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-tagged-a.jpg" {
		t.Fatalf("expected only the rated tagged asset, got total=%d rows=%v", total, rows)
	}
}

// Task 17 gap 6 — MergeScope with a real scope + stored query + user filter. The
// existing smart-collection test merges predicates but leaves the outer scope nil;
// this sets a folder scope on the outer query so all three constraints (scope,
// stored predicate, user predicate) must hold at once.
func TestQueryAssets_MergeScopeWithScopeAndUserFilter(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	srcA := testutil.NewTestVolume(t, db, "srcA")
	srcB := testutil.NewTestVolume(t, db, "srcB")

	// winner satisfies all three; each decoy fails exactly one constraint.
	winner := testutil.NewTestAsset(t, db, srcA.ID, "winner.jpg")
	lowRating := testutil.NewTestAsset(t, db, srcA.ID, "low-rating.jpg")
	notPicked := testutil.NewTestAsset(t, db, srcA.ID, "not-picked.jpg")
	wrongSource := testutil.NewTestAsset(t, db, srcB.ID, "wrong-source.jpg")

	if err := repo.ApplyTriagePatch(ctx, []string{winner.ID}, catalog.TriagePatch{
		Rating: domain.SetOpt(5), Flag: domain.SetOpt(domain.FlagPick),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ApplyTriagePatch(ctx, []string{lowRating.ID}, catalog.TriagePatch{
		Rating: domain.SetOpt(2), Flag: domain.SetOpt(domain.FlagPick), // fails stored rating>=4
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ApplyTriagePatch(ctx, []string{notPicked.ID}, catalog.TriagePatch{
		Rating: domain.SetOpt(5), Flag: domain.SetOpt(domain.FlagReject), // fails user flag=pick
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ApplyTriagePatch(ctx, []string{wrongSource.ID}, catalog.TriagePatch{
		Rating: domain.SetOpt(5), Flag: domain.SetOpt(domain.FlagPick), // fails srcA scope
	}); err != nil {
		t.Fatal(err)
	}

	// outer carries the source scope (where you're looking) and the user's ad-hoc
	// filter; storedWhere is the smart collection's persisted predicate.
	outer := ast.Query{
		Version: ast.Version,
		Scope:   &ast.Scope{Kind: ast.ScopeFolder, VolumeID: srcA.ID, Recursive: true},
		Where:   ast.Leaf{Field: ast.FieldFlag, Cmp: ast.OpIn, Value: []string{"pick"}},
	}
	storedWhere := ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpGte, Value: float64(4)}
	merged := ast.MergeScope(outer, storedWhere)

	rows, total, err := repo.QueryAssets(ctx, merged, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("merged query: %v", err)
	}
	if total != 1 || rows[0].ID != winner.ID {
		t.Fatalf("expected only winner.jpg (scope+stored+user all bind), got total=%d rows=%v", total, rows)
	}
}

// Task 17 gap 7 — multi-field compound query. The nested-boolean test uses two
// fields; this intersects four heterogeneous field kinds (numeric rating, date
// range, tag reference, free-text FTS) with one decoy per dimension, so a single
// asset survives only by satisfying every constraint.
func TestQueryAssets_CompoundMultiField(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")

	// Filenames carry the "sunset" FTS token except the text decoy.
	winner := testutil.NewTestAsset(t, db, src.ID, "winner-sunset.jpg")    // matches all four
	dimRating := testutil.NewTestAsset(t, db, src.ID, "dim-sunset.jpg")    // fails rating
	staleDate := testutil.NewTestAsset(t, db, src.ID, "stale-sunset.jpg")  // fails date
	bareTag := testutil.NewTestAsset(t, db, src.ID, "bare-sunset.jpg")     // fails tag
	noText := testutil.NewTestAsset(t, db, src.ID, "winner-landscape.jpg") // fails text

	recentAt := time.Now().Add(-5 * 24 * time.Hour).UTC().Format(time.RFC3339)
	staleAt := time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
	for _, id := range []string{winner.ID, dimRating.ID, bareTag.ID, noText.ID} {
		if _, err := db.ExecContext(ctx, `UPDATE assets SET captured_at = ? WHERE id = ?`, recentAt, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE assets SET captured_at = ? WHERE id = ?`, staleAt, staleDate.ID); err != nil {
		t.Fatal(err)
	}

	// Rate everyone 5 except the rating decoy.
	for _, id := range []string{winner.ID, staleDate.ID, bareTag.ID, noText.ID} {
		if err := repo.ApplyTriagePatch(ctx, []string{id}, catalog.TriagePatch{Rating: domain.SetOpt(5)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.ApplyTriagePatch(ctx, []string{dimRating.ID}, catalog.TriagePatch{Rating: domain.SetOpt(2)}); err != nil {
		t.Fatal(err)
	}

	// Tag t1 on everyone except the tag decoy. The tag name ("Beach") deliberately
	// avoids the "sunset" token so the text predicate binds only through filenames.
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO tags (id, name, slug, path, created_at) VALUES ('t1', 'Beach', 'beach', '/t1/', ?)`, now); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{winner.ID, dimRating.ID, staleDate.ID, noText.ID} {
		if _, err := db.ExecContext(ctx, `INSERT INTO asset_tags (asset_id, tag_id, source, created_at) VALUES (?, 't1', 'user', ?)`, id, now); err != nil {
			t.Fatal(err)
		}
	}

	// rating>=4 AND capturedAt within last 30d AND has tag t1 AND text matches "sunset".
	query := ast.Query{
		Version: ast.Version,
		Where: ast.Group{
			Op: ast.GroupAnd,
			Children: []ast.Node{
				ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpGte, Value: float64(4)},
				ast.Leaf{Field: ast.FieldCapturedAt, Cmp: ast.OpWithin, Value: ast.DateValue{
					Anchor:   ast.DateAnchor{Now: true},
					Duration: ast.DateDuration{Days: -30},
				}},
				ast.Leaf{Field: ast.FieldTag, Cmp: ast.OpHas, Value: "t1"},
				ast.Leaf{Field: ast.FieldText, Cmp: ast.OpMatches, Value: "sunset"},
			},
		},
	}
	rows, total, err := repo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("compound query: %v", err)
	}
	if total != 1 || rows[0].ID != winner.ID {
		t.Fatalf("expected only winner-sunset.jpg to satisfy all four fields, got total=%d rows=%v", total, rows)
	}
}
