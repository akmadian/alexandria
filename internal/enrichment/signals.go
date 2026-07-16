package enrichment

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"time"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/signals"
	"github.com/akmadian/alexandria/internal/thumbnailer"
)

// The cheap-signal jobs (task 20, D28): sharpness, clipping, phash — three kinds
// whose prerequisite is the thumbnail artifact. Each producer reads the analysis
// thumbnail off disk, hands it to a pure function in internal/signals, and returns
// the ApplyFunc that commits the derived column(s). The thumbnail is the operand
// the signal READS (D28: a signal reads its parents' artifact, never the original
// bytes) — re-decoding a 512px thumb costs single-digit ms, the knowing price of
// per-artifact atomicity.

// errThumbnailDecode marks a decode-stage failure so signalReason can tell it
// apart from an open-stage failure without inferring from errno. Only a decode
// failure is decode_failed; every open failure (missing, permission, fd
// exhaustion, I/O error) is read_failed.
var errThumbnailDecode = errors.New("thumbnail decode")

// openAnalysisThumbnail opens and decodes the thumbnail tier the signals analyze.
// The thumbnail is a prerequisite artifact, so it is normally present; a missing
// or unreadable file (concurrent deletion) becomes a DLQ row via signalReason.
func openAnalysisThumbnail(thumbnails *thumbnailer.Thumbnailer, assetID string) (image.Image, error) {
	path := thumbnails.Path(assetID, thumbnails.AnalysisSize())
	file, err := os.Open(path)
	if err != nil {
		return nil, err // open-stage failure — signalReason maps it to read_failed
	}
	defer func() { _ = file.Close() }()
	img, err := jpeg.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", errThumbnailDecode, path, err)
	}
	return img, nil
}

// signalReason maps a thumbnail-read failure onto the DLQ reason taxonomy: any
// open-stage failure is read_failed (the file could not be opened — missing,
// permission, fd exhaustion, I/O error); only a decode-stage failure is
// decode_failed (corrupt thumbnail bytes).
func signalReason(err error) string {
	if errors.Is(err, errThumbnailDecode) {
		return "decode_failed"
	}
	return "read_failed"
}

// sharpnessProducer computes the variance-of-Laplacian focus measure.
func sharpnessProducer(thumbnails *thumbnailer.Thumbnailer) ProduceFunc {
	return func(_ context.Context, asset *domain.Asset, _ func()) (ApplyFunc, error) {
		img, err := openAnalysisThumbnail(thumbnails, asset.ID)
		if err != nil {
			return nil, Fail(signalReason(err), err)
		}
		value := signals.Sharpness(img)
		return func(ctx context.Context, writer catalog.AssetDerivedWriter) error {
			return writer.SetSharpness(ctx, asset.ID, value)
		}, nil
	}
}

// clippingProducer computes highlight and shadow clipping percentages — one kind,
// one histogram pass, two columns committed together.
func clippingProducer(thumbnails *thumbnailer.Thumbnailer) ProduceFunc {
	return func(_ context.Context, asset *domain.Asset, _ func()) (ApplyFunc, error) {
		img, err := openAnalysisThumbnail(thumbnails, asset.ID)
		if err != nil {
			return nil, Fail(signalReason(err), err)
		}
		highlights, shadows := signals.Clipping(img)
		return func(ctx context.Context, writer catalog.AssetDerivedWriter) error {
			return writer.SetClipping(ctx, asset.ID, highlights, shadows)
		}, nil
	}
}

// phashProducer computes the perceptual hash (dHash by default; the algorithm is
// a swappable strategy in internal/signals). The user-facing near-dup query
// surface over these hashes is deferred (DEFERRED §12).
func phashProducer(thumbnails *thumbnailer.Thumbnailer) ProduceFunc {
	return func(_ context.Context, asset *domain.Asset, _ func()) (ApplyFunc, error) {
		img, err := openAnalysisThumbnail(thumbnails, asset.ID)
		if err != nil {
			return nil, Fail(signalReason(err), err)
		}
		hash := signals.FormatHash(signals.DefaultHasher(img))
		return func(ctx context.Context, writer catalog.AssetDerivedWriter) error {
			return writer.SetPhash(ctx, asset.ID, hash)
		}, nil
	}
}

// signalTimeout budgets one cheap-signal job. The input is a fixed-size analysis
// thumbnail, not the original file, so the original's size is irrelevant and the
// budget is a flat, generous constant — the compute is in-process milliseconds.
func signalTimeout(_ int64, _ domain.FileType) time.Duration {
	return 15 * time.Second
}
