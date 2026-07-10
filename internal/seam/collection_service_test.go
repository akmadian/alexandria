package seam_test

import (
	"context"
	"errors"
	"testing"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/seam"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
)

// fakeCollections is an in-memory collection repository recording the last write.
type fakeCollections struct {
	list         []*domain.Collection
	byID         map[string]*domain.Collection
	created      *domain.Collection
	updated      *domain.Collection
	deleted      string
	addedTo      []string
	removedTo    []string
	err          error  // injected on List/Get/Create/Delete
	updateErr    error  // injected on Update only, so Get can still succeed
	failAddAsset string // AddAsset returns an error for this asset id (loop-abort test)
}

func (f *fakeCollections) List(context.Context) ([]*domain.Collection, error) {
	return f.list, f.err
}

func (f *fakeCollections) Get(_ context.Context, id string) (*domain.Collection, error) {
	if f.err != nil {
		return nil, f.err
	}
	if c, ok := f.byID[id]; ok {
		return c, nil
	}
	return nil, &domain.NotFoundError{Resource: "collection", ID: id}
}

func (f *fakeCollections) Create(_ context.Context, c *domain.Collection) error {
	if f.err != nil {
		return f.err
	}
	f.created = c
	return nil
}

func (f *fakeCollections) Update(_ context.Context, c *domain.Collection) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = c
	return nil
}

func (f *fakeCollections) Delete(_ context.Context, id string) error {
	f.deleted = id
	return f.err
}

func (f *fakeCollections) AddAsset(_ context.Context, _, assetID string) error {
	if assetID == f.failAddAsset {
		return errors.New("add failed")
	}
	f.addedTo = append(f.addedTo, assetID)
	return f.err
}

func (f *fakeCollections) RemoveAsset(_ context.Context, _, assetID string) error {
	f.removedTo = append(f.removedTo, assetID)
	return f.err
}

func TestCreateCollection_DefaultsToManual(t *testing.T) {
	fake := &fakeCollections{}
	service := seam.NewCollectionService(fake)

	got, err := service.CreateCollection(seam.CollectionInput{Name: "Favorites"})
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if got.ID == "" || got.Kind != domain.CollectionKindManual || got.Query != nil {
		t.Fatalf("expected manual collection with no query, got %+v", got)
	}
}

func TestCreateCollection_SmartStoresValidatedQuery(t *testing.T) {
	fake := &fakeCollections{}
	service := seam.NewCollectionService(fake)

	query := ast.Query{Version: 1}
	got, err := service.CreateCollection(seam.CollectionInput{
		Name:  "Recent RAWs",
		Kind:  domain.CollectionKindSmart,
		Query: &query,
	})
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if got.Query == nil || *got.Query == "" {
		t.Fatal("expected the smart query to be stored as JSON")
	}
}

func TestCreateCollection_RejectsContradictoryShapes(t *testing.T) {
	service := seam.NewCollectionService(&fakeCollections{})

	query := ast.Query{Version: 1}
	cases := []struct {
		name  string
		input seam.CollectionInput
	}{
		{"empty name", seam.CollectionInput{Name: ""}},
		{"smart without query", seam.CollectionInput{Name: "x", Kind: domain.CollectionKindSmart}},
		{"manual with query", seam.CollectionInput{Name: "x", Kind: domain.CollectionKindManual, Query: &query}},
		// Empty kind defaults to manual before the guards run, so a query on it must
		// still be rejected — proves the defaulting and the guard compose correctly.
		{"empty kind with query", seam.CollectionInput{Name: "x", Query: &query}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.CreateCollection(tc.input)
			assertDomainCode(t, err, "validation")
		})
	}
}

func TestCreateCollection_RepoErrorMapsToUnexpected(t *testing.T) {
	service := seam.NewCollectionService(&fakeCollections{err: errors.New("db down")})

	_, err := service.CreateCollection(seam.CollectionInput{Name: "Favorites"})
	assertUnexpected(t, err)
}

func TestDeleteCollection_RepoErrorMapsToUnexpected(t *testing.T) {
	service := seam.NewCollectionService(&fakeCollections{err: errors.New("db down")})

	err := service.DeleteCollection("c1")
	assertUnexpected(t, err)
}

func TestCreateCollection_InvalidQueryMapsToCode(t *testing.T) {
	service := seam.NewCollectionService(&fakeCollections{})

	bad := ast.Query{Version: 1, Where: ast.Leaf{Field: "nope", Cmp: ast.OpEq, Value: "x"}}
	_, err := service.CreateCollection(seam.CollectionInput{Name: "x", Kind: domain.CollectionKindSmart, Query: &bad})
	assertDomainCode(t, err, "query_invalid")
}

func TestUpdateCollection_OverlaysName(t *testing.T) {
	existing := &domain.Collection{ID: "c1", Name: "Old", Kind: domain.CollectionKindManual}
	fake := &fakeCollections{byID: map[string]*domain.Collection{"c1": existing}}
	service := seam.NewCollectionService(fake)

	name := "New"
	if err := service.UpdateCollection("c1", seam.CollectionPatch{Name: &name}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	if fake.updated.Name != "New" {
		t.Fatalf("expected renamed collection, got %+v", fake.updated)
	}
}

func TestUpdateCollection_OverlaysAndValidatesQuery(t *testing.T) {
	existing := &domain.Collection{ID: "c1", Name: "Smart", Kind: domain.CollectionKindSmart}
	fake := &fakeCollections{byID: map[string]*domain.Collection{"c1": existing}}
	service := seam.NewCollectionService(fake)

	query := ast.Query{Version: 1}
	if err := service.UpdateCollection("c1", seam.CollectionPatch{Query: &query}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	if fake.updated.Query == nil || *fake.updated.Query == "" {
		t.Fatal("expected the new query stored as JSON")
	}
}

func TestUpdateCollection_InvalidQueryMapsToCode(t *testing.T) {
	existing := &domain.Collection{ID: "c1", Name: "Smart", Kind: domain.CollectionKindSmart}
	fake := &fakeCollections{byID: map[string]*domain.Collection{"c1": existing}}
	service := seam.NewCollectionService(fake)

	bad := ast.Query{Version: 1, Where: ast.Leaf{Field: "nope", Cmp: ast.OpEq, Value: "x"}}
	err := service.UpdateCollection("c1", seam.CollectionPatch{Query: &bad})
	assertDomainCode(t, err, "query_invalid")
}

func TestUpdateCollection_OverlaysCoverAsset(t *testing.T) {
	existing := &domain.Collection{ID: "c1", Name: "Smart", Kind: domain.CollectionKindManual}
	fake := &fakeCollections{byID: map[string]*domain.Collection{"c1": existing}}
	service := seam.NewCollectionService(fake)

	cover := "asset-9"
	if err := service.UpdateCollection("c1", seam.CollectionPatch{CoverAssetID: &cover}); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	if fake.updated.CoverAssetID == nil || *fake.updated.CoverAssetID != "asset-9" {
		t.Fatalf("cover asset not overlaid: %+v", fake.updated.CoverAssetID)
	}
}

func TestUpdateCollection_UnknownIDMapsToNotFound(t *testing.T) {
	service := seam.NewCollectionService(&fakeCollections{byID: map[string]*domain.Collection{}})

	name := "x"
	err := service.UpdateCollection("missing", seam.CollectionPatch{Name: &name})
	assertDomainCode(t, err, "not_found")
}

func TestUpdateCollection_WriteErrorMapsToUnexpected(t *testing.T) {
	existing := &domain.Collection{ID: "c1", Name: "Old", Kind: domain.CollectionKindManual}
	fake := &fakeCollections{
		byID:      map[string]*domain.Collection{"c1": existing},
		updateErr: errors.New("db down"),
	}
	service := seam.NewCollectionService(fake)

	name := "New"
	err := service.UpdateCollection("c1", seam.CollectionPatch{Name: &name})
	assertUnexpected(t, err)
}

// The membership loop must abort on the first failing id, not silently continue —
// proving the behavior the AddToCollection ponytail comment describes.
func TestAddToCollection_AbortsOnFirstError(t *testing.T) {
	fake := &fakeCollections{failAddAsset: "b"}
	service := seam.NewCollectionService(fake)

	err := service.AddToCollection("c1", []string{"a", "b", "c"})
	assertUnexpected(t, err)
	if len(fake.addedTo) != 1 || fake.addedTo[0] != "a" {
		t.Fatalf("expected the loop to stop at the failing id (only 'a' added), got %v", fake.addedTo)
	}
}

func TestAddToCollection_RepoErrorMapsToUnexpected(t *testing.T) {
	service := seam.NewCollectionService(&fakeCollections{err: errors.New("db down")})

	err := service.AddToCollection("c1", []string{"a"})
	assertUnexpected(t, err)
}

// Real-catalog wiring: create + list + add-member against actual SQL, proving the
// service is bound to a real repo (the construction app.go uses), not just a fake.
func TestCollectionService_OverRealCatalog(t *testing.T) {
	db := testutil.NewTestDB(t)
	source := testutil.NewTestSource(t, db, "photos")
	asset := testutil.NewTestAsset(t, db, source.ID, "a.jpg")
	service := seam.NewCollectionService(&sqlite.CollectionRepo{DB: db})

	created, err := service.CreateCollection(seam.CollectionInput{Name: "Favorites"})
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := service.AddToCollection(created.ID, []string{asset.ID}); err != nil {
		t.Fatalf("AddToCollection: %v", err)
	}

	list, err := service.ListCollections()
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID || list[0].Name != "Favorites" {
		t.Fatalf("expected the created collection back, got %+v", list)
	}
}

func TestAddAndRemoveFromCollection_LoopEveryAsset(t *testing.T) {
	fake := &fakeCollections{}
	service := seam.NewCollectionService(fake)

	if err := service.AddToCollection("c1", []string{"a", "b", "c"}); err != nil {
		t.Fatalf("AddToCollection: %v", err)
	}
	if len(fake.addedTo) != 3 {
		t.Fatalf("expected 3 adds, got %v", fake.addedTo)
	}
	if err := service.RemoveFromCollection("c1", []string{"a"}); err != nil {
		t.Fatalf("RemoveFromCollection: %v", err)
	}
	if len(fake.removedTo) != 1 {
		t.Fatalf("expected 1 remove, got %v", fake.removedTo)
	}
}

func TestDeleteCollection_CallsRepo(t *testing.T) {
	fake := &fakeCollections{}
	service := seam.NewCollectionService(fake)

	if err := service.DeleteCollection("c1"); err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}
	if fake.deleted != "c1" {
		t.Fatalf("expected delete of c1, got %q", fake.deleted)
	}
}
