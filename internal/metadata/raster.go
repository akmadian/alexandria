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
func ExtractRaster(reader io.ReadSeeker) (Metadata, error) {
	var result Metadata

	if _, err := reader.Seek(0, io.SeekStart); err == nil {
		if cfg, _, err := image.DecodeConfig(reader); err == nil {
			result.Width = intPtr(cfg.Width)
			result.Height = intPtr(cfg.Height)
		}
	}

	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return result, nil
	}
	rawExif, err := exif.SearchAndExtractExifWithReader(reader)
	if err != nil {
		if errors.Is(err, exif.ErrNoExif) {
			return result, nil // no EXIF (png/gif/plain jpeg) is normal
		}
		return result, fmt.Errorf("reading exif: %w", err) // corrupt: caller logs, keeps dimensions
	}
	tags, _, err := exif.GetFlatExifData(rawExif, nil)
	if err != nil {
		return result, fmt.Errorf("parsing exif: %w", err)
	}
	applyExif(&result, indexTags(tags))
	return result, nil
}

func indexTags(tags []exif.ExifTag) map[string]exif.ExifTag {
	byName := make(map[string]exif.ExifTag, len(tags))
	for i := range tags {
		if _, seen := byName[tags[i].TagName]; !seen { // IFD0 wins over thumbnail IFD1
			byName[tags[i].TagName] = tags[i]
		}
	}
	return byName
}

func applyExif(result *Metadata, tags map[string]exif.ExifTag) {
	result.CameraMake = exifString(tags, "Make")
	result.CameraModel = exifString(tags, "Model")
	result.LensModel = exifString(tags, "LensModel")
	result.Creator = exifString(tags, "Artist")
	result.Copyright = exifString(tags, "Copyright")
	result.CapturedAt = exifTime(tags, "DateTimeOriginal")
	result.Aperture = exifRatFloat(tags, "FNumber")
	result.FocalLengthMM = exifRatFloat(tags, "FocalLength")
	result.ShutterSpeed = exifShutter(tags, "ExposureTime")
	result.ISO = exifInt(tags, "ISOSpeedRatings")
	if result.ISO == nil {
		result.ISO = exifInt(tags, "PhotographicSensitivity") // EXIF 2.3+ name
	}
	result.GPSLat = exifGPS(tags, "GPSLatitude", "GPSLatitudeRef", "S")
	result.GPSLon = exifGPS(tags, "GPSLongitude", "GPSLongitudeRef", "W")
	result.ColorSpace = exifColorSpace(tags)
}

// exifColorSpace maps the EXIF ColorSpace short (0xA001) onto its common display
// names; rare vendor codes yield nil. EXIF has no standard code for Adobe RGB:
// cameras write Uncalibrated (65535) plus InteroperabilityIndex "R03", so that
// pair reports as Adobe RGB.
func exifColorSpace(tags map[string]exif.ExifTag) *string {
	code := exifInt(tags, "ColorSpace")
	if code == nil {
		return nil
	}
	var name string
	switch *code {
	case 1:
		name = "sRGB"
	case 2: // non-standard, but some cameras write it for Adobe RGB
		name = "Adobe RGB"
	case 65535:
		name = "Uncalibrated"
		if index := exifString(tags, "InteroperabilityIndex"); index != nil && strings.HasPrefix(*index, "R03") {
			name = "Adobe RGB"
		}
	default:
		return nil
	}
	return &name
}

func exifString(tags map[string]exif.ExifTag, name string) *string {
	tag, ok := tags[name]
	if !ok {
		return nil
	}
	// Fixed-length EXIF ASCII fields (e.g. LensModel) are NUL-padded to their slot
	// width; TrimSpace alone leaves the NULs, so they must be cut too or they land
	// verbatim in the catalog.
	trimmed := strings.Trim(tagString(&tag), " \t\r\n\x00")
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func tagString(t *exif.ExifTag) string {
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
	parsed, err := time.Parse("2006:01:02 15:04:05", strings.TrimSpace(tagString(&t)))
	if err != nil {
		return nil
	}
	return &parsed
}

func firstRat(t *exif.ExifTag) (exifcommon.Rational, bool) {
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
	r, ok := firstRat(&t)
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
	r, ok := firstRat(&t)
	if !ok || r.Denominator == 0 {
		return nil
	}
	seconds := ratFloat(r)
	var formatted string
	switch {
	case seconds <= 0:
		return nil
	case seconds < 1:
		formatted = fmt.Sprintf("1/%d", int(math.Round(1/seconds)))
	default:
		formatted = strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", seconds), "0"), ".")
	}
	return &formatted
}

func exifInt(tags map[string]exif.ExifTag, name string) *int {
	tag, ok := tags[name]
	if !ok {
		return nil
	}
	switch values := tag.Value.(type) {
	case []uint16:
		if len(values) > 0 {
			n := int(values[0])
			return &n
		}
	case []uint32:
		if len(values) > 0 {
			n := int(values[0])
			return &n
		}
	}
	if n, err := strconv.Atoi(strings.TrimSpace(tag.FormattedFirst)); err == nil {
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
