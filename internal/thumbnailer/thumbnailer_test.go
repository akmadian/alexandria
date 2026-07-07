package thumbnailer_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/akmadian/alexandria/internal/thumbnailer"
)

// pngBytes returns a solid-color PNG of the given size as a seekable reader.
func pngReader(t *testing.T, w, h int) *bytes.Reader {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(buf.Bytes())
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

func TestGenerate_DownscalesPreservingAspect(t *testing.T) {
	reg := thumbnailer.New(t.TempDir())
	if ok, err := reg.Generate(thumbnailer.GenerateRaster, pngReader(t, 1000, 800), "ab1234"); err != nil || !ok {
		t.Fatalf("generate: ok=%v err=%v", ok, err)
	}
	w, h := dims(t, reg.Path("ab1234", 512))
	if w != 512 || h != 410 { // long edge → 512, 800*512/1000 = 409.6 → 410
		t.Errorf("thumb = %dx%d, want 512x410", w, h)
	}
}

func TestGenerate_DoesNotUpscale(t *testing.T) {
	reg := thumbnailer.New(t.TempDir())
	if ok, err := reg.Generate(thumbnailer.GenerateRaster, pngReader(t, 100, 80), "cd5678"); err != nil || !ok {
		t.Fatalf("generate: ok=%v err=%v", ok, err)
	}
	if w, h := dims(t, reg.Path("cd5678", 512)); w != 100 || h != 80 {
		t.Errorf("thumb = %dx%d, want 100x80 (no upscale)", w, h)
	}
}

func TestGenerate_NilGenIsNoOp(t *testing.T) {
	reg := thumbnailer.New(t.TempDir())
	ok, err := reg.Generate(nil, pngReader(t, 100, 100), "ef9012")
	if err != nil || ok {
		t.Fatalf("nil generator should be a no-op, got ok=%v err=%v", ok, err)
	}
	if _, err := os.Stat(reg.Path("ef9012", 512)); !os.IsNotExist(err) {
		t.Error("no-op should not write a file")
	}
}

func TestPath_ShardsByPrefix(t *testing.T) {
	reg := thumbnailer.New("/data")
	if got, want := reg.Path("ab1234", 512), "/data/512/ab/ab1234.jpg"; got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}
