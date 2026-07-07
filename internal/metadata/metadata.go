// Package metadata extracts normalized asset metadata (dimensions, EXIF, …) from
// files, mapping each format's raw tags onto one shared Metadata struct. This
// package owns the decoders; per-type dispatch lives in the assettype registry
// (internal/assettype), which points each extension at the right ExtractFunc.
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

// ExtractFunc reads normalized metadata from an opened, seekable file. A nil
// ExtractFunc in the registry means the type has no extractor yet — that's not a
// failure, the caller simply skips extraction. Extraction is best-effort: a
// corrupt metadata block yields partial data plus an error, never a stop.
type ExtractFunc func(r io.ReadSeeker) (Metadata, error)

func intPtr(n int) *int { return &n }
