package importer_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/akmadian/alexandria/internal/assettype"
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
		Store:   sqlite.NewStore(db),
		Imports: &sqlite.ImportRepo{DB: db},
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
	var count int
	if err := assets.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM assets WHERE is_deleted=0").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("catalog has %d assets, want 3", count)
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
	if err := filepath.WalkDir("../../testdata", func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && assettype.IsSupported(filepath.Ext(d.Name())) {
			want++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
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
	rows, _ := assets.DB.QueryContext(context.Background(), "SELECT relative_path, partial_hash FROM assets WHERE is_deleted=0")
	defer rows.Close()
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			t.Fatal(err)
		}
		if hash == "" {
			t.Errorf("asset %s has no hash", path)
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

// TestIngestFile_GonePathMarksMissing proves the impl/05 corrected model: a
// watcher-fed path that no longer exists is marked missing by the IMPORTER's
// single-path entry (the watcher hands over a path, never a verdict). This is the
// same action the walk-end diff takes, in one place.
func TestIngestFile_GonePathMarksMissing(t *testing.T) {
	imp, src, assets := newImporter(t)
	ctx := context.Background()
	if _, err := imp.Run(ctx, src, fstest.MapFS{"a.jpg": {Data: []byte("a")}}); err != nil {
		t.Fatalf("import: %v", err)
	}

	// The file is gone; the watcher feeds its path to the single-path entry.
	if err := imp.IngestFile(ctx, src, fstest.MapFS{}, "a.jpg"); err != nil {
		t.Fatalf("ingest gone path: %v", err)
	}
	a, _ := assets.FindBySourcePath(ctx, src.ID, "a.jpg")
	if a == nil || a.FileStatus != domain.FileStatusMissing {
		t.Fatalf("a.jpg status = %v, want missing", a)
	}

	// A second gone-event for the same (now-missing) path is a no-op, not an error.
	if err := imp.IngestFile(ctx, src, fstest.MapFS{}, "a.jpg"); err != nil {
		t.Fatalf("repeat gone path: %v", err)
	}
	// An unknown gone path is a no-op too.
	if err := imp.IngestFile(ctx, src, fstest.MapFS{}, "never-seen.jpg"); err != nil {
		t.Fatalf("unknown gone path: %v", err)
	}
}

// TestWalk_MarksMissingThenRestores proves the full walk (Run) subsumes the old
// standalone reconcile (retired in impl/05.3): an unvisited known file is marked
// missing, and reappears online on the next walk (reimport restores it at its
// original path — D20, no relink). Whole-source-offline is no longer the importer's
// job — the watcher's poll monitor owns it (see internal/watcher poll tests).
func TestWalk_MarksMissingThenRestores(t *testing.T) {
	imp, src, assets := newImporter(t)
	ctx := context.Background()
	full := fstest.MapFS{
		"a.jpg": {Data: []byte("a")},
		"b.jpg": {Data: []byte("b")},
	}
	if _, err := imp.Run(ctx, src, full); err != nil {
		t.Fatalf("import: %v", err)
	}

	// b.jpg vanished: a walk that doesn't visit it marks it missing.
	res, err := imp.Run(ctx, src, fstest.MapFS{"a.jpg": full["a.jpg"]})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if res.Missing != 1 {
		t.Fatalf("walk: missing=%d, want 1", res.Missing)
	}
	reloaded, _ := assets.FindBySourcePath(ctx, src.ID, "b.jpg")
	if reloaded == nil || reloaded.FileStatus != domain.FileStatusMissing {
		t.Fatalf("b.jpg status = %v, want missing", reloaded)
	}

	// b.jpg came back at its original path: reimport restores it online.
	if _, err := imp.Run(ctx, src, full); err != nil {
		t.Fatalf("walk back: %v", err)
	}
	if reloaded, _ = assets.FindBySourcePath(ctx, src.ID, "b.jpg"); reloaded.FileStatus != domain.FileStatusOnline {
		t.Fatalf("b.jpg status = %q, want online", reloaded.FileStatus)
	}
}

// TestWalk_FolderReorgRecordsMove proves D20 for the walk path: a file moved to a
// new dir (folder reorg) is NOT relinked — the old path goes missing, the new path
// is a distinct asset, and the pair is a pending review row for the user to confirm.
func TestWalk_FolderReorgRecordsMove(t *testing.T) {
	imp, src, assets := newImporter(t)
	ctx := context.Background()
	content := []byte("the same photo bytes")

	if _, err := imp.Run(ctx, src, fstest.MapFS{"old/photo.jpg": {Data: content}}); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Folder reorganized while the app was closed: old path gone, same file at a new path.
	res, err := imp.Run(ctx, src, fstest.MapFS{"new/photo.jpg": {Data: content}})
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if res.Added != 1 || res.Missing != 1 {
		t.Fatalf("folder reorg must mint the new path (1) + mark old missing (1), got added=%d missing=%d", res.Added, res.Missing)
	}
	var count int
	if err := assets.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM assets WHERE is_deleted=0").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("no relink: expected 2 distinct assets, got %d", count)
	}
	dups, _ := (&sqlite.DuplicateRepo{DB: assets.DB}).ListPending(ctx)
	if len(dups) != 1 {
		t.Fatalf("the move must be recorded as one pending review pair, got %d", len(dups))
	}
}
