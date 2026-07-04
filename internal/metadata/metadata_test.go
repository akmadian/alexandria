package metadata_test

import (
	"os"
	"strings"
	"testing"

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

	md, err := metadata.Default().Extract(f, "image/jpeg")
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
	// Garbage bytes for a registered type → best-effort, no panic, no dimensions.
	md, err := metadata.Default().Extract(strings.NewReader("not a real jpeg"), "image/jpeg")
	if err != nil {
		t.Fatalf("garbage jpeg: %v", err)
	}
	if md.Width != nil {
		t.Error("garbage input should yield no dimensions")
	}
	// Unregistered MIME → zero metadata.
	md, err = metadata.Default().Extract(strings.NewReader("x"), "application/pdf")
	if err != nil {
		t.Fatalf("unknown mime: %v", err)
	}
	if md.Width != nil || md.Creator != nil {
		t.Error("unknown mime should yield empty metadata")
	}
}

// Validates the camera/exposure/GPS mapping (rationals, DMS→decimal) against real
// data. Skipped until an original camera JPEG with full EXIF is dropped at
// testdata/exif-original.jpg — the committed fixtures are stripped exports, so
// this mapping is otherwise unexercised.
func TestExtract_FullEXIF(t *testing.T) {
	const path = "../../testdata/exif-original.jpg"
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("no full-EXIF fixture at %s (drop an original camera JPEG there to enable): %v", path, err)
	}
	defer f.Close()

	md, err := metadata.Default().Extract(f, "image/jpeg")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	t.Logf("Make=%v Model=%v Lens=%v Captured=%v F=%v Focal=%v Shutter=%v ISO=%v GPS=%v,%v",
		deref(md.CameraMake), deref(md.CameraModel), deref(md.LensModel), deref(md.CapturedAt),
		deref(md.Aperture), deref(md.FocalLengthMM), deref(md.ShutterSpeed), deref(md.ISO),
		deref(md.GPSLat), deref(md.GPSLon))
	if md.CameraMake == nil {
		t.Error("expected CameraMake from a full-EXIF original")
	}
	if md.CapturedAt == nil {
		t.Error("expected CapturedAt from a full-EXIF original")
	}
	if md.Aperture == nil {
		t.Error("expected Aperture from a full-EXIF original")
	}
}
