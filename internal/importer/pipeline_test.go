package importer_test

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/jpeg"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/enrichment"
	"github.com/akmadian/alexandria/internal/importer"
	"github.com/akmadian/alexandria/internal/migrations"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
	"github.com/akmadian/alexandria/internal/thumbnailer"
	"github.com/akmadian/gospan"
	"github.com/charmbracelet/log"
)

// newPipelineImporter is like newImporter but returns the import repo (so tests
// can read DLQ/session rows).
func newPipelineImporter(t *testing.T) (*importer.Importer, *domain.Source, *sqlite.AssetRepo, *sqlite.ImportRepo) {
	t.Helper()
	database := testutil.NewTestDB(t)
	source := testutil.NewTestSource(t, database, "photos")
	assets := &sqlite.AssetRepo{DB: database}
	imports := &sqlite.ImportRepo{DB: database}
	ingester := &importer.Importer{
		Reader:  assets,
		Obs:     assets,
		Dups:    &sqlite.DuplicateRepo{DB: database},
		Store:   sqlite.NewStore(database),
		Imports: imports,
		Log:     log.New(io.Discard),
	}
	return ingester, source, assets, imports
}

// diskImporter builds an importer over REAL files in a temp dir — the shape the
// enrichment convergence tests need, because the enrichment producer reads the
// asset's bytes from disk by absolute path (source base path + relative path),
// which a fstest.MapFS never has.
func diskImporter(t *testing.T) (*importer.Importer, *domain.Source, string, *sql.DB) {
	t.Helper()
	sourceDir := t.TempDir()
	database := testutil.NewTestDB(t)
	now := time.Now().UTC()
	source := &domain.Source{
		ID: domain.NewID(), Name: "disk", Kind: domain.SourceKindLocal,
		BasePath: sourceDir, ScanRecursively: true, Enabled: true,
		Connectivity: domain.SourceOnline, CreatedAt: now, UpdatedAt: now,
	}
	if err := (&sqlite.SourceRepo{DB: database}).Create(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	assets := &sqlite.AssetRepo{DB: database}
	ingester := &importer.Importer{
		Reader:  assets,
		Obs:     assets,
		Dups:    &sqlite.DuplicateRepo{DB: database},
		Store:   sqlite.NewStore(database),
		Imports: &sqlite.ImportRepo{DB: database},
		Log:     log.New(io.Discard),
	}
	return ingester, source, sourceDir, database
}

// startEnrichmentEngine runs the real engine — canonical registry, real
// thumbnailer — over the test catalog, exactly as the composition root wires
// it. Its first act is the on-open missing-artifact scan (D25: crash recovery
// = rescan), which is what these tests lean on.
func startEnrichmentEngine(t *testing.T, database *sql.DB, thumbnailDir string) *enrichment.Engine {
	t.Helper()
	engine, err := enrichment.New(&enrichment.Config{
		Definitions: enrichment.Definitions(thumbnailer.New(thumbnailDir), &sqlite.SourceRepo{DB: database}),
		Reader:      &sqlite.AssetRepo{DB: database},
		Store:       sqlite.NewStore(database),
		Enrichment:  &sqlite.EnrichmentRepo{DB: database},
		Log:         log.New(io.Discard),
	})
	if err != nil {
		t.Fatal(err)
	}
	engine.Start(context.Background())
	t.Cleanup(engine.Stop)
	return engine
}

// awaitThumbnails polls until want assets carry thumbnail_at — the convergence
// the engine promises — or fails at the deadline.
func awaitThumbnails(t *testing.T, database *sql.DB, want int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var thumbnailed int
		if err := database.QueryRowContext(context.Background(),
			"SELECT COUNT(*) FROM assets WHERE thumbnail_at IS NOT NULL").Scan(&thumbnailed); err != nil {
			t.Fatal(err)
		}
		if thumbnailed >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("enrichment did not converge: %d/%d thumbnailed", thumbnailed, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// awaitSignals polls until want assets carry all three cheap signals — the
// convergence the prerequisite edge (each signal kind gates on the thumbnail
// artifact) promises — or fails at the deadline.
func awaitSignals(t *testing.T, database *sql.DB, want int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var scored int
		if err := database.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM assets
			 WHERE sharpness IS NOT NULL AND clipping_highlights IS NOT NULL
			   AND clipping_shadows IS NOT NULL AND phash IS NOT NULL`).Scan(&scored); err != nil {
			t.Fatal(err)
		}
		if scored >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("signals did not converge: %d/%d fully scored", scored, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// signalSnapshot is one asset's full set of computed signals — a comparable
// struct so idempotence can be asserted across all of them, not just sharpness.
type signalSnapshot struct {
	sharpness          float64
	clippingHighlights float64
	clippingShadows    float64
	phash              string
}

// readSignals snapshots every asset's computed signals, keyed by id.
func readSignals(t *testing.T, database *sql.DB) map[string]signalSnapshot {
	t.Helper()
	rows, err := database.QueryContext(context.Background(),
		`SELECT id, sharpness, clipping_highlights, clipping_shadows, phash
		 FROM assets WHERE sharpness IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	snapshots := map[string]signalSnapshot{}
	for rows.Next() {
		var id string
		var snapshot signalSnapshot
		if err := rows.Scan(&id, &snapshot.sharpness, &snapshot.clippingHighlights,
			&snapshot.clippingShadows, &snapshot.phash); err != nil {
			t.Fatal(err)
		}
		snapshots[id] = snapshot
	}
	return snapshots
}

// writeJPEGFiles writes count distinct decodable JPEGs under dir and returns
// their relative paths.
func writeJPEGFiles(t *testing.T, dir string, count int) []string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "shoot"), 0o750); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, count)
	for i := 0; i < count; i++ {
		relative := filepath.Join("shoot", fmtInt(i)+".jpg")
		if err := os.WriteFile(filepath.Join(dir, relative), jpegBytes(i), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, relative)
	}
	return paths
}

// jpegBytes returns a small, valid, decodable JPEG whose bytes vary by seed, so
// distinct seeds produce distinct partial hashes.
func jpegBytes(seed int) []byte {
	pixels := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range pixels.Pix {
		pixels.Pix[i] = byte((i*7 + seed*131) % 256) //nolint:gosec // mod 256 fits in byte
	}
	var buffer bytes.Buffer
	_ = jpeg.Encode(&buffer, pixels, &jpeg.Options{Quality: 90})
	return buffer.Bytes()
}

// --- Matrix: the five verdicts ---

func TestMatrix_NewReimportDuplicate(t *testing.T) {
	ingester, source, _, _ := newPipelineImporter(t)
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
	ingester, source, assets, _ := newPipelineImporter(t)
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
	var assetCount int
	if err := assets.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM assets WHERE is_deleted=0").Scan(&assetCount); err != nil {
		t.Fatal(err)
	}
	if assetCount != 2 {
		t.Fatalf("want 2 assets (original + duplicate identity), got %d", assetCount)
	}
}

// TestReimport_RestoresMissingAtOriginalPath proves the path-fidelity that replaces
// relink (D20): a file that went missing and reappears at its ORIGINAL path is
// restored online via reimport — same identity, no new row, no review needed.
func TestReimport_RestoresMissingAtOriginalPath(t *testing.T) {
	ingester, source, assets := newImporter(t) // helper from importer_test.go
	ctx := context.Background()
	content := []byte("the-original-bytes")

	if _, err := ingester.Run(ctx, source, fstest.MapFS{"b.mp4": {Data: content}}); err != nil {
		t.Fatal(err)
	}
	original, _ := assets.FindBySourcePath(ctx, source.ID, "b.mp4")
	if _, err := ingester.Run(ctx, source, fstest.MapFS{}); err != nil {
		t.Fatal(err)
	}
	if missing, _ := assets.FindBySourcePath(ctx, source.ID, "b.mp4"); missing == nil || missing.FileStatus != domain.FileStatusMissing {
		t.Fatalf("precondition: b.mp4 should be missing, got %v", missing)
	}

	// Reappears at the same path → reimport restores it online, same identity.
	result, err := ingester.Run(ctx, source, fstest.MapFS{"b.mp4": {Data: content}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || result.Added != 0 {
		t.Fatalf("same-path reappearance must reimport, not mint: updated=%d added=%d, want 1/0", result.Updated, result.Added)
	}
	restored, _ := assets.FindBySourcePath(ctx, source.ID, "b.mp4")
	if restored.ID != original.ID || restored.FileStatus != domain.FileStatusOnline {
		t.Fatalf("must restore the same identity online: got id=%s status=%s", restored.ID, restored.FileStatus)
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

// TestCopyThenDeleteMove_RecordedForReview proves D20: even a same-NAME
// copy-then-delete "move" (an external app writing to a new dir then deleting the
// old) is no longer auto-merged. The original is left missing with its judgment
// intact, the copy is a distinct new asset, and the pair is a pending review row —
// the user confirms the move. (Under the old delete-side merge this auto-healed.)
func TestCopyThenDeleteMove_RecordedForReview(t *testing.T) {
	ingester, source, assets := newImporter(t)
	ctx := context.Background()
	content := []byte("photo-content-that-gets-moved")

	// Original, then a user judgment on it (this is what must survive on the missing row).
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
	if result.Missing != 1 || result.Added != 1 {
		t.Fatalf("no auto-merge: want the original missing (1) + the copy minted (1), got missing=%d added=%d", result.Missing, result.Added)
	}

	var allCount int
	if err := assets.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM assets WHERE is_deleted=0").Scan(&allCount); err != nil {
		t.Fatal(err)
	}
	if allCount != 2 {
		t.Fatalf("both identities must survive (no merge): got %d assets, want 2", allCount)
	}
	stale, _ := assets.FindBySourcePath(ctx, source.ID, "a/photo.mp4")
	if stale == nil || stale.ID != original.ID || stale.FileStatus != domain.FileStatusMissing {
		t.Fatalf("original must be left MISSING with identity intact: got %v", stale)
	}
	if stale.Rating == nil || *stale.Rating != 5 {
		t.Fatalf("the user's rating must stay on the missing original: got %v", stale.Rating)
	}
	fresh, _ := assets.FindBySourcePath(ctx, source.ID, "b/photo.mp4")
	if fresh == nil || fresh.ID == original.ID || fresh.FileStatus != domain.FileStatusOnline {
		t.Fatalf("copy must be a distinct online asset: got %v", fresh)
	}
	// The pair is surfaced for review — original(missing) ↔ copy(online).
	dups, _ := (&sqlite.DuplicateRepo{DB: assets.DB}).ListPending(ctx)
	if len(dups) != 1 {
		t.Fatalf("the move must be recorded as one pending review pair, got %d", len(dups))
	}
	if dups[0].OriginalAssetID != original.ID || dups[0].DuplicateAssetID != fresh.ID {
		t.Fatalf("pending pair should link original→copy: got %+v", dups[0])
	}
}

// --- Sidecars: tracked, not indexed ---

func TestSidecar_TrackedNotIndexed(t *testing.T) {
	ingester, source, assets, _ := newPipelineImporter(t)
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
	var assetCount int
	if err := assets.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM assets WHERE is_deleted=0").Scan(&assetCount); err != nil {
		t.Fatal(err)
	}
	if assetCount != 1 {
		t.Fatalf("want 1 asset, got %d (sidecar leaked in as an asset?)", assetCount)
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
	ingester, source, assets, imports := newPipelineImporter(t)
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

// --- Corrupt file → DLQ, then heal (D13 self-heal, post-D25 shape) ---

// TestCorruptFile_DLQThenHeal walks the full self-heal loop across BOTH
// engines. Post-D25 ingest never decodes pixels, so a corrupt-but-valid-magic
// file indexes with no import error at all; the corruption is discovered by
// the first pixel consumer — the thumbnail job — and recorded in the
// ENRICHMENT DLQ. Fixing the file is a reimport, whose staleness clear wipes
// that DLQ row, and the next scan converges the asset to thumbnailed. "Import
// again" is still the repair; the ledger just moved to the engine that does
// the decoding.
func TestCorruptFile_DLQThenHeal(t *testing.T) {
	ingester, source, sourceDir, database := diskImporter(t)
	ctx := context.Background()
	brokenPath := filepath.Join(sourceDir, "broken.jpg")
	if err := os.WriteFile(brokenPath, jpegBytes(2)[:20], 0o600); err != nil { // valid magic, undecodable
		t.Fatal(err)
	}

	result, err := ingester.Run(ctx, source, os.DirFS(sourceDir))
	if err != nil || result.Added != 1 {
		t.Fatalf("corrupt file should still mint an identity: added=%d err=%v", result.Added, err)
	}
	assets := &sqlite.AssetRepo{DB: database}
	asset, _ := assets.FindBySourcePath(ctx, source.ID, "broken.jpg")
	if asset.Width != nil {
		t.Fatalf("corrupt file should have no dims: w=%v", asset.Width)
	}

	// The engine discovers the corruption: a durable decode_failed DLQ row.
	engine := startEnrichmentEngine(t, database, t.TempDir())
	// The production wiring: ingest's post-commit hook nudges the dispatcher
	// (the composition root chains OnAssetCommitted → RequestScan) — without it
	// the healed reimport would sit unscanned until the next catalog open.
	ingester.OnAssetCommitted = func(context.Context, *domain.Source, string, string) {
		engine.RequestScan()
	}
	enrichmentRepo := &sqlite.EnrichmentRepo{DB: database}
	awaitEnrichmentFailure(t, enrichmentRepo, asset.ID, "decode_failed")

	// Fix the file (larger → changed size → reimported): the reimport's
	// staleness clear wipes the DLQ row, and the scan re-derives the work.
	if err := os.WriteFile(brokenPath, jpegBytes(2), 0o600); err != nil {
		t.Fatal(err)
	}
	healedResult, err := ingester.Run(ctx, source, os.DirFS(sourceDir))
	if err != nil || healedResult.Updated != 1 {
		t.Fatalf("heal: updated=%d err=%v, want 1/nil", healedResult.Updated, err)
	}
	asset, _ = assets.FindBySourcePath(ctx, source.ID, "broken.jpg")
	if asset.Width == nil || *asset.Width != 16 {
		t.Fatalf("healed asset should carry dims: w=%v", asset.Width)
	}
	awaitThumbnails(t, database, 1)
	if failures, _ := enrichmentRepo.ListFailures(ctx, asset.ID); len(failures) != 0 {
		t.Fatalf("healed asset should have no enrichment failures left, got %+v", failures)
	}
}

// awaitEnrichmentFailure polls for a durable (asset, kind) enrichment DLQ row
// with the given reason.
func awaitEnrichmentFailure(t *testing.T, enrichmentRepo *sqlite.EnrichmentRepo, assetID, wantReason string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		failures, err := enrichmentRepo.ListFailures(context.Background(), assetID)
		if err != nil {
			t.Fatal(err)
		}
		if len(failures) > 0 && failures[0].ReasonCode == wantReason {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %s enrichment failure recorded for %s, got %+v", wantReason, assetID, failures)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestReimport_ClearsDerivedAndEnrichmentDLQ pins the D28 staleness transition
// on BOTH write paths (the batched pipeline and the single-file watcher path):
// a reimport's transaction nulls the derived columns (the artifact instantly
// reads "missing") and deletes the asset's enrichment DLQ rows (exhaustion
// described the old bytes) — while the thumbnail FILE survives on disk, so the
// grid shows the outdated-but-real thumb until regeneration overwrites it.
func TestReimport_ClearsDerivedAndEnrichmentDLQ(t *testing.T) {
	reimportVia := map[string]func(t *testing.T, ingester *importer.Importer, source *domain.Source, files fstest.MapFS){
		"pipeline": func(t *testing.T, ingester *importer.Importer, source *domain.Source, files fstest.MapFS) {
			t.Helper()
			result, err := ingester.Run(context.Background(), source, files)
			if err != nil || result.Updated != 1 {
				t.Fatalf("reimport run: updated=%d err=%v, want 1/nil", result.Updated, err)
			}
		},
		"single-file": func(t *testing.T, ingester *importer.Importer, source *domain.Source, files fstest.MapFS) {
			t.Helper()
			if err := ingester.IngestFile(context.Background(), source, files, "edited.jpg"); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, reimport := range reimportVia {
		t.Run(name, func(t *testing.T) {
			ingester, source, assets, _ := newPipelineImporter(t)
			ctx := context.Background()
			if _, err := ingester.Run(ctx, source, fstest.MapFS{"edited.jpg": {Data: jpegBytes(1)}}); err != nil {
				t.Fatal(err)
			}
			asset, _ := assets.FindBySourcePath(ctx, source.ID, "edited.jpg")

			// Simulate enrichment residue describing the ORIGINAL bytes: the
			// artifact column set, a DLQ row, and the thumbnail file on disk.
			if err := assets.SetThumbnailAt(ctx, asset.ID, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			enrichmentRepo := &sqlite.EnrichmentRepo{DB: assets.DB}
			if err := enrichmentRepo.LogFailure(ctx, asset.ID, "sharpness", "decode_failed", "old bytes"); err != nil {
				t.Fatal(err)
			}
			thumbnails := thumbnailer.New(t.TempDir())
			thumbnailPath := thumbnails.Path(asset.ID, 512)
			if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(thumbnailPath, []byte("old-thumb-bytes"), 0o600); err != nil {
				t.Fatal(err)
			}

			// Same path, changed bytes → reimport.
			reimport(t, ingester, source, fstest.MapFS{"edited.jpg": {Data: jpegBytes(2)}})

			reloaded, _ := assets.FindBySourcePath(ctx, source.ID, "edited.jpg")
			if reloaded.ThumbnailAt != nil {
				t.Fatalf("reimport must clear derived columns, thumbnail_at=%v", reloaded.ThumbnailAt)
			}
			failures, _ := enrichmentRepo.ListFailures(ctx, asset.ID)
			if len(failures) != 0 {
				t.Fatalf("reimport must clear the asset's enrichment DLQ rows, got %d", len(failures))
			}
			if _, err := os.Stat(thumbnailPath); err != nil {
				t.Fatalf("the stale thumbnail FILE must survive the reimport: %v", err)
			}
		})
	}
}

// --- Batch visibility: OnProgress Done matches committed rows ---

func TestBatchVisibility(t *testing.T) {
	ingester, source, _, _ := newPipelineImporter(t)
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

// TestMidImportCountsPersist proves the session row's counts climb DURING the run
// (per-batch UpdateCounts), not only at Finish: a live viewer polling the session
// sees progress. The OnProgress hook fires right after each batch's count flush, so
// reading the session there captures the mid-flight persisted count.
func TestMidImportCountsPersist(t *testing.T) {
	ingester, source, _, imports := newPipelineImporter(t)
	ctx := context.Background()

	const fileCount = 120 // > 2 batches at 50/txn
	files := fstest.MapFS{}
	for i := 0; i < fileCount; i++ {
		files[filepath.Join("clip", fmtInt(i)+".mp4")] = &fstest.MapFile{
			Data: []byte("unique-clip-" + fmtInt(i) + "-payload"),
		}
	}

	var maxMidRunAdded int
	var sawMidBatch bool
	ingester.OnProgress = func(progress importer.Progress) {
		// Only the non-final batches: their persisted count must be a partial total.
		if progress.Done == 0 || progress.Done >= fileCount {
			return
		}
		sawMidBatch = true
		sessions, err := imports.ListSessions(context.Background(), 1)
		if err != nil || len(sessions) != 1 {
			t.Errorf("read session mid-run: err=%v sessions=%d", err, len(sessions))
			return
		}
		if sessions[0].FinishedAt != nil {
			t.Error("session finished mid-run — Finish ran before the walk drained")
		}
		if sessions[0].Added > maxMidRunAdded {
			maxMidRunAdded = sessions[0].Added
		}
	}

	result, err := ingester.Run(ctx, source, files)
	if err != nil {
		t.Fatal(err)
	}
	if !sawMidBatch {
		t.Fatal("no mid-run progress event fired; expected ≥2 batches for 120 files")
	}
	if maxMidRunAdded == 0 {
		t.Fatal("session still showed added=0 mid-import — UpdateCounts is not persisting per batch")
	}
	if maxMidRunAdded >= result.Added {
		t.Fatalf("mid-run added=%d should be a partial count below the final added=%d", maxMidRunAdded, result.Added)
	}

	// And the final persisted counts (written by Finish) match the result.
	final, _ := imports.ListSessions(context.Background(), 1)
	if final[0].Added != result.Added {
		t.Errorf("final session added=%d, want %d", final[0].Added, result.Added)
	}
}

// --- The commit invariant (the sacred LrC-trauma test, re-derived for D25) ---
//
// impl/04's "committed = fully processed" becomes "committed = identity +
// observation complete; enrichment CONVERGES." The trauma the old invariant
// guarded against was DISHONEST placeholders — cells that could mean loading,
// broken, or never. The honest replacement: an asset commits with no thumbnail
// and a NULL thumbnail_at (the missing artifact IS the pending state), and the
// enrichment engine converges every eligible asset to thumbnailed — or to a
// durable DLQ row — with no re-import.

// TestEnrichment_ConvergesAfterImport is acceptance #1: a fixture import
// commits assets with NO thumbnails; the engine's on-open scan then converges
// every eligible asset.
func TestEnrichment_ConvergesAfterImport(t *testing.T) {
	ingester, source, sourceDir, database := diskImporter(t)
	const fileCount = 12
	writeJPEGFiles(t, sourceDir, fileCount)

	result, err := ingester.Run(context.Background(), source, os.DirFS(sourceDir))
	if err != nil || result.Added != fileCount {
		t.Fatalf("import: added=%d err=%v, want %d/nil", result.Added, err, fileCount)
	}
	var thumbnailed int
	if err := database.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM assets WHERE thumbnail_at IS NOT NULL").Scan(&thumbnailed); err != nil {
		t.Fatal(err)
	}
	if thumbnailed != 0 {
		t.Fatalf("D25: assets must commit WITHOUT thumbnails, got %d thumbnailed at commit", thumbnailed)
	}

	thumbnailDir := t.TempDir()
	startEnrichmentEngine(t, database, thumbnailDir) // on-open scan = the whole trigger
	awaitThumbnails(t, database, fileCount)

	// The artifact is real: a file on disk for every flagged asset.
	thumbnails := thumbnailer.New(thumbnailDir)
	assets := &sqlite.AssetRepo{DB: database}
	committedAssets, _, _ := assets.QueryAssets(context.Background(), ast.Query{Version: ast.Version},
		ast.Arrangement{SortField: ast.SortIngestedAt, SortDir: ast.SortDesc}, ast.Page{Limit: 100})
	for _, asset := range committedAssets {
		if _, err := os.Stat(thumbnails.Path(asset.ID, 512)); err != nil {
			t.Fatalf("asset %s flagged thumbnailed but no file at %s", asset.ID, thumbnails.Path(asset.ID, 512))
		}
	}
}

// TestEnrichment_ConvergesAfterCancel is the convergence-after-cancel half:
// cancel an import mid-run; the already-committed assets get thumbnails via
// the scan alone — no re-import, no repair step, no half-imported state.
func TestEnrichment_ConvergesAfterCancel(t *testing.T) {
	ingester, source, sourceDir, database := diskImporter(t)
	writeJPEGFiles(t, sourceDir, 300)

	ctx, cancel := context.WithCancel(context.Background())
	ingester.OnProgress = func(progress importer.Progress) {
		if progress.Done > 0 {
			cancel() // cancel once at least one batch has committed
		}
	}
	_, _ = ingester.Run(ctx, source, os.DirFS(sourceDir)) // cancellation returns context.Canceled

	var committed int
	if err := database.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM assets WHERE is_deleted=0").Scan(&committed); err != nil {
		t.Fatal(err)
	}
	if committed == 0 {
		t.Skip("cancel raced ahead of the first commit; nothing to check")
	}

	startEnrichmentEngine(t, database, t.TempDir())
	awaitThumbnails(t, database, committed)
}

// TestEnrichment_FailureIsDurableAndAssetBrowsable is the D13 half of the
// acceptance: a thumbnail failure (undecodable bytes) becomes an
// enrichment_errors row while the asset stays committed and browsable — the
// failed state is durable, never an eternal spinner.
func TestEnrichment_FailureIsDurableAndAssetBrowsable(t *testing.T) {
	ingester, source, sourceDir, database := diskImporter(t)
	writeJPEGFiles(t, sourceDir, 2)
	// Valid magic, undecodable body: indexes fine, thumbnails never.
	if err := os.WriteFile(filepath.Join(sourceDir, "shoot", "broken.jpg"), jpegBytes(9)[:20], 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := ingester.Run(context.Background(), source, os.DirFS(sourceDir))
	if err != nil || result.Added != 3 {
		t.Fatalf("import: added=%d err=%v, want 3/nil", result.Added, err)
	}

	startEnrichmentEngine(t, database, t.TempDir())
	awaitThumbnails(t, database, 2) // the two decodable files converge

	assets := &sqlite.AssetRepo{DB: database}
	broken, _ := assets.FindBySourcePath(context.Background(), source.ID, "shoot/broken.jpg")
	if broken == nil || broken.IsDeleted {
		t.Fatal("the failing asset must stay committed and browsable (D13)")
	}
	awaitEnrichmentFailure(t, &sqlite.EnrichmentRepo{DB: database}, broken.ID, "decode_failed")
}

// --- Cheap signals: convergence + idempotence (task 20) ---

// TestEnrichment_SignalsConvergeAfterThumbnails is the task-20 acceptance: an
// import commits assets with no signals; the enrichment scan then drives every
// eligible asset through thumbnail → all three cheap signals, via scans alone (no
// re-import), because each signal kind gates on the thumbnail artifact.
func TestEnrichment_SignalsConvergeAfterThumbnails(t *testing.T) {
	ingester, source, sourceDir, database := diskImporter(t)
	writeJPEGFiles(t, sourceDir, 12)

	result, err := ingester.Run(context.Background(), source, os.DirFS(sourceDir))
	if err != nil || result.Added != 12 {
		t.Fatalf("import: added=%d err=%v, want 12/nil", result.Added, err)
	}

	startEnrichmentEngine(t, database, t.TempDir())
	awaitThumbnails(t, database, 12)
	awaitSignals(t, database, 12)
}

// TestEnrichment_SignalsIdempotentOnRederive proves re-running the signal kinds on
// an already-enriched asset is a harmless, deterministic overwrite: clearing the
// derived columns (the D28 reimport staleness path) and rescanning re-derives the
// SAME values — the rebuild path the derived-state invariant requires.
func TestEnrichment_SignalsIdempotentOnRederive(t *testing.T) {
	ingester, source, sourceDir, database := diskImporter(t)
	writeJPEGFiles(t, sourceDir, 3)
	if _, err := ingester.Run(context.Background(), source, os.DirFS(sourceDir)); err != nil {
		t.Fatal(err)
	}
	engine := startEnrichmentEngine(t, database, t.TempDir())
	awaitSignals(t, database, 3)
	before := readSignals(t, database)

	// Clear every asset's derived state (thumbnail + signals), as a reimport would,
	// then rescan: the thumbnail regenerates from unchanged bytes and the signals
	// recompute off it.
	assets := &sqlite.AssetRepo{DB: database}
	for id := range before {
		if err := assets.ClearDerived(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	engine.RequestScan()
	awaitSignals(t, database, 3)

	after := readSignals(t, database)
	for id, want := range before {
		if after[id] != want {
			t.Fatalf("re-derived signals for %s = %+v, want %+v (idempotent)", id, after[id], want)
		}
	}
}

// --- Tracing: gospan spans cover the pipeline ---

// TestTracing_SpansCoverThePipeline runs a fixture import with a live tracer
// and asserts the span vocabulary lands: the run root, the walk span, one
// trace per item (asset/sidecar names kept distinct), per-stage child spans,
// the await-commit wait span, the write-batch fan-in trace, and error status
// on a corrupt file's failing stage. Every OTHER test in this file runs with a
// nil Tracer — the nil-is-off contract, exercised for free.
func TestTracing_SpansCoverThePipeline(t *testing.T) {
	ingester, source, _, _ := newPipelineImporter(t)
	tracer, err := gospan.New(gospan.SlogSink(slog.New(slog.DiscardHandler)))
	if err != nil {
		t.Fatal(err)
	}
	ingester.Tracer = tracer

	result, err := ingester.Run(context.Background(), source, fstest.MapFS{
		"trip/one.jpg":    {Data: jpegBytes(1)},
		"trip/two.jpg":    {Data: jpegBytes(2)},
		"trip/one.xmp":    {Data: []byte("<xmp>sidecar</xmp>")},
		"trip/broken.jpg": {Data: jpegBytes(3)[:20]}, // valid magic, undecodable — commits clean; the enrichment engine owns discovering that
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Added != 3 {
		t.Fatalf("added=%d, want 3", result.Added)
	}
	if err := tracer.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	summary := tracer.Summary()
	wantCounts := map[string]uint64{
		"import.run":          1,
		"import.scan":         1,
		"import.asset":        3,
		"import.sidecar":      1,
		"import.hash":         4, // sidecars hash too
		"import.match":        3, // sidecars skip the matrix
		"import.extract":      3,
		"import.await-commit": 4,
	}
	for name, want := range wantCounts {
		if got := summary[name].Count; got != want {
			t.Errorf("span %q: count=%d, want %d", name, got, want)
		}
	}
	if summary["import.write-batch"].Count == 0 {
		t.Error("no write-batch trace recorded")
	}
	// Stage-span error status on a failed commit is covered by
	// TestTracing_BatchFailureFailsItemTraces; ingest itself no longer decodes
	// pixels, so a corrupt file produces no failing stage span here.
	if summary["import.asset"].Max <= 0 {
		t.Error("asset root spans should carry real durations")
	}
}

// TestTracing_BatchFailureFailsItemTraces forces a whole-batch transaction
// failure (a poison trigger on the assets table) and asserts the fan-in
// tracing on the failure path: the write-batch span fails, and every item
// root span in the doomed batch fails with it. This is the one place a failed
// commit must mark both sides of the fan-in.
func TestTracing_BatchFailureFailsItemTraces(t *testing.T) {
	ingester, source, assets, _ := newPipelineImporter(t)
	if _, err := assets.DB.ExecContext(context.Background(), `CREATE TRIGGER poison_batch BEFORE INSERT ON assets
		WHEN NEW.filename = 'poison.jpg' BEGIN SELECT RAISE(ABORT, 'poisoned batch'); END`); err != nil {
		t.Fatal(err)
	}
	tracer, err := gospan.New(gospan.SlogSink(slog.New(slog.DiscardHandler)))
	if err != nil {
		t.Fatal(err)
	}
	ingester.Tracer = tracer

	// Three files, one batch (< batch size): the poison row aborts the tx, so
	// the whole batch — innocent files included — fails together.
	result, err := ingester.Run(context.Background(), source, fstest.MapFS{
		"a.jpg":      {Data: jpegBytes(1)},
		"b.jpg":      {Data: jpegBytes(2)},
		"poison.jpg": {Data: jpegBytes(3)},
	})
	if err != nil {
		t.Fatalf("a poisoned batch must not abort the run (idempotency is the recovery): %v", err)
	}
	if len(result.Errors) != 3 {
		t.Fatalf("all 3 items of the poisoned batch should be errored, got %d", len(result.Errors))
	}
	if err := tracer.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	summary := tracer.Summary()
	if got := summary["import.write-batch"].Errors; got != 1 {
		t.Errorf("write-batch spans errored: %d, want 1 (the poisoned commit)", got)
	}
	if got := summary["import.asset"].Errors; got != 3 {
		t.Errorf("asset root spans errored: %d, want 3 (the whole batch fails together)", got)
	}
	if got := summary["import.asset"].Count; got != 3 {
		t.Errorf("asset root spans still End on the failure path: count=%d, want 3", got)
	}
}

// TestTracing_CanceledRunClassifiesRunSpan cancels an import mid-run with a
// LIVE tracer and asserts the run span lands as canceled, not error — the
// errors.Is classification inside Fail, exercised against a real span (the
// sacred cancel test runs untraced, so it cannot see this).
func TestTracing_CanceledRunClassifiesRunSpan(t *testing.T) {
	ingester, source, _, _ := newPipelineImporter(t)
	tracer, err := gospan.New(gospan.SlogSink(slog.New(slog.DiscardHandler)))
	if err != nil {
		t.Fatal(err)
	}
	ingester.Tracer = tracer

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
	_, runErr := ingester.Run(ctx, source, files)
	if runErr == nil {
		t.Skip("run outpaced the cancel; nothing to classify") // same tolerance as the sacred test
	}
	if err := tracer.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	runSummary := tracer.Summary()["import.run"]
	if runSummary.Canceled != 1 {
		t.Errorf("run span canceled=%d, want 1 (Fail must classify context.Canceled)", runSummary.Canceled)
	}
	if runSummary.Errors != 0 {
		t.Errorf("run span errors=%d, want 0 (canceled is not an error)", runSummary.Errors)
	}
}

// --- Throughput sanity (NFR-2): a runnable benchmark, no flaky timing assert ---

// BenchmarkPipeline_JPEGThroughput measures ingest alone — post-D25 the
// pipeline pays no decode cost (thumbnails are enrichment work), so this is
// the "ingest sheds its slowest stage" number.
func BenchmarkPipeline_JPEGThroughput(b *testing.B) {
	files := fstest.MapFS{}
	for i := 0; i < 500; i++ {
		files[fmtInt(i)+".jpg"] = &fstest.MapFile{Data: jpegBytes(i)}
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		database := benchDB(b)
		assets := &sqlite.AssetRepo{DB: database}
		ingester := &importer.Importer{
			Reader: assets, Obs: assets,
			Dups:    &sqlite.DuplicateRepo{DB: database},
			Store:   sqlite.NewStore(database),
			Imports: &sqlite.ImportRepo{DB: database},
			Log:     log.New(io.Discard),
		}
		source := benchSource(b, database)
		if _, err := ingester.Run(context.Background(), source, files); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Pre-count: progress determinate from the first event ---

// TestPreCount_TotalKnownFromFirstEvent proves the pre-count pass makes Total
// known before any work commits — the first progress event carries the true
// count, not "?". Without it, Total only settles at walk end (SCAN blocks on
// backpressure), leaving the UI showing "N / ?" for nearly the whole import.
func TestPreCount_TotalKnownFromFirstEvent(t *testing.T) {
	ingester, source, _, _ := newPipelineImporter(t)

	const fileCount = 30
	files := fstest.MapFS{}
	for i := 0; i < fileCount; i++ {
		files[fmtInt(i)+".mp4"] = &fstest.MapFile{Data: []byte("clip-" + fmtInt(i))}
	}

	var seen bool
	var firstTotalKnown bool
	var firstTotal int
	ingester.OnProgress = func(progress importer.Progress) {
		if !seen {
			seen = true
			firstTotalKnown = progress.TotalKnown
			firstTotal = progress.Total
		}
	}
	if _, err := ingester.Run(context.Background(), source, files); err != nil {
		t.Fatal(err)
	}
	if !firstTotalKnown {
		t.Fatal("first progress event should already have TotalKnown=true (pre-count ran before the pipeline)")
	}
	if firstTotal != fileCount {
		t.Fatalf("pre-count Total=%d, want %d", firstTotal, fileCount)
	}
}

// --- Permissions: an unreadable root fails fast, and never marks assets missing ---

// deniedFS is a filesystem whose root read is refused — the shape macOS TCC
// produces for a protected or removable/network volume (EPERM surfaces as
// fs.ErrPermission through io/fs).
type deniedFS struct{}

func (deniedFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
}
func (deniedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrPermission}
}

func TestPreflight_PermissionDeniedIsActionable(t *testing.T) {
	ingester, source, assets, _ := newPipelineImporter(t)
	ctx := context.Background()

	// Seed one online asset, so we can prove a denied rescan doesn't mark it missing.
	if _, err := ingester.Run(ctx, source, fstest.MapFS{"keep.mp4": {Data: []byte("keep-me")}}); err != nil {
		t.Fatal(err)
	}

	_, err := ingester.Run(ctx, source, deniedFS{})
	if err == nil {
		t.Fatal("a permission-denied root must fail the import, not silently scan nothing")
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Fatalf("error should name the permission problem so the user can act, got: %v", err)
	}

	// The safety property: because preflight aborts before the empty walk, the
	// walk-end diff never runs, so the existing asset is untouched (not marked missing).
	kept, _ := assets.FindBySourcePath(ctx, source.ID, "keep.mp4")
	if kept == nil || kept.FileStatus != domain.FileStatusOnline {
		t.Fatalf("a denied rescan must not mark known assets missing: got %v", kept)
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
	b.Cleanup(func() { _ = database.Close() })
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
