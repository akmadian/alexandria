package enrichment

import (
	"context"
	"errors"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/thumbnailer"
)

// These are internal tests for the thumbnail producer's GLUE — path
// resolution, strategy dispatch, the DLQ reason taxonomy, and the policy
// functions. The strategies themselves are tested in internal/thumbnailer;
// the full engine loop is exercised end-to-end by the importer acceptance
// suite (import → converge, corrupt → DLQ → heal).

// fakeSourceResolver serves one source by ID — the whole SourceResolver
// surface (the producer holds resolution only, never source mutation).
type fakeSourceResolver struct {
	source *domain.Source
}

func (f *fakeSourceResolver) Get(_ context.Context, id string) (*domain.Source, error) {
	if f.source != nil && f.source.ID == id {
		return f.source, nil
	}
	return nil, nil
}

// recordingDerivedWriter captures the ApplyFunc's side effect.
type recordingDerivedWriter struct {
	thumbnailedAssetID string
	thumbnailedAt      time.Time
}

func (w *recordingDerivedWriter) SetThumbnailAt(_ context.Context, id string, at time.Time) error {
	w.thumbnailedAssetID, w.thumbnailedAt = id, at
	return nil
}
func (w *recordingDerivedWriter) ClearDerived(context.Context, string) error { panic("unused") }

func thumbnailFixture(t *testing.T) (*thumbnailer.Thumbnailer, *fakeSourceResolver, *domain.Asset) {
	t.Helper()
	sourceDir := t.TempDir()
	pixels := image.NewRGBA(image.Rect(0, 0, 16, 16))
	file, err := os.Create(filepath.Join(sourceDir, "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(file, pixels, nil); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	source := &domain.Source{ID: "source-1", BasePath: sourceDir}
	asset := &domain.Asset{ID: "asset-1", SourceID: source.ID, RelativePath: "photo.jpg", Extension: "jpg"}
	return thumbnailer.New(t.TempDir()), &fakeSourceResolver{source: source}, asset
}

func TestThumbnailProducer_ProducesAndApplies(t *testing.T) {
	thumbnails, sources, asset := thumbnailFixture(t)
	produce := thumbnailProducer(thumbnails, sources)

	apply, err := produce(context.Background(), asset, func() {})
	if err != nil {
		t.Fatalf("produce: %v", err)
	}
	if _, err := os.Stat(thumbnails.Path(asset.ID, 512)); err != nil {
		t.Fatalf("no thumbnail file produced: %v", err)
	}
	writer := &recordingDerivedWriter{}
	if err := apply(context.Background(), writer); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if writer.thumbnailedAssetID != asset.ID || writer.thumbnailedAt.IsZero() {
		t.Fatalf("apply recorded %q at %v, want %q at nonzero", writer.thumbnailedAssetID, writer.thumbnailedAt, asset.ID)
	}
}

func TestThumbnailProducer_FailureTaxonomy(t *testing.T) {
	t.Run("missing file is read_failed", func(t *testing.T) {
		thumbnails, sources, asset := thumbnailFixture(t)
		asset.RelativePath = "gone.jpg"
		_, err := thumbnailProducer(thumbnails, sources)(context.Background(), asset, func() {})
		assertReason(t, err, "read_failed")
	})
	t.Run("undecodable bytes are decode_failed", func(t *testing.T) {
		thumbnails, sources, asset := thumbnailFixture(t)
		if err := os.WriteFile(filepath.Join(sources.source.BasePath, "photo.jpg"), []byte("\xff\xd8garbage"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := thumbnailProducer(thumbnails, sources)(context.Background(), asset, func() {})
		assertReason(t, err, "decode_failed")
	})
	t.Run("RAW without the daemon is tool_unavailable", func(t *testing.T) {
		thumbnails, sources, asset := thumbnailFixture(t) // Exiftool nil
		asset.Extension = "cr2"
		_, err := thumbnailProducer(thumbnails, sources)(context.Background(), asset, func() {})
		assertReason(t, err, "tool_unavailable")
	})
	t.Run("unknown source is source_unknown", func(t *testing.T) {
		thumbnails, sources, asset := thumbnailFixture(t)
		asset.SourceID = "no-such-source"
		_, err := thumbnailProducer(thumbnails, sources)(context.Background(), asset, func() {})
		assertReason(t, err, "source_unknown")
	})
	t.Run("extension without a strategy is not_applicable", func(t *testing.T) {
		thumbnails, sources, asset := thumbnailFixture(t)
		asset.Extension = "mp4" // registered, no thumbnail strategy
		_, err := thumbnailProducer(thumbnails, sources)(context.Background(), asset, func() {})
		assertReason(t, err, "not_applicable")
	})
}

func assertReason(t *testing.T, err error, wantReason string) {
	t.Helper()
	var reasonError *ReasonError
	if !errors.As(err, &reasonError) {
		t.Fatalf("want a ReasonError(%s), got %v", wantReason, err)
	}
	if reasonError.ReasonCode != wantReason {
		t.Fatalf("reason = %q, want %q", reasonError.ReasonCode, wantReason)
	}
}

func TestThumbnailPolicies(t *testing.T) {
	if thumbnailTimeout(0, domain.FileTypeImage) <= 0 {
		t.Fatal("timeout must have a positive base")
	}
	small, huge := thumbnailTimeout(1<<20, domain.FileTypeImage), thumbnailTimeout(500<<20, domain.FileTypeImage)
	if huge <= small {
		t.Fatalf("timeout must grow with size: %v then %v", small, huge)
	}
	if thumbnailWeight(0) != 1 {
		t.Fatalf("weight floor = %d, want 1", thumbnailWeight(0))
	}
	if thumbnailWeight(100<<20) <= thumbnailWeight(1<<20) {
		t.Fatal("weight must grow with size")
	}
}

// compile-time proof the fakes satisfy the real interfaces.
var (
	_ SourceResolver             = (*fakeSourceResolver)(nil)
	_ catalog.AssetDerivedWriter = (*recordingDerivedWriter)(nil)
)
