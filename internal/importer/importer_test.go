package importer_test

import (
	"context"
	"io"
	"os"
	"testing"
	"testing/fstest"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/importer"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
	"github.com/charmbracelet/log"
)

func newImporter(t *testing.T) (*importer.Importer, *domain.Source, *sqlite.AssetRepo) {
	t.Helper()
	db := testutil.NewTestDB(t)
	src := testutil.NewTestSource(t, db, "photos")
	assets := &sqlite.AssetRepo{DB: db}
	imp := &importer.Importer{
		Assets: assets,
		Dups:   &sqlite.DuplicateRepo{DB: db},
		Log:    log.New(io.Discard), // injected quiet logger — no test output noise
	}
	return imp, src, assets
}

func TestRun_IndexesSupportedFilesOnly(t *testing.T) {
	imp, src, assets := newImporter(t)
	fsys := fstest.MapFS{
		"a.jpg":       {Data: []byte("jpeg-a")},
		"b.png":       {Data: []byte("png-b")},
		"sub/c.mp4":   {Data: []byte("video-c")},
		"notes.txt":   {Data: []byte("unsupported")},
		".hidden.jpg": {Data: []byte("hidden")},
	}
	res, err := imp.Run(context.Background(), src, fsys)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Added != 3 {
		t.Fatalf("added=%d, want 3 (jpg, png, mp4)", res.Added)
	}
	got, _ := assets.List(context.Background(), catalog.AssetFilter{})
	if len(got) != 3 {
		t.Fatalf("catalog has %d assets, want 3", len(got))
	}
}

func TestRun_Idempotent(t *testing.T) {
	imp, src, _ := newImporter(t)
	fsys := fstest.MapFS{
		"a.jpg": {Data: []byte("jpeg-a")},
		"b.png": {Data: []byte("png-b")},
	}
	if _, err := imp.Run(context.Background(), src, fsys); err != nil {
		t.Fatalf("first run: %v", err)
	}
	res, err := imp.Run(context.Background(), src, fsys)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Added != 0 || res.Skipped != 2 {
		t.Fatalf("second run: added=%d skipped=%d, want 0/2", res.Added, res.Skipped)
	}
}

// Exercises the real files on disk in testdata/ (per the "keep real-file tests"
// requirement) — os.DirFS is just another fs.FS.
func TestRun_RealFilesOnDisk(t *testing.T) {
	imp, src, assets := newImporter(t)
	fsys := os.DirFS("../../testdata")

	res, err := imp.Run(context.Background(), src, fsys)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Added != 3 {
		t.Fatalf("added=%d, want 3 sample JPEGs (the .txt in subdir is unsupported)", res.Added)
	}
	got, _ := assets.List(context.Background(), catalog.AssetFilter{})
	for _, a := range got {
		if a.PartialHash == "" {
			t.Errorf("asset %s has no hash", a.RelativePath)
		}
	}

	// Idempotent on real files too (mtime tolerance absorbs RFC3339 truncation).
	res2, err := imp.Run(context.Background(), src, fsys)
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if res2.Added != 0 || res2.Skipped != 3 {
		t.Fatalf("re-run: added=%d skipped=%d, want 0/3", res2.Added, res2.Skipped)
	}
}
