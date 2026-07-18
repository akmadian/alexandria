package enrichment

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/thumbnailer"
)

// Internal tests for the cheap-signal producers' GLUE — reading the analysis
// thumbnail off disk, applying the derived column(s), and the DLQ reason
// taxonomy. The signal math is tested in internal/signals; the full engine loop
// (import → thumbnail → signals converge) is the importer acceptance suite.

// writeThumbnail encodes img as the analysis-tier thumbnail JPEG for assetID, so
// a signal producer can read it — the state the thumbnail prerequisite guarantees.
func writeThumbnail(t *testing.T, thumbnails *thumbnailer.Thumbnailer, assetID string, img image.Image) {
	t.Helper()
	path := thumbnails.Path(assetID, thumbnails.AnalysisSize())
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSignalProducers_ProduceAndApply(t *testing.T) {
	thumbnails := thumbnailer.New(t.TempDir())
	asset := &domain.Asset{ID: "asset-signal-1", Extension: "jpg"}
	writeThumbnail(t, thumbnails, asset.ID, image.NewRGBA(image.Rect(0, 0, 64, 64)))
	ctx := context.Background()

	cases := map[string]struct {
		produce ProduceFunc
		check   func(t *testing.T, writer *recordingDerivedWriter)
	}{
		"sharpness": {sharpnessProducer(thumbnails), func(t *testing.T, writer *recordingDerivedWriter) {
			if writer.sharpness == nil {
				t.Fatal("sharpness not applied")
			}
		}},
		"clipping": {clippingProducer(thumbnails), func(t *testing.T, writer *recordingDerivedWriter) {
			if writer.clippingHighlights == nil || writer.clippingShadows == nil {
				t.Fatal("clipping must apply both columns")
			}
		}},
		"phash": {phashProducer(thumbnails), func(t *testing.T, writer *recordingDerivedWriter) {
			if len(writer.phash) != 16 {
				t.Fatalf("phash %q is not 16 hex digits", writer.phash)
			}
		}},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			apply, err := testCase.produce(ctx, asset, func() {})
			if err != nil {
				t.Fatalf("produce: %v", err)
			}
			writer := &recordingDerivedWriter{}
			if err := apply(ctx, writer); err != nil {
				t.Fatalf("apply: %v", err)
			}
			testCase.check(t, writer)
		})
	}
}

func TestSignalProducers_FailureTaxonomy(t *testing.T) {
	ctx := context.Background()
	asset := &domain.Asset{ID: "asset-1", Extension: "jpg"}

	t.Run("missing thumbnail is read_failed", func(t *testing.T) {
		thumbnails := thumbnailer.New(t.TempDir()) // nothing on disk
		for name, produce := range map[string]ProduceFunc{
			"sharpness": sharpnessProducer(thumbnails),
			"clipping":  clippingProducer(thumbnails),
			"phash":     phashProducer(thumbnails),
		} {
			t.Run(name, func(t *testing.T) {
				_, err := produce(ctx, asset, func() {})
				assertReason(t, err, "read_failed")
			})
		}
	})
	t.Run("undecodable thumbnail is decode_failed", func(t *testing.T) {
		thumbnails := thumbnailer.New(t.TempDir())
		path := thumbnails.Path(asset.ID, thumbnails.AnalysisSize())
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("not a jpeg"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := clippingProducer(thumbnails)(ctx, asset, func() {})
		assertReason(t, err, "decode_failed")
	})
}

func TestSignalTimeout_Positive(t *testing.T) {
	if signalTimeout(0, domain.FileTypeImage) <= 0 {
		t.Fatal("signal timeout must be a positive budget")
	}
}
