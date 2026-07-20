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
	file, err := os.Open("../../testdata/_6160345-.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	result, err := metadata.ExtractRaster(file)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if result.Width == nil || *result.Width != 2160 || result.Height == nil || *result.Height != 1620 {
		t.Errorf("dimensions = %vx%v, want 2160x1620", deref(result.Width), deref(result.Height))
	}
	if result.Creator == nil || *result.Creator != "Ari Madian" {
		t.Errorf("creator = %v, want Ari Madian", deref(result.Creator))
	}
	if result.Copyright == nil || *result.Copyright != "ALL RIGHTS RESERVED | Ari Madian" {
		t.Errorf("copyright = %v", deref(result.Copyright))
	}
	// ColorSpace short 1 → "sRGB" (exifColorSpace).
	if result.ColorSpace == nil || *result.ColorSpace != "sRGB" {
		t.Errorf("ColorSpace = %v, want sRGB", deref(result.ColorSpace))
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
// JPEG. testdata/exif-original.JPG is a FUJIFILM X-T5 frame; these are its actual
// recorded values, so a regression in any coercion breaks exactly one assertion.
// The fixture carries no GPS, so the DMS→decimal path is covered separately in
// TestExifGPS_DMSToDecimal.
func TestExtract_FullEXIF(t *testing.T) {
	// The path case must match the committed fixture exactly: a mismatch skips
	// silently on macOS but fails on case-sensitive CI — hence Fatal, not Skip.
	const path = "../../testdata/exif-original.JPG"
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("full-EXIF fixture missing at %s: %v", path, err)
	}
	defer file.Close()

	result, err := metadata.ExtractRaster(file)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	if result.CameraMake == nil || *result.CameraMake != "FUJIFILM" {
		t.Errorf("CameraMake = %v, want FUJIFILM", deref(result.CameraMake))
	}
	if result.CameraModel == nil || *result.CameraModel != "X-T5" {
		t.Errorf("CameraModel = %v, want X-T5", deref(result.CameraModel))
	}
	if result.LensModel == nil || *result.LensModel != "XF55-200mmF3.5-4.8 R LM OIS" {
		t.Errorf("LensModel = %v, want XF55-200mmF3.5-4.8 R LM OIS", deref(result.LensModel))
	}
	// FNumber rational 11/1 → 11.0 (exifRatFloat).
	if result.Aperture == nil || *result.Aperture != 11 {
		t.Errorf("Aperture = %v, want 11", deref(result.Aperture))
	}
	// FocalLength rational 200/1 → 200.0 (exifRatFloat).
	if result.FocalLengthMM == nil || *result.FocalLengthMM != 200 {
		t.Errorf("FocalLengthMM = %v, want 200", deref(result.FocalLengthMM))
	}
	// ExposureTime 1/3200 (<1s) → "1/3200" (exifShutter fast-branch).
	if result.ShutterSpeed == nil || *result.ShutterSpeed != "1/3200" {
		t.Errorf("ShutterSpeed = %v, want 1/3200", deref(result.ShutterSpeed))
	}
	// ISO coerced to int (exifInt).
	if result.ISO == nil || *result.ISO != 1600 {
		t.Errorf("ISO = %v, want 1600", deref(result.ISO))
	}
	// DateTimeOriginal parsed as UTC-labelled wall-clock (exifTime).
	if result.CapturedAt == nil || !result.CapturedAt.Equal(time.Date(2026, 5, 20, 15, 59, 30, 0, time.UTC)) {
		t.Errorf("CapturedAt = %v, want 2026-05-20 15:59:30 UTC", deref(result.CapturedAt))
	}
	// ColorSpace 65535 + InteroperabilityIndex R03 → "Adobe RGB" (exifColorSpace).
	if result.ColorSpace == nil || *result.ColorSpace != "Adobe RGB" {
		t.Errorf("ColorSpace = %v, want Adobe RGB", deref(result.ColorSpace))
	}
}
