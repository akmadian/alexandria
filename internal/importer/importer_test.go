package importer_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/akmadian/alexandria/internal/assettype"
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
		Reader:  assets,
		Obs:     assets,
		Derived: assets,
		Dups:    &sqlite.DuplicateRepo{DB: db},
		Log:     log.New(io.Discard), // injected quiet logger — no test output noise
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
	// Derive the expected count from the fixtures actually present, so adding or
	// removing sample files doesn't break the test. Count every supported type
	// (any case, any extension) — that's exactly what the importer indexes.
	want := 0
	filepath.WalkDir("../../testdata", func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && assettype.IsSupported(filepath.Ext(d.Name())) {
			want++
		}
		return nil
	})
	if want == 0 {
		t.Skip("no supported fixtures in testdata/")
	}

	res, err := imp.Run(context.Background(), src, fsys)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Added != want {
		t.Fatalf("added=%d, want %d JPEGs (the .txt in subdir is unsupported)", res.Added, want)
	}
	got, _ := assets.List(context.Background(), catalog.AssetFilter{})
	for _, a := range got {
		if a.PartialHash == "" {
			t.Errorf("asset %s has no hash", a.RelativePath)
		}
	}

	// Metadata extraction ran through the pipeline: real files carry dimensions
	// and rights (these fixtures are exports without camera EXIF).
	sample, err := assets.FindBySourcePath(context.Background(), src.ID, "_6160345-.jpg")
	if err != nil || sample == nil {
		t.Fatalf("sample asset: %v", err)
	}
	if sample.Width == nil || *sample.Width != 2160 || sample.Height == nil || *sample.Height != 1620 {
		t.Errorf("dimensions not extracted end-to-end: %v x %v", sample.Width, sample.Height)
	}
	if sample.Creator == nil || *sample.Creator != "Ari Madian" {
		t.Errorf("creator not extracted end-to-end: %v", sample.Creator)
	}

	// Idempotent on real files too (mtime tolerance absorbs RFC3339 truncation).
	res2, err := imp.Run(context.Background(), src, fsys)
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if res2.Added != 0 || res2.Skipped != want {
		t.Fatalf("re-run: added=%d skipped=%d, want 0/%d", res2.Added, res2.Skipped, want)
	}
}

func TestReconcile_MarksMissingAndRestores(t *testing.T) {
	imp, src, assets := newImporter(t)
	ctx := context.Background()
	full := fstest.MapFS{
		"a.jpg": {Data: []byte("a")},
		"b.jpg": {Data: []byte("b")},
	}
	if _, err := imp.Run(ctx, src, full); err != nil {
		t.Fatalf("import: %v", err)
	}

	// b.jpg vanished from disk.
	res, err := imp.Reconcile(ctx, src, fstest.MapFS{"a.jpg": full["a.jpg"]})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.Missing != 1 || res.Restored != 0 {
		t.Fatalf("reconcile: missing=%d restored=%d, want 1/0", res.Missing, res.Restored)
	}
	b, _ := assets.FindBySourcePath(ctx, src.ID, "b.jpg")
	if b == nil || b.FileStatus != domain.FileStatusMissing {
		t.Fatalf("b.jpg status = %v, want missing", b)
	}

	// b.jpg came back.
	res, err = imp.Reconcile(ctx, src, full)
	if err != nil {
		t.Fatalf("reconcile back: %v", err)
	}
	if res.Restored != 1 {
		t.Fatalf("reconcile: restored=%d, want 1", res.Restored)
	}
	if b, _ = assets.FindBySourcePath(ctx, src.ID, "b.jpg"); b.FileStatus != domain.FileStatusOnline {
		t.Fatalf("b.jpg status = %q, want online", b.FileStatus)
	}
}

func TestReconcile_SourceOfflineMarksAllOffline(t *testing.T) {
	imp, src, assets := newImporter(t)
	ctx := context.Background()
	if _, err := imp.Run(ctx, src, fstest.MapFS{"a.jpg": {Data: []byte("a")}}); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Unreachable filesystem: a nonexistent directory.
	res, err := imp.Reconcile(ctx, src, os.DirFS(filepath.Join(t.TempDir(), "gone")))
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.Missing != 0 {
		t.Fatalf("offline source must not mark files missing, got missing=%d", res.Missing)
	}
	a, _ := assets.FindBySourcePath(ctx, src.ID, "a.jpg")
	if a.FileStatus != domain.FileStatusOffline {
		t.Fatalf("a.jpg status = %q, want offline", a.FileStatus)
	}
}

// The capstone: reconcile + import together relink a moved file instead of
// duplicating it — the pipeline's most important protection, now active.
func TestMoveRelink_AfterReconcile(t *testing.T) {
	imp, src, assets := newImporter(t)
	ctx := context.Background()
	content := []byte("the same photo bytes")

	if _, err := imp.Run(ctx, src, fstest.MapFS{"old/photo.jpg": {Data: content}}); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Folder reorganized while the app was closed: the old path is gone.
	if _, err := imp.Reconcile(ctx, src, fstest.MapFS{}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// Re-import finds the same file (same name + content) at a new path.
	res, err := imp.Run(ctx, src, fstest.MapFS{"new/photo.jpg": {Data: content}})
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if res.Moved != 1 || res.Added != 0 {
		t.Fatalf("re-import: moved=%d added=%d, want 1/0 (relink, not duplicate)", res.Moved, res.Added)
	}
	all, _ := assets.List(ctx, catalog.AssetFilter{IncludeDeleted: true})
	if len(all) != 1 {
		t.Fatalf("expected 1 relinked asset, got %d (duplicated?)", len(all))
	}
	if all[0].RelativePath != "new/photo.jpg" || all[0].FileStatus != domain.FileStatusOnline {
		t.Fatalf("relinked asset at %q status %q, want new/photo.jpg online",
			all[0].RelativePath, all[0].FileStatus)
	}
}
