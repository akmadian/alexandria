// Package thumbnailer generates cached thumbnails from asset files, mapping each
// format onto one shared decode→resize→encode path. This package owns the
// decoders and the on-disk layout; per-type dispatch lives in the assettype
// registry (internal/assettype), which points each extension at the right GenFunc
// (nil = no generator → generic card, not an error).
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
)

// DefaultQuality is the JPEG quality for generated thumbnails.
const DefaultQuality = 80

// GenFunc decodes r once and writes one JPEG per requested size. dst maps a size
// to its output path. A nil GenFunc means the type has no generator.
type GenFunc func(r io.ReadSeeker, sizes []int, quality int, dst func(size int) string) error

// Thumbnailer writes thumbnails for assetID using the supplied generator,
// sourcing pixels from an opened, seekable file. It reports whether a thumbnail
// was produced: a nil generator is a no-op (false, nil) — callers use ok to
// decide whether to record the thumbnail (don't flag an asset thumbnailed when
// no file was written).
type Thumbnailer interface {
	Generate(gen GenFunc, r io.ReadSeeker, assetID string) (ok bool, err error)
}

// Registry owns the output layout: where thumbnails go (Dir), at what sizes, and
// at what JPEG quality. It carries no per-type dispatch — the caller passes the
// generator for the file at hand.
type Registry struct {
	Dir     string // app-data thumbnails root
	Sizes   []int  // long-edge pixels; one file generated per entry
	Quality int    // JPEG quality 1..100
}

// New returns a Registry writing under dir.
//
// ponytail: one size (512) for v1 — nothing consumes larger tiers yet. Add a
// preview tier (e.g. 2048) here when the loupe needs it and every asset gets a
// file per size, no other change.
func New(dir string) Registry {
	return Registry{Dir: dir, Sizes: []int{512}, Quality: DefaultQuality}
}

// Generate writes one thumbnail per configured size using gen. A nil gen (the
// type has no generator) is a no-op reporting ok=false.
func (reg Registry) Generate(gen GenFunc, r io.ReadSeeker, assetID string) (bool, error) {
	if gen == nil {
		return false, nil
	}
	for _, size := range reg.Sizes {
		if err := os.MkdirAll(filepath.Dir(reg.Path(assetID, size)), 0o755); err != nil {
			return false, fmt.Errorf("thumbnail dir: %w", err)
		}
	}
	dst := func(size int) string { return reg.Path(assetID, size) }
	if err := gen(r, reg.Sizes, reg.Quality, dst); err != nil {
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
