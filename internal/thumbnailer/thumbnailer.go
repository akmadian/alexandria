// Package thumbnailer generates cached thumbnails from asset files, mapping each
// format onto one shared decode→resize→encode path. It mirrors the metadata
// package: dispatch by MIME, unsupported types are a no-op (nil error), and
// generation is best-effort. Add a format by registering its MIME in New.
//
// Thumbnails are throwaway JPEGs, rebuildable from source. The on-disk layout is
// keyed by asset ID, not file path, because a file's path can change without a
// re-import (catalog-first). Callers derive the path from the ID via Path rather
// than storing it — see the schema's thumbnail_at flag for "is it generated?".
package thumbnailer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/charmbracelet/log"
)

// DefaultQuality is the JPEG quality for generated thumbnails.
const DefaultQuality = 80

// Thumbnailer writes thumbnails for assetID, sourcing pixels from an opened,
// seekable file. mime selects the generator. It reports whether a thumbnail was
// produced: an unsupported type is a no-op (false, nil) — a missing generator is
// not a failure, same as metadata. Callers use ok to decide whether to record
// the thumbnail (don't flag an asset thumbnailed when no file was written).
type Thumbnailer interface {
	Generate(r io.ReadSeeker, mime, assetID string) (ok bool, err error)
}

// genFunc decodes r once and writes one JPEG per requested size. dst maps a size
// to its output path.
type genFunc func(r io.ReadSeeker, sizes []int, quality int, dst func(size int) string) error

// Registry dispatches generation to a per-MIME function and owns the output
// layout (Dir, Sizes). Zero MIMEs match → Generate is a no-op.
type Registry struct {
	Dir     string // app-data thumbnails root
	Sizes   []int  // long-edge pixels; one file generated per entry
	Quality int    // JPEG quality 1..100
	byMIME  map[string]genFunc
}

// New returns a Registry writing under dir with the built-in raster generators.
// One size (512) today; add more sizes here (e.g. a 2048 preview tier) and every
// asset gets a file per size — no other change. Only the raster formats the Go
// stdlib can decode are registered; raw, video, PDF, etc. are follow-ups.
func New(dir string) Registry {
	return Registry{
		Dir:     dir,
		Sizes:   []int{512, 1024, 2048},
		Quality: DefaultQuality,
		byMIME: map[string]genFunc{
			"image/jpeg": generateImage,
			"image/png":  generateImage,
			"image/gif":  generateImage,
		},
	}
}

// Generate implements Thumbnailer, writing one thumbnail per configured size.
func (reg Registry) Generate(r io.ReadSeeker, mime, assetID string) (bool, error) {
	fn, ok := reg.byMIME[mime]
	if !ok {
		// Supported asset type with no thumbnail generator yet (raw, video, …).
		log.Warn("no thumbnailer for file type", "mime", mime, "asset", assetID)
		return false, nil
	}
	log.Debug("generating thumbnails", "asset", assetID, "mime", mime, "sizes", reg.Sizes)
	for _, size := range reg.Sizes {
		if err := os.MkdirAll(filepath.Dir(reg.Path(assetID, size)), 0o755); err != nil {
			return false, fmt.Errorf("thumbnail dir: %w", err)
		}
	}
	dst := func(size int) string { return reg.Path(assetID, size) }
	if err := fn(r, reg.Sizes, reg.Quality, dst); err != nil {
		return false, fmt.Errorf("thumbnail %s: %w", assetID, err)
	}
	return true, nil
}

// Path returns the on-disk thumbnail path for an asset at a given size. Pure
// string derivation, no I/O: <Dir>/<size>/<2-char shard>/<id>.jpg. The shard
// prefix caps files per directory at large library sizes.
func (reg Registry) Path(assetID string, size int) string {
	shard := assetID
	if len(shard) >= 2 {
		shard = shard[:2]
	}
	return filepath.Join(reg.Dir, strconv.Itoa(size), shard, assetID+".jpg")
}
