package metadata

import (
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

// ExtractRaster reads pixel dimensions (via the stdlib image decoders) and, when
// present, EXIF. It is a metadata.ExtractFunc for the standard raster formats
// (JPEG/PNG/GIF) — distinct from RAW, whose metadata comes from its own decoder.
// Everything is best-effort: a failure in one part leaves those fields nil rather
// than failing the whole extraction — a corrupt EXIF block must not stop the file
// being indexed.
func ExtractRaster(r io.ReadSeeker) (Metadata, error) {
	var md Metadata

	if _, err := r.Seek(0, io.SeekStart); err == nil {
		if cfg, _, err := image.DecodeConfig(r); err == nil {
			md.Width = intPtr(cfg.Width)
			md.Height = intPtr(cfg.Height)
		}
	}

	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return md, nil
	}
	rawExif, err := exif.SearchAndExtractExifWithReader(r)
	if err != nil {
		if errors.Is(err, exif.ErrNoExif) {
			return md, nil // no EXIF (png/gif/plain jpeg) is normal
		}
		return md, fmt.Errorf("reading exif: %w", err) // corrupt: caller logs, keeps dimensions
	}
	tags, _, err := exif.GetFlatExifData(rawExif, nil)
	if err != nil {
		return md, fmt.Errorf("parsing exif: %w", err)
	}
	applyExif(&md, indexTags(tags))
	return md, nil
}

func indexTags(tags []exif.ExifTag) map[string]exif.ExifTag {
	byName := make(map[string]exif.ExifTag, len(tags))
	for _, t := range tags {
		if _, seen := byName[t.TagName]; !seen { // IFD0 wins over thumbnail IFD1
			byName[t.TagName] = t
		}
	}
	return byName
}

func applyExif(md *Metadata, tags map[string]exif.ExifTag) {
	md.CameraMake = exifString(tags, "Make")
	md.CameraModel = exifString(tags, "Model")
	md.LensModel = exifString(tags, "LensModel")
	md.Creator = exifString(tags, "Artist")
	md.Copyright = exifString(tags, "Copyright")
	md.CapturedAt = exifTime(tags, "DateTimeOriginal")
	md.Aperture = exifRatFloat(tags, "FNumber")
	md.FocalLengthMM = exifRatFloat(tags, "FocalLength")
	md.ShutterSpeed = exifShutter(tags, "ExposureTime")
	md.ISO = exifInt(tags, "ISOSpeedRatings")
	if md.ISO == nil {
		md.ISO = exifInt(tags, "PhotographicSensitivity") // EXIF 2.3+ name
	}
	md.GPSLat = exifGPS(tags, "GPSLatitude", "GPSLatitudeRef", "S")
	md.GPSLon = exifGPS(tags, "GPSLongitude", "GPSLongitudeRef", "W")
}

func exifString(tags map[string]exif.ExifTag, name string) *string {
	t, ok := tags[name]
	if !ok {
		return nil
	}
	s := strings.TrimSpace(tagString(t))
	if s == "" {
		return nil
	}
	return &s
}

func tagString(t exif.ExifTag) string {
	if s, ok := t.Value.(string); ok {
		return s
	}
	return t.Formatted
}

// exifTime parses an EXIF timestamp. EXIF has no timezone, so we parse it as
// wall-clock (UTC-labelled) — the capture time displays as the camera recorded.
func exifTime(tags map[string]exif.ExifTag, name string) *time.Time {
	t, ok := tags[name]
	if !ok {
		return nil
	}
	parsed, err := time.Parse("2006:01:02 15:04:05", strings.TrimSpace(tagString(t)))
	if err != nil {
		return nil
	}
	return &parsed
}

func firstRat(t exif.ExifTag) (exifcommon.Rational, bool) {
	if rs, ok := t.Value.([]exifcommon.Rational); ok && len(rs) > 0 {
		return rs[0], true
	}
	return exifcommon.Rational{}, false
}

func ratFloat(r exifcommon.Rational) float64 {
	if r.Denominator == 0 {
		return 0
	}
	return float64(r.Numerator) / float64(r.Denominator)
}

func exifRatFloat(tags map[string]exif.ExifTag, name string) *float64 {
	t, ok := tags[name]
	if !ok {
		return nil
	}
	r, ok := firstRat(t)
	if !ok || r.Denominator == 0 {
		return nil
	}
	v := ratFloat(r)
	return &v
}

// exifShutter renders ExposureTime as a human shutter speed: "1/250" for fast,
// decimal seconds for slow.
func exifShutter(tags map[string]exif.ExifTag, name string) *string {
	t, ok := tags[name]
	if !ok {
		return nil
	}
	r, ok := firstRat(t)
	if !ok || r.Denominator == 0 {
		return nil
	}
	v := ratFloat(r)
	var s string
	switch {
	case v <= 0:
		return nil
	case v < 1:
		s = fmt.Sprintf("1/%d", int(math.Round(1/v)))
	default:
		s = strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".")
	}
	return &s
}

func exifInt(tags map[string]exif.ExifTag, name string) *int {
	t, ok := tags[name]
	if !ok {
		return nil
	}
	switch v := t.Value.(type) {
	case []uint16:
		if len(v) > 0 {
			n := int(v[0])
			return &n
		}
	case []uint32:
		if len(v) > 0 {
			n := int(v[0])
			return &n
		}
	}
	if n, err := strconv.Atoi(strings.TrimSpace(t.FormattedFirst)); err == nil {
		return &n
	}
	return nil
}

// exifGPS converts a DMS coordinate (3 rationals) plus its N/S/E/W ref into
// signed decimal degrees.
func exifGPS(tags map[string]exif.ExifTag, valueTag, refTag, negativeRef string) *float64 {
	t, ok := tags[valueTag]
	if !ok {
		return nil
	}
	rs, ok := t.Value.([]exifcommon.Rational)
	if !ok || len(rs) < 3 {
		return nil
	}
	deg := ratFloat(rs[0]) + ratFloat(rs[1])/60 + ratFloat(rs[2])/3600
	if ref := exifString(tags, refTag); ref != nil && strings.EqualFold(*ref, negativeRef) {
		deg = -deg
	}
	return &deg
}
