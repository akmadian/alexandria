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

// TestIngestFile_GoneHealsCopyThenDelete proves the single-path gone branch runs
// the SAME delete-side merge as the walk: ingest the copy, then feed the original
// as gone → the original identity is preserved (judgment intact) and adopts the
// copy's path, leaving no stranded missing row.
func TestIngestFile_GoneHealsCopyThenDelete(t *testing.T) {
	imp, src, assets := newImporter(t)
	ctx := context.Background()
	content := []byte("photo-moved-by-copy-then-delete")

	// Original with a user rating (what must survive), then the copy is ingested.
	if _, err := imp.Run(ctx, src, fstest.MapFS{"a/photo.jpg": {Data: content}}); err != nil {
		t.Fatalf("import: %v", err)
	}
	original, _ := assets.FindBySourcePath(ctx, src.ID, "a/photo.jpg")
	if err := assets.ApplyTriagePatch(ctx, []string{original.ID}, catalog.TriagePatch{Rating: domain.SetOpt(5)}); err != nil {
		t.Fatalf("rate: %v", err)
	}
	if err := imp.IngestFile(ctx, src, fstest.MapFS{"b/photo.jpg": {Data: content}}, "b/photo.jpg"); err != nil {
		t.Fatalf("ingest copy: %v", err)
	}

	// The old path is now gone — fed to the single-path entry, it heals the move.
	if err := imp.IngestFile(ctx, src, fstest.MapFS{"b/photo.jpg": {Data: content}}, "a/photo.jpg"); err != nil {
		t.Fatalf("ingest gone original: %v", err)
	}

	all, _ := assets.List(ctx, catalog.AssetFilter{})
	if len(all) != 1 {
		t.Fatalf("copy must be absorbed, not left as a second identity: got %d assets, want 1", len(all))
	}
	survivor := all[0]
	if survivor.ID != original.ID {
		t.Fatalf("original identity must survive: got %s, want %s", survivor.ID, original.ID)
	}
	if survivor.RelativePath != "b/photo.jpg" || survivor.FileStatus != domain.FileStatusOnline {
		t.Fatalf("survivor should be online at the new path: path=%s status=%s", survivor.RelativePath, survivor.FileStatus)
	}
	if survivor.Rating == nil || *survivor.Rating != 5 {
		t.Fatalf("the user's rating must be preserved through the merge: got %v", survivor.Rating)
	}
}

// TestWalk_MarksMissingThenRestores proves the full walk (Run) subsumes the old
// standalone reconcile (retired in impl/05.3): an unvisited known file is marked
// missing, and reappears online on the next walk (the matrix relinks it).
// Whole-source-offline is no longer the importer's job — the watcher's poll
// monitor owns it (see internal/watcher poll tests).
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
	b, _ := assets.FindBySourcePath(ctx, src.ID, "b.jpg")
	if b == nil || b.FileStatus != domain.FileStatusMissing {
		t.Fatalf("b.jpg status = %v, want missing", b)
	}

	// b.jpg came back: the next walk relinks it online (same content+name).
	if _, err := imp.Run(ctx, src, full); err != nil {
		t.Fatalf("walk back: %v", err)
	}
	if b, _ = assets.FindBySourcePath(ctx, src.ID, "b.jpg"); b.FileStatus != domain.FileStatusOnline {
		t.Fatalf("b.jpg status = %q, want online", b.FileStatus)
	}
}

// The capstone: a walk after a folder reorg relinks a moved file instead of
// duplicating it — the pipeline's most important protection.
func TestMoveRelink_AfterReconcile(t *testing.T) {
	imp, src, assets := newImporter(t)
	ctx := context.Background()
	content := []byte("the same photo bytes")

	if _, err := imp.Run(ctx, src, fstest.MapFS{"old/photo.jpg": {Data: content}}); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Folder reorganized while the app was closed: the old path is gone, so a walk
	// that doesn't visit it marks it missing (the relink target).
	if _, err := imp.Run(ctx, src, fstest.MapFS{}); err != nil {
		t.Fatalf("walk: %v", err)
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
