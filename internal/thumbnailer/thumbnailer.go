// Package thumbnailer generates cached thumbnails from asset files, mapping each
// format onto one shared resize→encode path. This package owns the decoders and
// the on-disk layout; per-type dispatch lives in the assettype registry
// (internal/assettype), which points each extension at the right GenFunc
// (nil = no generator → generic card, not an error).
//
// A GenFunc is a strategy expressed as a method value on Thumbnailer: the static
// assettype table can hold `thumbnailer.GenerateRaster` at package init because
// the receiver — the Thumbnailer instance carrying the runtime dependencies
// (output layout, the exiftool daemon) — is passed in at call time. The contract
// is the postcondition, never the mechanism: after a successful call, one JPEG
// exists at Path(assetID, size) for every configured size.
//
// Thumbnails are throwaway JPEGs, rebuildable from source. The on-disk layout is
// keyed by asset ID, not file path, because a file's path can change without a
// re-import (catalog-first). Callers derive the path from the ID via Path rather
// than storing it — see the schema's thumbnail_at flag for "is it generated?".
package thumbnailer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/akmadian/alexandria/internal/dependency"
)

// DefaultQuality is the JPEG quality for generated thumbnails.
const DefaultQuality = 80

// GenFunc is one thumbnail strategy: given the Thumbnailer and a source file's
// absolute path, leave one JPEG per configured size at Path(assetID, size).
// A nil GenFunc on an assettype row means the type has no generator.
//
// Ceiling: if a strategy ever needs per-call state that is not derivable from
// (Thumbnailer fields + source path), this signature grows — that is the
// trigger to revisit toward constructed strategy values.
type GenFunc func(thumb *Thumbnailer, ctx context.Context, sourcePath string, assetID string) error

// The strategies the assettype registry rows point at.
var (
	// GenerateRaster decodes the file itself — the stdlib-decodable formats
	// (JPEG/PNG/GIF today).
	GenerateRaster GenFunc = (*Thumbnailer).generateRaster
	// GenerateRawPreview extracts the camera-written embedded JPEG preview via
	// the exiftool daemon, then reuses the raster resize/encode backend on those
	// bytes. Owning RAW decoding is anti-scope: the per-camera quirk table is
	// exiftool's twenty-year moat (D28 — delegation is permanent).
	GenerateRawPreview GenFunc = (*Thumbnailer).generateRawPreview
)

// ErrExiftoolUnavailable is returned by GenerateRawPreview when the Thumbnailer
// was built without a daemon — the capability exists in the registry, the tool
// is missing at runtime. The enrichment producer maps it to a distinct DLQ
// reason (tool_unavailable) so the asset reads "failed: tool missing", never
// an eternal spinner and never a silent skip.
var ErrExiftoolUnavailable = errors.New("thumbnailer: exiftool daemon unavailable")

// Thumbnailer owns the output layout (where thumbnails go, at what sizes and
// JPEG quality) and every runtime dependency a strategy needs. It carries no
// per-type dispatch — the assettype registry hands the caller the strategy for
// the file at hand.
type Thumbnailer struct {
	Dir     string // app-data thumbnails root
	Sizes   []int  // long-edge pixels; one file generated per entry
	Quality int    // JPEG quality 1..100
	// Exiftool is the shared daemon GenerateRawPreview delegates to. Nil means
	// the RAW preview capability is unavailable at runtime (tool undiscovered);
	// RAW jobs then fail into the DLQ with tool_unavailable.
	Exiftool *dependency.ExiftoolDaemon
}

// New returns a Thumbnailer writing under dir.
//
// ponytail: one size (512) for v1 — nothing consumes larger tiers yet. Add a
// preview tier (e.g. 2048) here when the loupe needs it and every asset gets a
// file per size, no other change.
func New(dir string) *Thumbnailer {
	return &Thumbnailer{Dir: dir, Sizes: []int{512}, Quality: DefaultQuality}
}

// Path returns the on-disk thumbnail path for an asset at a given size. Pure
// string derivation, no I/O: <Dir>/<size>/<2-char shard>/<id>.jpg. The shard
// prefix caps files per directory at large library sizes.
func (thumb *Thumbnailer) Path(assetID string, size int) string {
	shard := assetID
	if len(shard) >= 2 {
		shard = shard[:2]
	}
	return filepath.Join(thumb.Dir, strconv.Itoa(size), shard, assetID+".jpg")
}

// ensureDirs creates the sharded output directories for one asset's files.
func (thumb *Thumbnailer) ensureDirs(assetID string) error {
	for _, size := range thumb.Sizes {
		if err := os.MkdirAll(filepath.Dir(thumb.Path(assetID, size)), 0o750); err != nil {
			return fmt.Errorf("thumbnail dir: %w", err)
		}
	}
	return nil
}
