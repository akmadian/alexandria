// Command mockthumbs generates a small pool of realistically-weighted 512px JPEG
// tiles for frontend dev (frontend/public/mock-thumbs). The frontend cycles the
// pool with a per-asset cache-bust query so scrolling triggers real JPEG decodes
// at realistic byte weight — SVG placeholders undercount decode/memory cost.
//
// Regenerate with: go run ./cmd/mockthumbs
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"os"
	"path/filepath"
)

const (
	count   = 20
	size    = 512
	quality = 78
	noise   = 0.05 // fine per-pixel jitter; tune with quality to hit ~27kb
	grid    = 24   // coarse color grid, bilinearly upscaled into smooth blobs
	outDir  = "frontend/public/mock-thumbs"
)

func main() {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}
	for i := range count {
		img := generate(rand.New(rand.NewSource(int64(i) + 1)))
		path := filepath.Join(outDir, fmt.Sprintf("%02d.jpg", i))
		f, err := os.Create(path)
		if err != nil {
			panic(err)
		}
		if err := jpeg.Encode(f, img, &jpeg.Options{Quality: quality}); err != nil {
			panic(err)
		}
		f.Close()
		info, _ := os.Stat(path)
		fmt.Printf("%s  %d bytes\n", path, info.Size())
	}
}

// generate builds a photographic-ish tile: a coarse random color grid bilinearly
// upscaled into smooth blobs, plus fine per-pixel noise for JPEG-realistic
// entropy (so it compresses like a real thumbnail, not a flat gradient).
func generate(r *rand.Rand) image.Image {
	g := make([][3]float64, (grid+1)*(grid+1))
	for j := range g {
		g[j] = [3]float64{r.Float64(), r.Float64(), r.Float64()}
	}
	at := func(gx, gy int) [3]float64 { return g[gy*(grid+1)+gx] }

	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cell := float64(size) / float64(grid)
	for y := range size {
		for x := range size {
			fx, fy := float64(x)/cell, float64(y)/cell
			gx, gy := int(fx), int(fy)
			tx, ty := fx-float64(gx), fy-float64(gy)
			c00, c10 := at(gx, gy), at(gx+1, gy)
			c01, c11 := at(gx, gy+1), at(gx+1, gy+1)
			var rgb [3]uint8
			for k := range 3 {
				top := c00[k]*(1-tx) + c10[k]*tx
				bot := c01[k]*(1-tx) + c11[k]*tx
				v := top*(1-ty) + bot*ty
				v += (r.Float64() - 0.5) * noise // fine noise → entropy
				rgb[k] = uint8(clamp01(v) * 255)
			}
			img.Set(x, y, color.RGBA{rgb[0], rgb[1], rgb[2], 255})
		}
	}
	return img
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
