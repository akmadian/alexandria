package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
)

func TestCollectionRepo_CRUDRoundTrip(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.CollectionRepo{DB: db}
	ctx := context.Background()

	collection := &domain.Collection{
		ID:      domain.NewID(),
		Name:    "Favorites",
		Kind:    domain.CollectionKindManual,
		SortDir: "asc",
	}
	if err := repo.Create(ctx, collection); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.Get(ctx, collection.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Favorites" || got.Kind != domain.CollectionKindManual {
		t.Fatalf("got %+v", got)
	}

	collection.Name = "Best Shots"
	if err := repo.Update(ctx, collection); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = repo.Get(ctx, collection.ID)
	if got.Name != "Best Shots" {
		t.Fatalf("name after update: %q", got.Name)
	}

	if err := repo.Delete(ctx, collection.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = repo.Get(ctx, collection.ID)
	var nf *domain.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError after delete, got %v", err)
	}
}

func TestCollectionRepo_List(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.CollectionRepo{DB: db}
	ctx := context.Background()

	for _, name := range []string{"Alpha", "Beta"} {
		repo.Create(ctx, &domain.Collection{ID: domain.NewID(), Name: name, Kind: domain.CollectionKindManual, SortDir: "asc"})
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
}

func TestCollectionRepo_AddRemoveAsset(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.CollectionRepo{DB: db}
	assetRepo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "a.jpg")
	testutil.NewTestAsset(t, db, src.ID, "b.jpg")

	col := &domain.Collection{ID: domain.NewID(), Name: "test", Kind: domain.CollectionKindManual, SortDir: "asc"}
	repo.Create(ctx, col)

	repo.AddAsset(ctx, col.ID, "asset-a.jpg")
	repo.AddAsset(ctx, col.ID, "asset-b.jpg")

	// Query via collection scope should return both.
	query := ast.Query{
		Version: ast.Version,
		Scope:   &ast.Scope{Kind: ast.ScopeCollection, ID: col.ID},
	}
	rows, total, err := assetRepo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 in collection, got %d", total)
	}
	_ = rows

	// Remove one.
	repo.RemoveAsset(ctx, col.ID, "asset-a.jpg")
	_, total, _ = assetRepo.QueryAssets(ctx, query, defaultArrangement(), ast.Page{Limit: 10})
	if total != 1 {
		t.Fatalf("expected 1 after remove, got %d", total)
	}
}

func TestCollectionRepo_AddAsset_PositionOrdering(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.CollectionRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		testutil.NewTestAsset(t, db, src.ID, name)
	}

	col := &domain.Collection{ID: domain.NewID(), Name: "ordered", Kind: domain.CollectionKindManual, SortDir: "asc"}
	repo.Create(ctx, col)
	repo.AddAsset(ctx, col.ID, "asset-a.jpg")
	repo.AddAsset(ctx, col.ID, "asset-b.jpg")
	repo.AddAsset(ctx, col.ID, "asset-c.jpg")

	// Positions should be sequential.
	rows, err := db.QueryContext(ctx,
		"SELECT asset_id, position FROM collection_assets WHERE collection_id = ? ORDER BY position", col.ID)
	if err != nil {
		t.Fatalf("query positions: %v", err)
	}
	defer rows.Close()

	var positions []int
	for rows.Next() {
		var assetID string
		var pos int
		if err := rows.Scan(&assetID, &pos); err != nil {
			t.Fatal(err)
		}
		positions = append(positions, pos)
	}
	if len(positions) != 3 || positions[0] >= positions[1] || positions[1] >= positions[2] {
		t.Fatalf("positions not sequential: %v", positions)
	}

	// Remove middle, add new — new should get max+1.
	repo.RemoveAsset(ctx, col.ID, "asset-b.jpg")
	testutil.NewTestAsset(t, db, src.ID, "d.jpg")
	repo.AddAsset(ctx, col.ID, "asset-d.jpg")

	var maxPos int
	db.QueryRowContext(ctx, "SELECT MAX(position) FROM collection_assets WHERE collection_id = ?", col.ID).Scan(&maxPos)
	if maxPos <= positions[2] {
		t.Fatalf("new position %d should exceed old max %d", maxPos, positions[2])
	}
}

func TestCollectionRepo_SmartCollection_ValidatesQuery(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.CollectionRepo{DB: db}
	ctx := context.Background()

	// Valid smart collection.
	validQuery := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpGte, Value: float64(4)},
	}
	queryBytes, _ := json.Marshal(validQuery)
	queryStr := string(queryBytes)
	col := &domain.Collection{
		ID: domain.NewID(), Name: "Smart", Kind: domain.CollectionKindSmart,
		Query: &queryStr, SortDir: "asc",
	}
	if err := repo.Create(ctx, col); err != nil {
		t.Fatalf("create valid smart: %v", err)
	}

	// Invalid query JSON should be rejected.
	badQuery := `{"version":1,"where":{"field":"nonexistent","cmp":"eq","value":"x"}}`
	badCol := &domain.Collection{
		ID: domain.NewID(), Name: "Bad", Kind: domain.CollectionKindSmart,
		Query: &badQuery, SortDir: "asc",
	}
	if err := repo.Create(ctx, badCol); err == nil {
		t.Fatal("expected error for invalid smart collection query")
	}
}

func TestCollectionRepo_SmartCollectionEvaluatesThroughQueryAssets(t *testing.T) {
	db := testutil.NewTestDB(t)
	assetRepo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestSource(t, db, "s")

	testutil.NewTestAsset(t, db, src.ID, "good.jpg")
	testutil.NewTestAsset(t, db, src.ID, "bad.jpg")

	assetRepo.ApplyTriagePatch(ctx, []string{"asset-good.jpg"},
		catalog.TriagePatch{Rating: domain.SetOpt(5)})

	// Simulate smart collection: stored query for rating >= 4.
	storedQuery := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldRating, Cmp: ast.OpGte, Value: float64(4)},
	}

	// MergeScope: user adds another filter on top.
	userQuery := ast.Query{
		Version: ast.Version,
		Where:   ast.Leaf{Field: ast.FieldFileType, Cmp: ast.OpIn, Value: []string{"image"}},
	}
	merged := ast.MergeScope(userQuery, storedQuery.Where)

	rows, total, err := assetRepo.QueryAssets(ctx, merged, defaultArrangement(), ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || rows[0].ID != "asset-good.jpg" {
		t.Fatalf("expected good.jpg, got total=%d", total)
	}
}
