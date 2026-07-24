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

// fakeVolumes is an in-memory volume repository for the service tests: it records
// the last Update it saw and can inject an error on any call.
type fakeVolumes struct {
	list      []*domain.Volume
	byID      map[string]*domain.Volume
	updated   *domain.Volume
	err       error
	updateErr error // injected on Update only, so Get can still succeed
}

func (f *fakeVolumes) List(context.Context) ([]*domain.Volume, error) {
	return f.list, f.err
}

func (f *fakeVolumes) Get(_ context.Context, id string) (*domain.Volume, error) {
	if f.err != nil {
		return nil, f.err
	}
	if volume, ok := f.byID[id]; ok {
		return volume, nil
	}
	return nil, &domain.NotFoundError{Resource: "volume", ID: id}
}

func (f *fakeVolumes) Update(_ context.Context, volume *domain.Volume) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = volume
	return nil
}

func TestListVolumes_ReturnsRepoResult(t *testing.T) {
	want := []*domain.Volume{{ID: "v1", Name: "Photos"}}
	service := seam.NewVolumeService(&fakeVolumes{list: want})

	got, err := service.ListVolumes()
	if err != nil {
		t.Fatalf("ListVolumes: %v", err)
	}
	if len(got) != 1 || got[0].ID != "v1" {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestListVolumes_PropagatesError(t *testing.T) {
	service := seam.NewVolumeService(&fakeVolumes{err: errors.New("db down")})

	_, err := service.ListVolumes()
	assertUnexpected(t, err)
}

func TestUpdateVolume_OverlaysName(t *testing.T) {
	existing := &domain.Volume{ID: "v1", Name: "Old", Kind: domain.VolumeKindLocal}
	fake := &fakeVolumes{byID: map[string]*domain.Volume{"v1": existing}}
	service := seam.NewVolumeService(fake)

	name := "New"
	if err := service.UpdateVolume("v1", seam.VolumePatch{Name: &name}); err != nil {
		t.Fatalf("UpdateVolume: %v", err)
	}
	if fake.updated.Name != "New" {
		t.Fatalf("overlay wrong: %+v", fake.updated)
	}
	if fake.updated.Kind != domain.VolumeKindLocal {
		t.Fatal("nil patch field must leave Kind untouched")
	}
}

func TestUpdateVolume_WriteErrorMapsToUnexpected(t *testing.T) {
	existing := &domain.Volume{ID: "v1", Name: "Old"}
	fake := &fakeVolumes{
		byID:      map[string]*domain.Volume{"v1": existing},
		updateErr: errors.New("db down"),
	}
	service := seam.NewVolumeService(fake)

	name := "New"
	err := service.UpdateVolume("v1", seam.VolumePatch{Name: &name})
	assertUnexpected(t, err)
}

func TestUpdateVolume_UnknownIDMapsToNotFound(t *testing.T) {
	service := seam.NewVolumeService(&fakeVolumes{byID: map[string]*domain.Volume{}})

	name := "x"
	err := service.UpdateVolume("missing", seam.VolumePatch{Name: &name})
	assertDomainCode(t, err, "not_found")
}

// The walking skeleton over a real migrated catalog: the seam service, wired to
// the actual sqlite.VolumeRepo, must return the rows the repo holds — proving the
// pipe end to end on the Go side.
func TestListVolumes_OverRealCatalog(t *testing.T) {
	db := testutil.NewTestDB(t)
	seeded := testutil.NewTestVolume(t, db, "photos")
	service := seam.NewVolumeService(&sqlite.VolumeRepo{DB: db})

	got, err := service.ListVolumes()
	if err != nil {
		t.Fatalf("ListVolumes: %v", err)
	}
	if len(got) != 1 || got[0].ID != seeded.ID || got[0].Name != "photos" {
		t.Fatalf("got %+v, want the seeded volume %q", got, seeded.ID)
	}
}
