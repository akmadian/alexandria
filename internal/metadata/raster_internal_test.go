package metadata

import (
	"testing"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

// dms builds an EXIF GPS coordinate tag: three rationals (degrees, minutes,
// seconds), the wire form exifGPS converts to signed decimal degrees.
func dms(deg, min, sec uint32) exif.ExifTag {
	return exif.ExifTag{Value: []exifcommon.Rational{
		{Numerator: deg, Denominator: 1},
		{Numerator: min, Denominator: 1},
		{Numerator: sec, Denominator: 1},
	}}
}

// exifGPS is the only EXIF mapping the committed camera fixture can't exercise
// (it carries no GPS), so its DMS→decimal math and hemisphere sign flip are
// covered here against a hand-built tag map. 37°48'30" = 37.808333°.
func TestExifGPS_DMSToDecimal(t *testing.T) {
	const want = 37.808333333333334

	cases := []struct {
		name    string
		ref     string
		negRef  string
		wantVal float64
		wantNil bool
	}{
		{name: "north-positive", ref: "N", negRef: "S", wantVal: want},
		{name: "south-negative", ref: "S", negRef: "S", wantVal: -want},
		{name: "missing-ref-positive", ref: "", negRef: "S", wantVal: want},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tags := map[string]exif.ExifTag{"GPSLatitude": dms(37, 48, 30)}
			if tc.ref != "" {
				tags["GPSLatitudeRef"] = exif.ExifTag{Value: tc.ref}
			}
			got := exifGPS(tags, "GPSLatitude", "GPSLatitudeRef", tc.negRef)
			if got == nil {
				t.Fatal("exifGPS returned nil for a valid coordinate")
			}
			if (*got-tc.wantVal) > 1e-6 || (tc.wantVal-*got) > 1e-6 {
				t.Errorf("exifGPS = %v, want %v", *got, tc.wantVal)
			}
		})
	}
}

// The committed fixtures exercise exifColorSpace's sRGB (code 1) and
// Uncalibrated+R03 (Adobe RGB) paths; the remaining codes and the nil paths are
// covered here against hand-built tag maps.
func TestExifColorSpace_CodeMapping(t *testing.T) {
	colorSpaceTag := func(code uint16) exif.ExifTag {
		return exif.ExifTag{Value: []uint16{code}}
	}
	cases := []struct {
		name    string
		tags    map[string]exif.ExifTag
		want    string
		wantNil bool
	}{
		{name: "srgb", tags: map[string]exif.ExifTag{"ColorSpace": colorSpaceTag(1)}, want: "sRGB"},
		{name: "adobe-rgb-nonstandard-code-2", tags: map[string]exif.ExifTag{"ColorSpace": colorSpaceTag(2)}, want: "Adobe RGB"},
		{name: "uncalibrated-without-interop", tags: map[string]exif.ExifTag{"ColorSpace": colorSpaceTag(65535)}, want: "Uncalibrated"},
		{name: "uncalibrated-r03-is-adobe-rgb", tags: map[string]exif.ExifTag{
			"ColorSpace":            colorSpaceTag(65535),
			"InteroperabilityIndex": {Value: "R03"},
		}, want: "Adobe RGB"},
		{name: "unknown-code-yields-nil", tags: map[string]exif.ExifTag{"ColorSpace": colorSpaceTag(3)}, wantNil: true},
		{name: "missing-tag-yields-nil", tags: map[string]exif.ExifTag{}, wantNil: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := exifColorSpace(tc.tags)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("exifColorSpace = %q, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("exifColorSpace = nil, want %q", tc.want)
			}
			if *got != tc.want {
				t.Errorf("exifColorSpace = %q, want %q", *got, tc.want)
			}
		})
	}
}

// A coordinate with fewer than three rationals is malformed and must yield nil,
// not a panic or a partial value.
func TestExifGPS_MalformedYieldsNil(t *testing.T) {
	tags := map[string]exif.ExifTag{
		"GPSLatitude": {Value: []exifcommon.Rational{{Numerator: 37, Denominator: 1}}},
	}
	if got := exifGPS(tags, "GPSLatitude", "GPSLatitudeRef", "S"); got != nil {
		t.Errorf("malformed DMS should yield nil, got %v", *got)
	}
	// Absent tag entirely → nil.
	if got := exifGPS(map[string]exif.ExifTag{}, "GPSLatitude", "GPSLatitudeRef", "S"); got != nil {
		t.Errorf("absent GPS tag should yield nil, got %v", *got)
	}
}
