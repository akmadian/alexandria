package enrichment

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/akmadian/alexandria/internal/assettype"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/thumbnailer"
)

// The thumbnail job (task 19, D25): the first real definition on the engine.
// The producer is a thin dispatcher — the assettype registry row holds the
// strategy (decode for raster, exiftool embedded preview for RAW), and the
// Thumbnailer instance passed at construction carries every runtime dependency
// a strategy needs. This file owns only the glue: absolute-path resolution,
// strategy dispatch, DLQ reason taxonomy, and the artifact commit.

// VolumeResolver is the one capability producers need to turn a stored key back
// into a file on disk: the absolute path of an asset's (volume, volume-relative
// path). The catalog stores (volume_id, relative_path) and the mount point is
// resolved live, so producers cannot join it themselves. Deliberately narrow —
// a background producer holds no volume mutation (narrowest-interface doctrine,
// catalog/interfaces.go). *volume.Resolver satisfies it.
type VolumeResolver interface {
	Absolute(ctx context.Context, volumeID, relativePath string) (string, error)
}

// thumbnailProducer returns the thumbnail ProduceFunc. volumes resolves an
// asset's absolute path — strategies (and the exiftool daemon in particular)
// need a real absolute path on disk, which only the live mount resolution can
// supply.
func thumbnailProducer(thumbnails *thumbnailer.Thumbnailer, volumes VolumeResolver) ProduceFunc {
	return func(ctx context.Context, asset *domain.Asset, _ func()) (ApplyFunc, error) {
		handler, known := assettype.Classify(asset.Extension)
		if !known || handler.Thumb == nil {
			// The dispatch recheck filters on applicability, so reaching here means
			// the registry changed under a queued job — skip-shaped, but a producer
			// can only fail; make the reason honest.
			return nil, Fail("not_applicable", fmt.Errorf("no thumbnail strategy for extension %q", asset.Extension))
		}
		absolutePath, err := volumes.Absolute(ctx, asset.VolumeID, asset.RelativePath)
		if err != nil {
			return nil, Fail("volume_unresolved", fmt.Errorf("resolve volume %s for asset %s: %w", asset.VolumeID, asset.ID, err))
		}
		if err := handler.Thumb(thumbnails, ctx, absolutePath, asset.ID); err != nil {
			return nil, Fail(thumbnailReason(err), err)
		}
		return func(ctx context.Context, writer catalog.AssetDerivedWriter) error {
			return writer.SetThumbnailAt(ctx, asset.ID, time.Now().UTC())
		}, nil
	}
}

// thumbnailReason maps a strategy failure onto the DLQ reason taxonomy:
// tool_unavailable (the exiftool daemon is not running — capability exists,
// tool missing), read_failed (the source file could not be opened), and
// decode_failed (everything else: undecodable bytes, no embedded preview,
// encode errors).
func thumbnailReason(err error) string {
	switch {
	case errors.Is(err, thumbnailer.ErrExiftoolUnavailable):
		return "tool_unavailable"
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, fs.ErrPermission):
		return "read_failed"
	default:
		return "decode_failed"
	}
}

// thumbnailTimeout budgets one thumbnail: a generous base plus a per-byte rate
// so a giant TIFF/PSD-class decode is not misread as a hang. Watchdog is not
// needed — decode is in-process, and the exiftool daemon answers in
// milliseconds against a warm process.
func thumbnailTimeout(sizeBytes int64, _ domain.FileType) time.Duration {
	return 30*time.Second + time.Duration(sizeBytes/(8<<20))*time.Second
}

// thumbnailWeight reserves CPU-budget tokens proportional to input size, since
// an in-flight decode holds the fully-decoded image in memory (D28: bounding
// peak memory by construction). One token per started 32 MiB — the stated
// assumption awaiting gospan samples-table calibration (see JobDefinition.Weight).
func thumbnailWeight(sizeBytes int64) int64 {
	return 1 + sizeBytes/(32<<20)
}
