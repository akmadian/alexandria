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
