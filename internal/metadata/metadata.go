// Package metadata extracts normalized asset metadata (dimensions, EXIF, …) from
// files, mapping each format's raw tags onto one shared Metadata struct. Add a
// new format by writing an extractor and registering its MIME type in Default —
// the pipeline and importer never change.
package metadata

import (
	"io"
	"time"
)

// Metadata is the normalized target every extractor maps onto. Pointer fields
// are left nil when a value isn't present. The long tail that doesn't fit a
// first-class field goes in Extended (persisted as JSON).
type Metadata struct {
	Width, Height *int
	DurationSecs  *float64
	CapturedAt    *time.Time
	CameraMake    *string
	CameraModel   *string
	LensModel     *string
	FocalLengthMM *float64
	Aperture      *float64
	ShutterSpeed  *string
	ISO           *int
	GPSLat        *float64
	GPSLon        *float64
	ColorSpace    *string
	BitDepth      *int
	Creator       *string
	Copyright     *string
	Extended      map[string]any
}

// Extractor reads normalized metadata from an opened, seekable file. mime selects
// the mapping. An unsupported type returns a zero Metadata and nil error —
// extraction is best-effort, and a missing extractor is not a failure.
type Extractor interface {
	Extract(r io.ReadSeeker, mime string) (Metadata, error)
}

type extractFunc func(io.ReadSeeker) (Metadata, error)

// Registry dispatches extraction to a per-MIME function.
type Registry struct {
	byMIME map[string]extractFunc
}

// Extract implements Extractor.
func (reg Registry) Extract(r io.ReadSeeker, mime string) (Metadata, error) {
	fn, ok := reg.byMIME[mime]
	if !ok {
		return Metadata{}, nil
	}
	return fn(r)
}

// Default returns a registry with the built-in extractors registered. Non-raw
// raster images today; video/audio/raw are follow-ups (register their MIME here).
func Default() Registry {
	return Registry{byMIME: map[string]extractFunc{
		"image/jpeg": extractImage,
		"image/png":  extractImage,
		"image/gif":  extractImage,
	}}
}

func intPtr(n int) *int { return &n }
