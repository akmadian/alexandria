package seam_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/seam"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
)

// filenameAsc is a concrete arrangement for the real-catalog tests (the compiler
// always appends an id tiebreaker, so order is deterministic).
var filenameAsc = ast.Arrangement{SortField: ast.SortFilename, SortDir: ast.SortAsc}

// fakeAssets satisfies both asset slices the AssetService needs. It records the
// last write it saw and can inject an error on any call.
type fakeAssets struct {
	rows          []catalog.AssetRow
	total         int
	distinct      []string
	index         *int
	asset         *domain.Asset
	err           error
	gotIDs        []string
	gotPatch      catalog.TriagePatch
	gotQuery      *ast.Query
	gotExceptIDs  []string
	byQueryResult []string
}

func (f *fakeAssets) Get(_ context.Context, id string) (*domain.Asset, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.asset, nil
}

func (f *fakeAssets) QueryAssets(_ context.Context, _ ast.Query, _ ast.Arrangement, _ ast.Page) ([]catalog.AssetRow, int, error) {
	return f.rows, f.total, f.err
}

func (f *fakeAssets) AssetIDSlice(_ context.Context, _ ast.Query, _ ast.Arrangement, _, _ int) ([]string, error) {
	return f.distinct, f.err
}

func (f *fakeAssets) IndexOfAsset(_ context.Context, _ ast.Query, _ ast.Arrangement, _ string) (*int, error) {
	return f.index, f.err
}

func (f *fakeAssets) DistinctValues(_ context.Context, _ ast.Field) ([]string, error) {
	return f.distinct, f.err
}

func (f *fakeAssets) ApplyTriagePatch(_ context.Context, ids []string, p catalog.TriagePatch) error {
	f.gotIDs, f.gotPatch = ids, p
	return f.err
}

func (f *fakeAssets) ApplyTriagePatchByQuery(_ context.Context, query ast.Query, exceptIDs []string, p catalog.TriagePatch) ([]string, error) {
	f.gotQuery, f.gotExceptIDs, f.gotPatch = &query, exceptIDs, p
	return f.byQueryResult, f.err
}

func (f *fakeAssets) SoftDelete(_ context.Context, ids []string) error {
	f.gotIDs = ids
	return f.err
}

// validQuery is a minimal well-formed query (version-only, scope-all).
func validQuery() ast.Query { return ast.Query{Version: 1} }

func TestQueryAssets_ReturnsItemsAndTotal(t *testing.T) {
	fake := &fakeAssets{rows: []catalog.AssetRow{{ID: "a"}}, total: 42}
	service := seam.NewAssetService(fake, fake)

	got, err := service.QueryAssets(validQuery(), ast.Arrangement{}, ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("QueryAssets: %v", err)
	}
	if got.Total != 42 || len(got.Items) != 1 || got.Items[0].ID != "a" {
		t.Fatalf("got %+v", got)
	}
}

func TestQueryAssets_InvalidQueryMapsToQueryInvalid(t *testing.T) {
	service := seam.NewAssetService(&fakeAssets{}, &fakeAssets{})

	bad := ast.Query{Version: 1, Where: ast.Leaf{Field: "nope", Cmp: ast.OpEq, Value: "x"}}
	_, err := service.QueryAssets(bad, ast.Arrangement{}, ast.Page{})
	assertDomainCode(t, err, "query_invalid")
}

func TestQueryAssets_VersionTooNewMapsToCode(t *testing.T) {
	fake := &fakeAssets{}
	service := seam.NewAssetService(fake, fake)

	_, err := service.QueryAssets(ast.Query{Version: 999}, ast.Arrangement{}, ast.Page{})
	assertDomainCode(t, err, "query_version_too_new")
}

func TestQueryAssets_RepoErrorMapsToUnexpected(t *testing.T) {
	fake := &fakeAssets{err: errors.New("sql exploded")}
	service := seam.NewAssetService(fake, fake)

	_, err := service.QueryAssets(validQuery(), ast.Arrangement{}, ast.Page{})
	assertUnexpected(t, err)
}

func TestGetAsset_NotFoundMapsToCode(t *testing.T) {
	fake := &fakeAssets{err: &domain.NotFoundError{Resource: "asset", ID: "x"}}
	service := seam.NewAssetService(fake, fake)

	_, err := service.GetAsset("x")
	assertDomainCode(t, err, "not_found")
}

func TestAssetIDSlice_ValidatesBeforeRepo(t *testing.T) {
	fake := &fakeAssets{distinct: []string{"a", "b"}}
	service := seam.NewAssetService(fake, fake)

	bad := ast.Query{Version: 1, Where: ast.Leaf{Field: "nope", Cmp: ast.OpEq, Value: "x"}}
	if _, err := service.AssetIDSlice(bad, ast.Arrangement{}, 0, 10); err == nil {
		t.Fatal("expected validation to reject the query before the repo")
	} else {
		assertDomainCode(t, err, "query_invalid")
	}

	got, err := service.AssetIDSlice(validQuery(), ast.Arrangement{}, 0, 10)
	if err != nil {
		t.Fatalf("AssetIDSlice: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestIndexOfAsset_ValidatesAndReportsPresence(t *testing.T) {
	found := 3
	fake := &fakeAssets{index: &found}
	service := seam.NewAssetService(fake, fake)

	bad := ast.Query{Version: 1, Where: ast.Leaf{Field: "nope", Cmp: ast.OpEq, Value: "x"}}
	if _, err := service.IndexOfAsset(bad, ast.Arrangement{}, "id"); err == nil {
		t.Fatal("expected validation to reject the query before the repo")
	} else {
		assertDomainCode(t, err, "query_invalid")
	}

	got, err := service.IndexOfAsset(validQuery(), ast.Arrangement{}, "id")
	if err != nil {
		t.Fatalf("IndexOfAsset: %v", err)
	}
	if got == nil || *got != 3 {
		t.Fatalf("expected index 3, got %v", got)
	}
}

func TestDistinctValues_RepoErrorMapsToUnexpected(t *testing.T) {
	fake := &fakeAssets{err: errors.New("sql exploded")}
	service := seam.NewAssetService(fake, fake)

	_, err := service.DistinctValues("cameraMake")
	assertUnexpected(t, err)
}

func TestUpdateAssets_ByIDsAppliesPatch(t *testing.T) {
	fake := &fakeAssets{}
	service := seam.NewAssetService(fake, fake)

	patch := seam.TriagePatchInput{
		Rating: json.RawMessage(`5`),
		Flag:   json.RawMessage(`null`),
	}
	if err := service.UpdateAssets(seam.UpdateTarget{IDs: []string{"a", "b"}}, patch); err != nil {
		t.Fatalf("UpdateAssets: %v", err)
	}
	if len(fake.gotIDs) != 2 {
		t.Fatalf("expected 2 ids, got %v", fake.gotIDs)
	}
	// rating: value → set with 5; flag: null → set with nil (clear); colorLabel: absent → not set.
	if !fake.gotPatch.Rating.Set || fake.gotPatch.Rating.Value == nil || *fake.gotPatch.Rating.Value != 5 {
		t.Fatalf("rating opt wrong: %+v", fake.gotPatch.Rating)
	}
	if !fake.gotPatch.Flag.Set || fake.gotPatch.Flag.Value != nil {
		t.Fatalf("flag opt should be set-to-clear: %+v", fake.gotPatch.Flag)
	}
	if fake.gotPatch.ColorLabel.Set {
		t.Fatalf("colorLabel absent should stay untouched: %+v", fake.gotPatch.ColorLabel)
	}
}

func TestUpdateAssets_DecodesEnumValueField(t *testing.T) {
	fake := &fakeAssets{}
	service := seam.NewAssetService(fake, fake)

	// colorLabel is an enum type — prove the three-state raw decode produces the
	// right domain value, not just int/null (which the other test covers).
	patch := seam.TriagePatchInput{ColorLabel: json.RawMessage(`"red"`)}
	if err := service.UpdateAssets(seam.UpdateTarget{IDs: []string{"a"}}, patch); err != nil {
		t.Fatalf("UpdateAssets: %v", err)
	}
	got := fake.gotPatch.ColorLabel
	if !got.Set || got.Value == nil || *got.Value != domain.ColorLabelRed {
		t.Fatalf("colorLabel opt wrong: %+v", got)
	}
}

func TestUpdateAssets_ByQueryPassesExceptIDs(t *testing.T) {
	fake := &fakeAssets{}
	service := seam.NewAssetService(fake, fake)

	query := validQuery()
	err := service.UpdateAssets(seam.UpdateTarget{Query: &query, ExceptIDs: []string{"skip"}}, seam.TriagePatchInput{})
	if err != nil {
		t.Fatalf("UpdateAssets: %v", err)
	}
	if fake.gotQuery == nil || len(fake.gotExceptIDs) != 1 || fake.gotExceptIDs[0] != "skip" {
		t.Fatalf("expected by-query apply with exceptIds, got query=%v except=%v", fake.gotQuery, fake.gotExceptIDs)
	}
}

func TestUpdateAssets_ByQueryValidatesBeforeWriting(t *testing.T) {
	fake := &fakeAssets{}
	service := seam.NewAssetService(fake, fake)

	// A malformed query must be rejected before the mass UPDATE runs.
	bad := ast.Query{Version: 1, Where: ast.Leaf{Field: "nope", Cmp: ast.OpEq, Value: "x"}}
	err := service.UpdateAssets(seam.UpdateTarget{Query: &bad}, seam.TriagePatchInput{Rating: json.RawMessage(`5`)})
	assertDomainCode(t, err, "query_invalid")
	if fake.gotQuery != nil {
		t.Fatal("the repo must not be reached when the query is invalid")
	}
}

func TestUpdateAssets_EmptyTargetMapsToValidation(t *testing.T) {
	service := seam.NewAssetService(&fakeAssets{}, &fakeAssets{})

	err := service.UpdateAssets(seam.UpdateTarget{}, seam.TriagePatchInput{})
	assertDomainCode(t, err, "validation")
}

func TestUpdateAssets_BadPatchValueMapsToValidation(t *testing.T) {
	service := seam.NewAssetService(&fakeAssets{}, &fakeAssets{})

	patch := seam.TriagePatchInput{Rating: json.RawMessage(`"not a number"`)}
	err := service.UpdateAssets(seam.UpdateTarget{IDs: []string{"a"}}, patch)
	assertDomainCode(t, err, "validation")
}

func TestDistinctValues_UnknownFieldMapsToQueryInvalid(t *testing.T) {
	service := seam.NewAssetService(&fakeAssets{}, &fakeAssets{})

	_, err := service.DistinctValues("nope")
	assertDomainCode(t, err, "query_invalid")
}

func TestDistinctValues_NonSuggestableFieldRejected(t *testing.T) {
	service := seam.NewAssetService(&fakeAssets{}, &fakeAssets{})

	// rating is a real field but not marked suggestable.
	_, err := service.DistinctValues("rating")
	assertDomainCode(t, err, "query_invalid")
}

func TestDistinctValues_SuggestableFieldReturnsValues(t *testing.T) {
	fake := &fakeAssets{distinct: []string{"Canon", "Nikon"}}
	service := seam.NewAssetService(fake, fake)

	got, err := service.DistinctValues("cameraMake")
	if err != nil {
		t.Fatalf("DistinctValues: %v", err)
	}
	if len(got) != 2 || got[0] != "Canon" {
		t.Fatalf("got %v", got)
	}
}

// --- Real-catalog wiring: prove the service against actual SQL, not a fake that
// echoes itself. These exercise the full path validate → sqlite repo → projection
// / persisted judgment, with the same repo construction app.go uses.

func TestQueryAssets_OverRealCatalog(t *testing.T) {
	db := testutil.NewTestDB(t)
	source := testutil.NewTestSource(t, db, "photos")
	testutil.NewTestAsset(t, db, source.ID, "a.jpg")
	testutil.NewTestAsset(t, db, source.ID, "b.jpg")
	repo := &sqlite.AssetRepo{DB: db}
	service := seam.NewAssetService(repo, repo)

	got, err := service.QueryAssets(validQuery(), filenameAsc, ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("QueryAssets: %v", err)
	}
	if got.Total != 2 || len(got.Items) != 2 {
		t.Fatalf("expected 2 real rows, got total=%d items=%d", got.Total, len(got.Items))
	}
	if got.Items[0].Filename != "a.jpg" {
		t.Fatalf("expected filename-asc order, got %q first", got.Items[0].Filename)
	}
}

func TestUpdateAssets_OverRealCatalog_PersistsJudgment(t *testing.T) {
	db := testutil.NewTestDB(t)
	source := testutil.NewTestSource(t, db, "photos")
	asset := testutil.NewTestAsset(t, db, source.ID, "a.jpg")
	repo := &sqlite.AssetRepo{DB: db}
	service := seam.NewAssetService(repo, repo)

	patch := seam.TriagePatchInput{Rating: json.RawMessage(`4`), ColorLabel: json.RawMessage(`"green"`)}
	if err := service.UpdateAssets(seam.UpdateTarget{IDs: []string{asset.ID}}, patch); err != nil {
		t.Fatalf("UpdateAssets: %v", err)
	}

	got, err := service.GetAsset(asset.ID)
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if got.Rating == nil || *got.Rating != 4 {
		t.Fatalf("rating not persisted: %+v", got.Rating)
	}
	if got.ColorLabel == nil || *got.ColorLabel != domain.ColorLabelGreen {
		t.Fatalf("color label not persisted: %+v", got.ColorLabel)
	}
	// D8: a judgment write must bump judgment_modified_at. The fresh test asset
	// leaves it NULL, so a non-nil value proves the judgment path ran (this write
	// went through AssetJudgmentWriter, the sole path that bumps it).
	if got.JudgmentModifiedAt == nil {
		t.Fatal("judgment_modified_at not bumped by the triage write (D8)")
	}
}

func TestRemoveFromCatalog_RepoErrorMapsToUnexpected(t *testing.T) {
	fake := &fakeAssets{err: errors.New("db down")}
	service := seam.NewAssetService(fake, fake)

	err := service.RemoveFromCatalog([]string{"a"})
	assertUnexpected(t, err)
}

func TestRemoveFromCatalog_OverRealCatalog_SoftDeletes(t *testing.T) {
	db := testutil.NewTestDB(t)
	source := testutil.NewTestSource(t, db, "photos")
	asset := testutil.NewTestAsset(t, db, source.ID, "a.jpg")
	repo := &sqlite.AssetRepo{DB: db}
	service := seam.NewAssetService(repo, repo)

	if err := service.RemoveFromCatalog([]string{asset.ID}); err != nil {
		t.Fatalf("RemoveFromCatalog: %v", err)
	}

	// The query compiler filters is_deleted = 0, so a soft-deleted asset leaves the
	// working set — proving the write actually landed, not just that no error came back.
	got, err := service.QueryAssets(validQuery(), filenameAsc, ast.Page{Limit: 10})
	if err != nil {
		t.Fatalf("QueryAssets: %v", err)
	}
	if got.Total != 0 {
		t.Fatalf("expected the soft-deleted asset gone from the working set, got total=%d", got.Total)
	}
}
