package metadata_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/metadata"
)

func deref[T any](p *T) any {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// Real-data test: the committed testdata JPEGs are downscaled exports that carry
// dimensions + rights metadata (creator/copyright) but no camera EXIF.
func TestExtract_RealJPEG_DimensionsAndRights(t *testing.T) {
	f, err := os.Open("../../testdata/_6160345-.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	md, err := metadata.ExtractRaster(f)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if md.Width == nil || *md.Width != 2160 || md.Height == nil || *md.Height != 1620 {
		t.Errorf("dimensions = %vx%v, want 2160x1620", deref(md.Width), deref(md.Height))
	}
	if md.Creator == nil || *md.Creator != "Ari Madian" {
		t.Errorf("creator = %v, want Ari Madian", deref(md.Creator))
	}
	if md.Copyright == nil || *md.Copyright != "ALL RIGHTS RESERVED | Ari Madian" {
		t.Errorf("copyright = %v", deref(md.Copyright))
	}
}

func TestExtract_Graceful(t *testing.T) {
	// Garbage bytes → best-effort, no panic, no dimensions. (Whether a type HAS an
	// extractor is the assettype registry's concern; here we only prove the decoder
	// degrades on junk input.)
	md, err := metadata.ExtractRaster(strings.NewReader("not a real jpeg"))
	if err != nil {
		t.Fatalf("garbage jpeg: %v", err)
	}
	if md.Width != nil {
		t.Error("garbage input should yield no dimensions")
	}
}

// Validates the camera/exposure mapping (rationals → floats, ExposureTime → "1/x"
// shutter, ISO int coercion, EXIF timestamp parse) against a real original-camera
// JPEG. testdata/exif-original.jpg is a FUJIFILM X-T5 frame; these are its actual
// recorded values, so a regression in any coercion breaks exactly one assertion.
// The fixture carries no GPS, so the DMS→decimal path is covered separately in
// TestExifGPS_DMSToDecimal.
func TestExtract_FullEXIF(t *testing.T) {
	const path = "../../testdata/exif-original.jpg"
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("no full-EXIF fixture at %s (drop an original camera JPEG there to enable): %v", path, err)
	}
	defer f.Close()

	md, err := metadata.ExtractRaster(f)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	if md.CameraMake == nil || *md.CameraMake != "FUJIFILM" {
		t.Errorf("CameraMake = %v, want FUJIFILM", deref(md.CameraMake))
	}
	if md.CameraModel == nil || *md.CameraModel != "X-T5" {
		t.Errorf("CameraModel = %v, want X-T5", deref(md.CameraModel))
	}
	if md.LensModel == nil || *md.LensModel != "XF55-200mmF3.5-4.8 R LM OIS" {
		t.Errorf("LensModel = %v, want XF55-200mmF3.5-4.8 R LM OIS", deref(md.LensModel))
	}
	// FNumber rational 11/1 → 11.0 (exifRatFloat).
	if md.Aperture == nil || *md.Aperture != 11 {
		t.Errorf("Aperture = %v, want 11", deref(md.Aperture))
	}
	// FocalLength rational 200/1 → 200.0 (exifRatFloat).
	if md.FocalLengthMM == nil || *md.FocalLengthMM != 200 {
		t.Errorf("FocalLengthMM = %v, want 200", deref(md.FocalLengthMM))
	}
	// ExposureTime 1/3200 (<1s) → "1/3200" (exifShutter fast-branch).
	if md.ShutterSpeed == nil || *md.ShutterSpeed != "1/3200" {
		t.Errorf("ShutterSpeed = %v, want 1/3200", deref(md.ShutterSpeed))
	}
	// ISO coerced to int (exifInt).
	if md.ISO == nil || *md.ISO != 1600 {
		t.Errorf("ISO = %v, want 1600", deref(md.ISO))
	}
	// DateTimeOriginal parsed as UTC-labelled wall-clock (exifTime).
	if md.CapturedAt == nil || !md.CapturedAt.Equal(time.Date(2026, 5, 20, 15, 59, 30, 0, time.UTC)) {
		t.Errorf("CapturedAt = %v, want 2026-05-20 15:59:30 UTC", deref(md.CapturedAt))
	}
}
