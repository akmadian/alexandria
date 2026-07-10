package seam_test

import (
	"context"
	"errors"
	"testing"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/seam"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
)

// fakeSources is an in-memory source repository for the service tests: it records
// the last Create/Update it saw and can inject an error on any call.
type fakeSources struct {
	list      []*domain.Source
	byID      map[string]*domain.Source
	created   *domain.Source
	updated   *domain.Source
	err       error
	updateErr error // injected on Update only, so Get can still succeed
}

func (f *fakeSources) List(context.Context) ([]*domain.Source, error) {
	return f.list, f.err
}

func (f *fakeSources) Get(_ context.Context, id string) (*domain.Source, error) {
	if f.err != nil {
		return nil, f.err
	}
	if source, ok := f.byID[id]; ok {
		return source, nil
	}
	return nil, &domain.NotFoundError{Resource: "source", ID: id}
}

func (f *fakeSources) Create(_ context.Context, source *domain.Source) error {
	if f.err != nil {
		return f.err
	}
	f.created = source
	return nil
}

func (f *fakeSources) Update(_ context.Context, source *domain.Source) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = source
	return nil
}

func TestListSources_ReturnsRepoResult(t *testing.T) {
	want := []*domain.Source{{ID: "s1", Name: "Photos"}}
	service := seam.NewSourceService(&fakeSources{list: want})

	got, err := service.ListSources()
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(got) != 1 || got[0].ID != "s1" {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestListSources_PropagatesError(t *testing.T) {
	service := seam.NewSourceService(&fakeSources{err: errors.New("db down")})

	_, err := service.ListSources()
	assertUnexpected(t, err)
}

func TestCreateSource_MintsEnabledOnlineSource(t *testing.T) {
	fake := &fakeSources{}
	service := seam.NewSourceService(fake)

	source, err := service.CreateSource(seam.SourceInput{Name: "Photos", BasePath: "/vol/photos"})
	if err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	if source.ID == "" {
		t.Fatal("expected a generated id")
	}
	if !source.Enabled || source.Connectivity != domain.SourceOnline {
		t.Fatalf("expected enabled+online, got enabled=%v connectivity=%q", source.Enabled, source.Connectivity)
	}
	if source.Kind != domain.SourceKindLocal {
		t.Fatalf("expected default local kind, got %q", source.Kind)
	}
	if fake.created == nil || fake.created.ID != source.ID {
		t.Fatal("expected the source to be persisted through the repo")
	}
}

func TestCreateSource_RejectsEmptyRequiredFields(t *testing.T) {
	service := seam.NewSourceService(&fakeSources{})

	_, err := service.CreateSource(seam.SourceInput{Name: "", BasePath: ""})
	assertDomainCode(t, err, "validation")
}

func TestUpdateSource_OverlaysProvidedFields(t *testing.T) {
	existing := &domain.Source{ID: "s1", Name: "Old", Enabled: true, ScanRecursively: true}
	fake := &fakeSources{byID: map[string]*domain.Source{"s1": existing}}
	service := seam.NewSourceService(fake)

	name := "New"
	disabled := false
	if err := service.UpdateSource("s1", seam.SourcePatch{Name: &name, Enabled: &disabled}); err != nil {
		t.Fatalf("UpdateSource: %v", err)
	}
	if fake.updated.Name != "New" || fake.updated.Enabled {
		t.Fatalf("overlay wrong: %+v", fake.updated)
	}
	if !fake.updated.ScanRecursively {
		t.Fatal("nil patch field must leave ScanRecursively untouched")
	}
}

func TestCreateSource_ConflictMapsToCode(t *testing.T) {
	fake := &fakeSources{err: &domain.ConflictError{Resource: "source", Field: "base_path", Message: "already exists"}}
	service := seam.NewSourceService(fake)

	_, err := service.CreateSource(seam.SourceInput{Name: "Photos", BasePath: "/vol/photos"})
	assertDomainCode(t, err, "conflict")
}

func TestUpdateSource_WriteErrorMapsToUnexpected(t *testing.T) {
	existing := &domain.Source{ID: "s1", Name: "Old"}
	fake := &fakeSources{
		byID:      map[string]*domain.Source{"s1": existing},
		updateErr: errors.New("db down"),
	}
	service := seam.NewSourceService(fake)

	name := "New"
	err := service.UpdateSource("s1", seam.SourcePatch{Name: &name})
	assertUnexpected(t, err)
}

func TestUpdateSource_UnknownIDMapsToNotFound(t *testing.T) {
	service := seam.NewSourceService(&fakeSources{byID: map[string]*domain.Source{}})

	name := "x"
	err := service.UpdateSource("missing", seam.SourcePatch{Name: &name})
	assertDomainCode(t, err, "not_found")
}

// The walking skeleton over a real migrated catalog: the seam service, wired to
// the actual sqlite.SourceRepo (the same construction main.go/app.go use), must
// return the rows the repo holds — proving the pipe end to end on the Go side.
func TestListSources_OverRealCatalog(t *testing.T) {
	db := testutil.NewTestDB(t)
	seeded := testutil.NewTestSource(t, db, "photos")
	service := seam.NewSourceService(&sqlite.SourceRepo{DB: db})

	got, err := service.ListSources()
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(got) != 1 || got[0].ID != seeded.ID || got[0].Name != "photos" {
		t.Fatalf("got %+v, want the seeded source %q", got, seeded.ID)
	}
}
