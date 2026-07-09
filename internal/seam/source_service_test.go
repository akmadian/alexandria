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

type fakeSources struct {
	list []*domain.Source
	err  error
}

func (f *fakeSources) List(context.Context) ([]*domain.Source, error) {
	return f.list, f.err
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
	wantErr := errors.New("db down")
	service := seam.NewSourceService(&fakeSources{err: wantErr})

	_, err := service.ListSources()
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want %v", err, wantErr)
	}
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
