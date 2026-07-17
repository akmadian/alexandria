package thumbnailer_test

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akmadian/alexandria/internal/dependency"
	"github.com/akmadian/alexandria/internal/thumbnailer"
	"github.com/charmbracelet/log"
)

// pngFile writes a solid-color PNG of the given size and returns its path.
func pngFile(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "source.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func dims(t *testing.T, path string) (int, int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatalf("decode thumb: %v", err)
	}
	return cfg.Width, cfg.Height
}

func TestGenerateRaster_DownscalesPreservingAspect(t *testing.T) {
	thumb := thumbnailer.New(t.TempDir())
	if err := thumbnailer.GenerateRaster(thumb, context.Background(), pngFile(t, 1000, 800), "ab1234"); err != nil {
		t.Fatalf("generate: %v", err)
	}
	w, h := dims(t, thumb.Path("ab1234", 512))
	if w != 512 || h != 410 { // long edge → 512, 800*512/1000 = 409.6 → 410
		t.Errorf("thumb = %dx%d, want 512x410", w, h)
	}
}

func TestGenerateRaster_DoesNotUpscale(t *testing.T) {
	thumb := thumbnailer.New(t.TempDir())
	if err := thumbnailer.GenerateRaster(thumb, context.Background(), pngFile(t, 100, 80), "cd5678"); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if w, h := dims(t, thumb.Path("cd5678", 512)); w != 100 || h != 80 {
		t.Errorf("thumb = %dx%d, want 100x80 (no upscale)", w, h)
	}
}

func TestGenerateRaster_MissingSourceErrors(t *testing.T) {
	thumb := thumbnailer.New(t.TempDir())
	err := thumbnailer.GenerateRaster(thumb, context.Background(), filepath.Join(t.TempDir(), "gone.png"), "ef9012")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want fs.ErrNotExist in chain, got %v", err)
	}
	if _, err := os.Stat(thumb.Path("ef9012", 512)); !os.IsNotExist(err) {
		t.Error("a failed generate should not write a file")
	}
}

// TestGenerateRawPreview_NoDaemonFailsDistinctly pins the degradation contract:
// the capability exists in the registry, the tool is missing at runtime — the
// strategy must fail with ErrExiftoolUnavailable (→ the tool_unavailable DLQ
// reason), never silently skip.
func TestGenerateRawPreview_NoDaemonFailsDistinctly(t *testing.T) {
	thumb := thumbnailer.New(t.TempDir()) // Exiftool nil
	err := thumbnailer.GenerateRawPreview(thumb, context.Background(), "irrelevant.cr2", "ab9999")
	if !errors.Is(err, thumbnailer.ErrExiftoolUnavailable) {
		t.Fatalf("want ErrExiftoolUnavailable, got %v", err)
	}
}

// TestGenerateRawPreview_ExtractsEmbeddedPreview drives the REAL preview path
// against a live exiftool daemon (skipped when exiftool isn't installed): the
// full tag-fallback ladder (PreviewImage and JpgFromRaw come back empty for a
// plain JPEG, ThumbnailImage hits), the binary `-b` round-trip over the
// stay_open pipe — the exact traffic the suffix-matched ready marker exists
// for — and the resize of the extracted bytes. The output dimensions prove the
// EMBEDDED image was thumbnailed, not the host file: the host is 640×480
// (would yield 512×384), the embedded preview is 64×48 (under 512, never
// upscaled — so 64×48 out means the preview bytes went through).
func TestGenerateRawPreview_ExtractsEmbeddedPreview(t *testing.T) {
	status := dependency.Discover(dependency.Exiftool, "")
	if !status.Available() {
		t.Skipf("exiftool not available (%s) — skipping the live preview-extraction path", status.State)
	}
	daemon, err := dependency.StartExiftool(status, log.New(io.Discard))
	if err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer daemon.Close()

	sourceDir := t.TempDir()
	hostPath := filepath.Join(sourceDir, "host.jpg")
	previewPath := pngToJPEG(t, pngFile(t, 64, 48))
	if err := os.Rename(pngToJPEG(t, pngFile(t, 640, 480)), hostPath); err != nil {
		t.Fatal(err)
	}
	if _, err := daemon.Execute(context.Background(), "-overwrite_original", "-ThumbnailImage<="+previewPath, hostPath); err != nil {
		t.Fatalf("embed preview: %v", err)
	}

	thumb := thumbnailer.New(t.TempDir())
	thumb.Exiftool = daemon
	if err := thumbnailer.GenerateRawPreview(thumb, context.Background(), hostPath, "ab4242"); err != nil {
		t.Fatalf("GenerateRawPreview: %v", err)
	}
	if w, h := dims(t, thumb.Path("ab4242", 512)); w != 64 || h != 48 {
		t.Fatalf("thumb = %dx%d, want 64x48 (the EMBEDDED preview's dims — 512x384 would mean the host was decoded instead)", w, h)
	}
}

// pngToJPEG re-encodes a PNG file as a JPEG beside it (exiftool's
// ThumbnailImage must be JPEG bytes) and returns the new path.
func pngToJPEG(t *testing.T, pngPath string) string {
	t.Helper()
	pngHandle, err := os.Open(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	defer pngHandle.Close()
	decoded, err := png.Decode(pngHandle)
	if err != nil {
		t.Fatal(err)
	}
	jpegPath := strings.TrimSuffix(pngPath, ".png") + ".jpg"
	jpegHandle, err := os.Create(jpegPath)
	if err != nil {
		t.Fatal(err)
	}
	defer jpegHandle.Close()
	if err := jpeg.Encode(jpegHandle, decoded, nil); err != nil {
		t.Fatal(err)
	}
	return jpegPath
}

func TestPath_ShardsByPrefix(t *testing.T) {
	thumb := thumbnailer.New("/data")
	if got, want := thumb.Path("ab1234", 512), "/data/512/ab/ab1234.jpg"; got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

// TestAnalysisSize_PicksSmallestTier pins the signal-input contract (task 20):
// the analysis size is the smallest generated tier regardless of Sizes order,
// so adding a larger tier never silently changes what the signals read.
func TestAnalysisSize_PicksSmallestTier(t *testing.T) {
	thumb := &thumbnailer.Thumbnailer{Sizes: []int{1024, 256, 512}}
	if got := thumb.AnalysisSize(); got != 256 {
		t.Errorf("AnalysisSize() = %d, want 256", got)
	}
}
