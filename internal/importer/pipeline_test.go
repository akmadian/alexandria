package importer_test

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/importer"
	"github.com/akmadian/alexandria/internal/migrations"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
	"github.com/akmadian/alexandria/internal/thumbnailer"
	"github.com/charmbracelet/log"
)

// newPipelineImporter is like newImporter but returns the import repo (so tests
// can read DLQ/session rows) and optionally wires a real thumbnailer.
func newPipelineImporter(t *testing.T, thumbnailDir string) (*importer.Importer, *domain.Source, *sqlite.AssetRepo, *sqlite.ImportRepo) {
	t.Helper()
	database := testutil.NewTestDB(t)
	source := testutil.NewTestSource(t, database, "photos")
	assets := &sqlite.AssetRepo{DB: database}
	imports := &sqlite.ImportRepo{DB: database}
	ingester := &importer.Importer{
		Reader:  assets,
		Obs:     assets,
		Derived: assets,
		Dups:    &sqlite.DuplicateRepo{DB: database},
		Store:   sqlite.NewStore(database),
		Imports: imports,
		Log:     log.New(io.Discard),
	}
	if thumbnailDir != "" {
		ingester.Thumbnail = thumbnailer.New(thumbnailDir)
	}
	return ingester, source, assets, imports
}

// jpegBytes returns a small, valid, decodable JPEG whose bytes vary by seed, so
// distinct seeds produce distinct partial hashes.
func jpegBytes(seed int) []byte {
	pixels := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range pixels.Pix {
		pixels.Pix[i] = byte((i*7 + seed*131) % 256)
	}
	var buffer bytes.Buffer
	_ = jpeg.Encode(&buffer, pixels, &jpeg.Options{Quality: 90})
	return buffer.Bytes()
}

// --- Matrix: the five verdicts ---

func TestMatrix_NewReimportDuplicate(t *testing.T) {
	ingester, source, _, _ := newPipelineImporter(t, "")
	ctx := context.Background()

	// (5) New.
	result, err := ingester.Run(ctx, source, fstest.MapFS{"a.mp4": {Data: []byte("content-A")}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 1 {
		t.Fatalf("new: added=%d, want 1", result.Added)
	}

	// (3) Reimport — same path, changed content.
	result, err = ingester.Run(ctx, source, fstest.MapFS{"a.mp4": {Data: []byte("content-A-edited-larger")}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || result.Added != 0 {
		t.Fatalf("reimport: updated=%d added=%d, want 1/0", result.Updated, result.Added)
	}

	// (4) Duplicate — same content as a PRESENT asset, new path.
	result, err = ingester.Run(ctx, source, fstest.MapFS{
		"a.mp4":    {Data: []byte("content-A-edited-larger")},
		"copy.mp4": {Data: []byte("content-A-edited-larger")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Dups != 1 || result.Skipped != 1 {
		t.Fatalf("duplicate: dups=%d skipped=%d, want 1/1", result.Dups, result.Skipped)
	}
}

func TestMatrix_InRunDuplicatePair(t *testing.T) {
	// A copy exists BEFORE the original is committed: the in-run hash map must
	// catch it (FindByHash can't — the original isn't committed yet).
	ingester, source, assets, _ := newPipelineImporter(t, "")
	ctx := context.Background()

	content := []byte("burst-frame-identical-bytes")
	result, err := ingester.Run(ctx, source, fstest.MapFS{
		"shoot/img_1.mp4": {Data: content},
		"shoot/img_2.mp4": {Data: content},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 2 || result.Dups != 1 {
		t.Fatalf("in-run pair: added=%d dups=%d, want 2/1", result.Added, result.Dups)
	}
	allAssets, _ := assets.List(ctx, catalog.AssetFilter{})
	if len(allAssets) != 2 {
		t.Fatalf("want 2 assets (original + duplicate identity), got %d", len(allAssets))
	}
}

// TestMatrix_RelinkOutranksReimport proves the precedence order: an incoming
// file that matches BOTH a missing asset (content+name) AND an asset at its path
// (reimport) is relinked, not reimported.
func TestMatrix_RelinkOutranksReimport(t *testing.T) {
	ingester, source, assets := newImporter(t) // helper from importer_test.go
	ctx := context.Background()
	content := []byte("the-original-bytes")

	if _, err := ingester.Run(ctx, source, fstest.MapFS{"b.mp4": {Data: content}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ingester.Run(ctx, source, fstest.MapFS{}); err != nil {
		t.Fatal(err)
	}
	if missing, _ := assets.FindBySourcePath(ctx, source.ID, "b.mp4"); missing == nil || missing.FileStatus != domain.FileStatusMissing {
		t.Fatalf("precondition: b.mp4 should be missing, got %v", missing)
	}

	result, err := ingester.Run(ctx, source, fstest.MapFS{"b.mp4": {Data: content}})
	if err != nil {
		t.Fatal(err)
	}
	// The path matches (a reimport candidate), but relink wins the verdict.
	if result.Moved != 1 || result.Updated != 0 {
		t.Fatalf("relink must outrank reimport: moved=%d updated=%d, want 1/0", result.Moved, result.Updated)
	}
}

// TestUnpairedRename_RecordedForReview proves the conservative policy for
// `mv a.mp4 b.mp4` fed as two independent hints (delete + create, no OS rename
// pairing): a name change is NOT auto-relinked. Instead the original is left
// missing and the new path is minted as a distinct asset, LINKED by a pending
// duplicates row — the "probable move" the review queue surfaces (a duplicate
// whose original is missing). No judgment is touched. Holds in BOTH orderings.
func TestUnpairedRename_RecordedForReview(t *testing.T) {
	content := []byte("bytes-that-get-renamed")

	// Ordering A — the delete graduates first (old already missing when new lands).
	t.Run("delete-then-create", func(t *testing.T) {
		ingester, source, assets := newImporter(t)
		ctx := context.Background()
		if err := ingester.IngestFile(ctx, source, fstest.MapFS{"old.mp4": {Data: content}}, "old.mp4"); err != nil {
			t.Fatal(err)
		}
		original, _ := assets.FindBySourcePath(ctx, source.ID, "old.mp4")
		if err := ingester.IngestFile(ctx, source, fstest.MapFS{}, "old.mp4"); err != nil {
			t.Fatal(err)
		}
		if err := ingester.IngestFile(ctx, source, fstest.MapFS{"new.mp4": {Data: content}}, "new.mp4"); err != nil {
			t.Fatal(err)
		}
		assertProbableMove(t, assets, source.ID, original.ID)
	})

	// Ordering B — the create graduates first (both paths exist, then old vanishes).
	t.Run("create-then-delete", func(t *testing.T) {
		ingester, source, assets := newImporter(t)
		ctx := context.Background()
		if err := ingester.IngestFile(ctx, source, fstest.MapFS{"old.mp4": {Data: content}}, "old.mp4"); err != nil {
			t.Fatal(err)
		}
		original, _ := assets.FindBySourcePath(ctx, source.ID, "old.mp4")
		both := fstest.MapFS{"old.mp4": {Data: content}, "new.mp4": {Data: content}}
		if err := ingester.IngestFile(ctx, source, both, "new.mp4"); err != nil {
			t.Fatal(err)
		}
		if err := ingester.IngestFile(ctx, source, fstest.MapFS{"new.mp4": {Data: content}}, "old.mp4"); err != nil {
			t.Fatal(err)
		}
		assertProbableMove(t, assets, source.ID, original.ID)
	})
}

// assertProbableMove checks B's end state: two distinct assets (original missing,
// new online) and a pending duplicates row linking them — the review-queue signal
// for "probable move", with nothing auto-merged.
func assertProbableMove(t *testing.T, assets *sqlite.AssetRepo, sourceID, originalID string) {
	t.Helper()
	ctx := context.Background()
	old, _ := assets.FindBySourcePath(ctx, sourceID, "old.mp4")
	if old == nil || old.ID != originalID || old.FileStatus != domain.FileStatusMissing {
		t.Fatalf("original must be left MISSING, not merged: got %v", old)
	}
	fresh, _ := assets.FindBySourcePath(ctx, sourceID, "new.mp4")
	if fresh == nil || fresh.ID == originalID || fresh.FileStatus != domain.FileStatusOnline {
		t.Fatalf("new path must be a distinct online asset: got %v", fresh)
	}
	dups, _ := (&sqlite.DuplicateRepo{DB: assets.DB}).ListPending(ctx)
	if len(dups) != 1 {
		t.Fatalf("a probable move must be recorded as one pending pair, got %d", len(dups))
	}
	if dups[0].OriginalAssetID != originalID || dups[0].DuplicateAssetID != fresh.ID {
		t.Fatalf("pending pair should link original→new: got %+v", dups[0])
	}
}

// TestDeleteSideMerge_HealsCopyThenDelete proves impl/05's delete-side merge: an
// external app that "moves" a file by copying it to a new path and deleting the
// old one leaves NO stranded missing row and PRESERVES the original's judgments —
// the freshly-minted copy is absorbed back into the original identity.
func TestDeleteSideMerge_HealsCopyThenDelete(t *testing.T) {
	ingester, source, assets := newImporter(t)
	ctx := context.Background()
	content := []byte("photo-content-that-gets-moved")

	// Original, then a user judgment on it (this is what must survive).
	if _, err := ingester.Run(ctx, source, fstest.MapFS{"a/photo.mp4": {Data: content}}); err != nil {
		t.Fatal(err)
	}
	original, _ := assets.FindBySourcePath(ctx, source.ID, "a/photo.mp4")
	if err := assets.ApplyTriagePatch(ctx, []string{original.ID}, catalog.TriagePatch{Rating: domain.SetOpt(5)}); err != nil {
		t.Fatal(err)
	}

	// Copy-then-delete "move": same filename, new dir; old path is gone.
	result, err := ingester.Run(ctx, source, fstest.MapFS{"b/photo.mp4": {Data: content}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Missing != 0 {
		t.Fatalf("delete-side merge must leave no missing row: missing=%d, want 0", result.Missing)
	}

	all, _ := assets.List(ctx, catalog.AssetFilter{})
	if len(all) != 1 {
		t.Fatalf("the copy must be absorbed, not left as a second identity: got %d assets, want 1", len(all))
	}
	survivor := all[0]
	if survivor.ID != original.ID {
		t.Fatalf("the ORIGINAL identity must survive: got %s, want %s", survivor.ID, original.ID)
	}
	if survivor.RelativePath != "b/photo.mp4" || survivor.FileStatus != domain.FileStatusOnline {
		t.Fatalf("survivor should be online at the new path: path=%s status=%s", survivor.RelativePath, survivor.FileStatus)
	}
	if survivor.Rating == nil || *survivor.Rating != 5 {
		t.Fatalf("the user's rating must be preserved through the merge: got %v", survivor.Rating)
	}
	if gone, _ := assets.FindBySourcePath(ctx, source.ID, "a/photo.mp4"); gone != nil {
		t.Fatalf("the old path should be gone, got %v", gone)
	}
	// The duplicates row logged for the copy must be cleaned (FK cascade).
	dups, _ := (&sqlite.DuplicateRepo{DB: assets.DB}).ListPending(ctx)
	if len(dups) != 0 {
		t.Fatalf("duplicates row should be cascade-deleted with the absorbed copy: got %d", len(dups))
	}
}

// TestDeleteSideMerge_GuardedByJudgment proves the safety guard: if the copy has
// already been judged, it is NOT absorbed — the original goes missing normally and
// both identities survive. (Here the guard is exercised by rating the whole
// content set so the young copy is non-zero-judgment.)
func TestDeleteSideMerge_GuardedByJudgment(t *testing.T) {
	ingester, source, assets := newImporter(t)
	ctx := context.Background()
	content := []byte("content-copied-and-then-judged")

	if _, err := ingester.Run(ctx, source, fstest.MapFS{"a/photo.mp4": {Data: content}}); err != nil {
		t.Fatal(err)
	}
	// Ingest the copy FIRST (present duplicate), judge it, THEN drop the original.
	if _, err := ingester.Run(ctx, source, fstest.MapFS{
		"a/photo.mp4": {Data: content},
		"b/photo.mp4": {Data: content},
	}); err != nil {
		t.Fatal(err)
	}
	copyAsset, _ := assets.FindBySourcePath(ctx, source.ID, "b/photo.mp4")
	if err := assets.ApplyTriagePatch(ctx, []string{copyAsset.ID}, catalog.TriagePatch{Rating: domain.SetOpt(3)}); err != nil {
		t.Fatal(err)
	}

	// Original vanishes. The copy is judged → must NOT be absorbed.
	result, err := ingester.Run(ctx, source, fstest.MapFS{"b/photo.mp4": {Data: content}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Missing != 1 {
		t.Fatalf("a judged copy must not heal the delete: missing=%d, want 1", result.Missing)
	}
	all, _ := assets.List(ctx, catalog.AssetFilter{})
	if len(all) != 2 {
		t.Fatalf("both identities must survive when the guard trips: got %d assets, want 2", len(all))
	}
}

// --- Sidecars: tracked, not indexed ---

func TestSidecar_TrackedNotIndexed(t *testing.T) {
	ingester, source, assets, _ := newPipelineImporter(t, "")
	ctx := context.Background()

	result, err := ingester.Run(ctx, source, fstest.MapFS{
		"trip/photo.mp4": {Data: []byte("video-bytes")},
		"trip/photo.xmp": {Data: []byte("<xmp>sidecar</xmp>")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 1 {
		t.Fatalf("added=%d, want 1 (the mp4; the xmp is a sidecar)", result.Added)
	}
	allAssets, _ := assets.List(ctx, catalog.AssetFilter{})
	if len(allAssets) != 1 {
		t.Fatalf("want 1 asset, got %d (sidecar leaked in as an asset?)", len(allAssets))
	}
	sidecars, err := (&sqlite.SidecarRepo{DB: assets.DB}).ListByKey(ctx, source.ID, "trip", "photo")
	if err != nil {
		t.Fatal(err)
	}
	if len(sidecars) != 1 || sidecars[0].Ext != "xmp" {
		t.Fatalf("want 1 xmp sidecar under (trip, photo), got %+v", sidecars)
	}
}

// --- Sniff mismatch (D7): a .png that is really a JPEG ---

func TestMismatch_TrustsContent(t *testing.T) {
	ingester, source, assets, imports := newPipelineImporter(t, "")
	ctx := context.Background()

	result, err := ingester.Run(ctx, source, fstest.MapFS{"mislabeled.png": {Data: jpegBytes(1)}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 1 {
		t.Fatalf("added=%d, want 1", result.Added)
	}
	asset, _ := assets.FindBySourcePath(ctx, source.ID, "mislabeled.png")
	if asset == nil {
		t.Fatal("asset missing")
	}
	if asset.MIMEType != "image/jpeg" || asset.FileType != domain.FileTypeImage {
		t.Fatalf("content not trusted: mime=%q type=%q, want image/jpeg", asset.MIMEType, asset.FileType)
	}
	if asset.ExtendedMetadata["alexandria:extension_mismatch"] == nil {
		t.Fatalf("missing extension_mismatch marker: %v", asset.ExtendedMetadata)
	}
	if dlqRows, _ := imports.ListErrors(ctx, result.SessionID); !hasReason(dlqRows, "ext_mismatch") {
		t.Fatalf("want an ext_mismatch DLQ row, got %v", dlqRows)
	}
}

// --- Corrupt file → DLQ, then heal (D13 self-heal) ---

func TestCorruptFile_DLQThenHeal(t *testing.T) {
	thumbnailDir := t.TempDir()
	ingester, source, assets, imports := newPipelineImporter(t, thumbnailDir)
	ctx := context.Background()

	// Truncated JPEG: valid magic (so it indexes) but undecodable.
	truncated := jpegBytes(2)[:20]
	result, err := ingester.Run(ctx, source, fstest.MapFS{"broken.jpg": {Data: truncated}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 1 {
		t.Fatalf("corrupt file should still mint an identity: added=%d", result.Added)
	}
	asset, _ := assets.FindBySourcePath(ctx, source.ID, "broken.jpg")
	if asset.Width != nil || asset.ThumbnailAt != nil {
		t.Fatalf("corrupt file should have no dims/thumbnail: w=%v thumb=%v", asset.Width, asset.ThumbnailAt)
	}
	if dlqRows, _ := imports.ListErrors(ctx, result.SessionID); !hasReason(dlqRows, "decode_failed") {
		t.Fatalf("want a decode_failed DLQ row, got %v", dlqRows)
	}

	// Fix the file: full valid bytes (larger → changed size → reimported).
	healedResult, err := ingester.Run(ctx, source, fstest.MapFS{"broken.jpg": {Data: jpegBytes(2)}})
	if err != nil {
		t.Fatal(err)
	}
	if healedResult.Updated != 1 {
		t.Fatalf("heal: updated=%d, want 1", healedResult.Updated)
	}
	asset, _ = assets.FindBySourcePath(ctx, source.ID, "broken.jpg")
	if asset.Width == nil || *asset.Width != 16 || asset.ThumbnailAt == nil {
		t.Fatalf("healed asset should carry dims + thumbnail: w=%v thumb=%v", asset.Width, asset.ThumbnailAt)
	}
	if healedRows, _ := imports.ListErrors(ctx, healedResult.SessionID); hasReason(healedRows, "decode_failed") {
		t.Fatalf("healed run should have no decode_failed rows, got %v", healedRows)
	}
}

// --- Batch visibility: OnProgress Done matches committed rows ---

func TestBatchVisibility(t *testing.T) {
	ingester, source, _, _ := newPipelineImporter(t, "")
	ctx := context.Background()

	const fileCount = 120 // > 2 batches at 50/txn
	files := fstest.MapFS{}
	for i := 0; i < fileCount; i++ {
		files[filepath.Join("clip", fmtInt(i)+".mp4")] = &fstest.MapFile{
			Data: []byte("unique-clip-" + fmtInt(i) + "-payload"),
		}
	}

	var progressCalls, lastDone int
	ingester.OnProgress = func(progress importer.Progress) {
		progressCalls++
		if progress.Done < lastDone {
			t.Errorf("Done went backwards: %d then %d", lastDone, progress.Done)
		}
		lastDone = progress.Done
	}

	result, err := ingester.Run(ctx, source, files)
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != fileCount {
		t.Fatalf("added=%d, want %d", result.Added, fileCount)
	}
	if progressCalls < 2 {
		t.Fatalf("OnProgress fired %d times, want ≥2 for %d files at 50/batch", progressCalls, fileCount)
	}
	if lastDone != fileCount {
		t.Fatalf("final Done=%d, want %d (committed rows)", lastDone, fileCount)
	}
}

// --- Full-processing invariant (the sacred LrC-trauma test) ---

func TestFullProcessingInvariant_Cancel(t *testing.T) {
	thumbnailDir := t.TempDir()
	ingester, source, assets, imports := newPipelineImporter(t, thumbnailDir)

	const fileCount = 300
	files := fstest.MapFS{}
	for i := 0; i < fileCount; i++ {
		files[filepath.Join("shoot", fmtInt(i)+".jpg")] = &fstest.MapFile{Data: jpegBytes(i)}
	}

	ctx, cancel := context.WithCancel(context.Background())
	ingester.OnProgress = func(progress importer.Progress) {
		if progress.Done > 0 {
			cancel() // cancel once at least one batch has committed
		}
	}
	result, _ := ingester.Run(ctx, source, files) // cancellation returns context.Canceled

	committedAssets, _ := assets.List(context.Background(), catalog.AssetFilter{})
	if len(committedAssets) == 0 {
		t.Skip("cancel raced ahead of the first commit; nothing to check")
	}
	dlqRows, _ := imports.ListErrors(context.Background(), result.SessionID)
	thumbnails := thumbnailer.New(thumbnailDir)
	for _, asset := range committedAssets {
		// Every committed asset is FULLY processed: a thumbnail on disk, or a DLQ
		// row explaining why. No half-imported placeholder, ever.
		if asset.ThumbnailAt != nil {
			if _, err := os.Stat(thumbnails.Path(asset.ID, 512)); err != nil {
				t.Fatalf("asset %s flagged thumbnailed but no file at %s", asset.ID, thumbnails.Path(asset.ID, 512))
			}
			continue
		}
		if !pathHasError(dlqRows, asset.RelativePath) {
			t.Fatalf("committed asset %s is half-processed: no thumbnail, no error row", asset.RelativePath)
		}
	}
}

// --- Throughput sanity (NFR-2): a runnable benchmark, no flaky timing assert ---

func BenchmarkPipeline_JPEGThroughput(b *testing.B) {
	thumbnailDir := b.TempDir()
	files := fstest.MapFS{}
	for i := 0; i < 500; i++ {
		files[fmtInt(i)+".jpg"] = &fstest.MapFile{Data: jpegBytes(i)}
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		database := benchDB(b)
		assets := &sqlite.AssetRepo{DB: database}
		ingester := &importer.Importer{
			Reader: assets, Obs: assets, Derived: assets,
			Dups:      &sqlite.DuplicateRepo{DB: database},
			Store:     sqlite.NewStore(database),
			Imports:   &sqlite.ImportRepo{DB: database},
			Thumbnail: thumbnailer.New(thumbnailDir),
			Log:       log.New(io.Discard),
		}
		source := benchSource(b, database)
		if _, err := ingester.Run(context.Background(), source, files); err != nil {
			b.Fatal(err)
		}
	}
}

// --- helpers ---

func benchDB(b *testing.B) *sql.DB {
	b.Helper()
	database, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		b.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	if err := migrations.Migrate(database); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { database.Close() })
	return database
}

func benchSource(b *testing.B, database *sql.DB) *domain.Source {
	b.Helper()
	now := time.Now().UTC()
	source := &domain.Source{
		ID: domain.NewID(), Name: "bench", Kind: domain.SourceKindLocal,
		BasePath: "/bench", ScanRecursively: true, Enabled: true,
		Connectivity: domain.SourceOnline, CreatedAt: now, UpdatedAt: now,
	}
	if err := (&sqlite.SourceRepo{DB: database}).Create(context.Background(), source); err != nil {
		b.Fatal(err)
	}
	return source
}

func hasReason(dlqRows []*domain.ImportError, reason string) bool {
	for _, row := range dlqRows {
		if row.ReasonCode == reason {
			return true
		}
	}
	return false
}

func pathHasError(dlqRows []*domain.ImportError, path string) bool {
	for _, row := range dlqRows {
		if row.Path == path {
			return true
		}
	}
	return false
}

func fmtInt(value int) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
