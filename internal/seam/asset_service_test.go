package seam_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

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

func TestGetAsset_ProjectsFullDetail(t *testing.T) {
	// Every same-typed field PAIR carries two DISTINCT values (width≠height,
	// lat≠lon, title≠caption, creator≠copyright, make≠model) so a transposed
	// assignment in the projection fails here instead of passing silently.
	capturedAt := time.Date(2026, 5, 21, 20, 56, 10, 0, time.UTC)
	rating, aperture, iso := 4, 3.2, 400
	width, height := 7728, 5152
	gpsLat, gpsLon := 47.6062, -122.3321
	lens := "SIGMA 18-50mm F2.8 DC DN Contemporary 021"
	cameraMake, cameraModel := "Fujifilm", "X-T5"
	title, caption := "Loowit from Pahto", "The mountain at dusk"
	creator, copyright := "Ari Madian", "ALL RIGHTS RESERVED"
	label := domain.ColorLabelGreen
	fake := &fakeAssets{asset: &domain.Asset{
		ID: "a1", VolumeID: "s1", Filename: "_DSF4926.RAF", Extension: "RAF",
		MIMEType: "image/x-fuji-raf", FileType: domain.FileTypeRaw,
		FileStatus: domain.FileStatusOnline, RelativePath: "Adams 2026/_DSF4926.RAF",
		SizeBytes: 81_140_000,
		Width:     &width, Height: &height,
		CapturedAt: &capturedAt, LensModel: &lens, Aperture: &aperture, ISO: &iso,
		CameraMake: &cameraMake, CameraModel: &cameraModel,
		GPSLat: &gpsLat, GPSLon: &gpsLon,
		Title: &title, Caption: &caption, Creator: &creator, Copyright: &copyright,
		Rating: &rating, ColorLabel: &label,
		ExtendedMetadata: map[string]any{"EXIF:Flash": "Did not fire"},
	}}
	service := seam.NewAssetService(fake, fake)

	got, err := service.GetAsset("a1")
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if got.ID != "a1" || got.Filename != "_DSF4926.RAF" || got.FileType != domain.FileTypeRaw {
		t.Fatalf("file facts not projected: %+v", got)
	}
	if got.CapturedAt == nil || !got.CapturedAt.Equal(capturedAt) {
		t.Fatalf("capturedAt not projected: %+v", got.CapturedAt)
	}
	if got.Width == nil || *got.Width != width || got.Height == nil || *got.Height != height {
		t.Fatalf("dimensions transposed or dropped: width=%v height=%v", got.Width, got.Height)
	}
	if got.GPSLat == nil || *got.GPSLat != gpsLat || got.GPSLon == nil || *got.GPSLon != gpsLon {
		t.Fatalf("gps transposed or dropped: lat=%v lon=%v", got.GPSLat, got.GPSLon)
	}
	if got.CameraMake == nil || *got.CameraMake != cameraMake || got.CameraModel == nil || *got.CameraModel != cameraModel {
		t.Fatalf("camera transposed or dropped: make=%v model=%v", got.CameraMake, got.CameraModel)
	}
	if got.Title == nil || *got.Title != title || got.Caption == nil || *got.Caption != caption {
		t.Fatalf("title/caption transposed or dropped: title=%v caption=%v", got.Title, got.Caption)
	}
	if got.Creator == nil || *got.Creator != creator || got.Copyright == nil || *got.Copyright != copyright {
		t.Fatalf("creator/copyright transposed or dropped: creator=%v copyright=%v", got.Creator, got.Copyright)
	}
	if got.LensModel == nil || *got.LensModel != lens {
		t.Fatalf("lens not projected: %+v", got.LensModel)
	}
	if got.Aperture == nil || *got.Aperture != aperture || got.ISO == nil || *got.ISO != iso {
		t.Fatalf("exposure fields not projected: aperture=%v iso=%v", got.Aperture, got.ISO)
	}
	if got.Rating == nil || *got.Rating != rating || got.ColorLabel == nil || *got.ColorLabel != label {
		t.Fatalf("judgment not projected: rating=%v label=%v", got.Rating, got.ColorLabel)
	}
	if got.ExtendedMetadata["EXIF:Flash"] != "Did not fire" {
		t.Fatalf("extended metadata not passed through: %+v", got.ExtendedMetadata)
	}
}

func TestGetAsset_NilFieldsStayNil(t *testing.T) {
	fake := &fakeAssets{asset: &domain.Asset{
		ID: "a2", VolumeID: "s1", Filename: "scan.jpg", Extension: "jpg",
		MIMEType: "image/jpeg", FileType: domain.FileTypeImage,
		FileStatus: domain.FileStatusOnline, RelativePath: "scans/scan.jpg",
	}}
	service := seam.NewAssetService(fake, fake)

	got, err := service.GetAsset("a2")
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	// An undated scan with no extraction carries nulls, never zero values
	// (4a: unrated = NULL end to end; same discipline for every optional field).
	if got.CapturedAt != nil || got.CameraMake != nil || got.Rating != nil ||
		got.Flag != nil || got.Note != nil || got.GPSLat != nil {
		t.Fatalf("expected nil optionals, got %+v", got)
	}
	if got.ExtendedMetadata != nil {
		t.Fatalf("expected nil extended metadata, got %+v", got.ExtendedMetadata)
	}
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
	// A real patch (empty patches are now a no-op) so the by-query write path runs.
	err := service.UpdateAssets(seam.UpdateTarget{Query: &query, ExceptIDs: []string{"skip"}}, seam.TriagePatchInput{Rating: json.RawMessage(`3`)})
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
	source := testutil.NewTestVolume(t, db, "photos")
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
	source := testutil.NewTestVolume(t, db, "photos")
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
	// D8: a judgment write must bump judgment_modified_at. The wire projection
	// deliberately omits that bookkeeping column, so probe the repo directly —
	// the fresh test asset leaves it NULL, so a non-nil value proves the write
	// went through AssetJudgmentWriter, the sole path that bumps it.
	stored, err := repo.Get(context.Background(), asset.ID)
	if err != nil {
		t.Fatalf("repo.Get: %v", err)
	}
	if stored.JudgmentModifiedAt == nil {
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
	source := testutil.NewTestVolume(t, db, "photos")
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

// --- catalog/changed emission (impl/16) ------------------------------------

func TestUpdateAssets_EmitsCatalogChanged(t *testing.T) {
	fake := &fakeAssets{}
	emitter := &fakeEmitter{}
	service := seam.NewAssetService(fake, fake, seam.WithEmitter(emitter))

	patch := seam.TriagePatchInput{Rating: json.RawMessage(`5`)}
	if err := service.UpdateAssets(seam.UpdateTarget{IDs: []string{"a"}}, patch); err != nil {
		t.Fatalf("UpdateAssets: %v", err)
	}
	events := emitter.snapshot()
	if len(events) != 1 || events[0].Type != seam.EventCatalogChanged {
		t.Fatalf("want one catalog/changed, got %v", emitter.typesOf())
	}
	change, ok := events[0].Payload.(seam.CatalogChange)
	if !ok || change.Scope != seam.ScopeAssets {
		t.Fatalf("want assets-scoped change, got %+v", events[0].Payload)
	}
}

func TestUpdateAssets_EmptyPatchIsNoOpAndEmitsNothing(t *testing.T) {
	fake := &fakeAssets{}
	emitter := &fakeEmitter{}
	service := seam.NewAssetService(fake, fake, seam.WithEmitter(emitter))

	// An all-absent patch touches nothing: no write, no event.
	if err := service.UpdateAssets(seam.UpdateTarget{IDs: []string{"a"}}, seam.TriagePatchInput{}); err != nil {
		t.Fatalf("UpdateAssets: %v", err)
	}
	if fake.gotIDs != nil {
		t.Fatalf("empty patch should not reach the writer, got ids %v", fake.gotIDs)
	}
	if len(emitter.snapshot()) != 0 {
		t.Fatalf("a no-op write must emit nothing, got %v", emitter.typesOf())
	}
}

func TestUpdateAssets_ByQueryAffectingNoRowsEmitsNothing(t *testing.T) {
	fake := &fakeAssets{byQueryResult: nil} // zero rows affected
	emitter := &fakeEmitter{}
	service := seam.NewAssetService(fake, fake, seam.WithEmitter(emitter))

	query := validQuery()
	patch := seam.TriagePatchInput{Rating: json.RawMessage(`3`)}
	if err := service.UpdateAssets(seam.UpdateTarget{Query: &query}, patch); err != nil {
		t.Fatalf("UpdateAssets: %v", err)
	}
	if len(emitter.snapshot()) != 0 {
		t.Fatalf("a query that moved no rows must emit nothing, got %v", emitter.typesOf())
	}
}

func TestRemoveFromCatalog_EmitsCatalogChanged(t *testing.T) {
	fake := &fakeAssets{}
	emitter := &fakeEmitter{}
	service := seam.NewAssetService(fake, fake, seam.WithEmitter(emitter))

	if err := service.RemoveFromCatalog([]string{"a"}); err != nil {
		t.Fatalf("RemoveFromCatalog: %v", err)
	}
	if types := emitter.typesOf(); len(types) != 1 || types[0] != seam.EventCatalogChanged {
		t.Fatalf("want one catalog/changed, got %v", types)
	}
}

func TestRemoveFromCatalog_EmptyIsNoOpAndEmitsNothing(t *testing.T) {
	fake := &fakeAssets{}
	emitter := &fakeEmitter{}
	service := seam.NewAssetService(fake, fake, seam.WithEmitter(emitter))

	if err := service.RemoveFromCatalog(nil); err != nil {
		t.Fatalf("RemoveFromCatalog: %v", err)
	}
	if fake.gotIDs != nil {
		t.Fatalf("empty remove should not reach the writer, got %v", fake.gotIDs)
	}
	if len(emitter.snapshot()) != 0 {
		t.Fatalf("empty remove must emit nothing, got %v", emitter.typesOf())
	}
}

func TestUpdateAssets_ByQueryEmptyPatchIsNoOpAndEmitsNothing(t *testing.T) {
	fake := &fakeAssets{}
	emitter := &fakeEmitter{}
	service := seam.NewAssetService(fake, fake, seam.WithEmitter(emitter))

	query := validQuery()
	// Empty patch on the by-query branch: validated, then short-circuited — the
	// writer is never called and nothing is emitted.
	if err := service.UpdateAssets(seam.UpdateTarget{Query: &query}, seam.TriagePatchInput{}); err != nil {
		t.Fatalf("UpdateAssets: %v", err)
	}
	if fake.gotQuery != nil {
		t.Fatalf("empty-patch by-query should not reach the writer, got %+v", fake.gotQuery)
	}
	if len(emitter.snapshot()) != 0 {
		t.Fatalf("empty-patch by-query must emit nothing, got %v", emitter.typesOf())
	}
}
