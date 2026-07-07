package thumbnailer

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math"
	"os"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/charmbracelet/log"
	"golang.org/x/image/draw"
)

// GenerateImage decodes a raster image once, then writes one JPEG per size, each
// scaled to fit that long edge. It is a thumbnailer.GenFunc.
func GenerateImage(r io.ReadSeeker, sizes []int, quality int, dst func(int) string) error {
	src, _, err := image.Decode(r)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	for _, size := range sizes {
		log.Debug("generating thumbnail", "size", size)
		if err := encodeJPEG(dst(size), fit(src, size), quality); err != nil {
			return err
		}
	}
	return nil
}

func encodeJPEG(path string, img image.Image, quality int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = jpeg.Encode(f, img, &jpeg.Options{Quality: quality})
	if cerr := f.Close(); err == nil {
		err = cerr // surface flush errors that only appear on Close
	}
	return err
}

// fit scales src so its long edge is at most long, preserving aspect ratio, and
// composites onto white. Small images are not upscaled. The white background
// flattens alpha (PNG/GIF transparency) since JPEG has no alpha channel.
// ponytail: white fill; revisit if transparent-thumb previews are ever wanted.
func fit(src image.Image, long int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	tw, th := w, h
	if w > long || h > long { // downscale only
		if w >= h {
			tw, th = long, round(h*long, w)
		} else {
			tw, th = round(w*long, h), long
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	if tw == w && th == h {
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Over) // 1:1, no resample
	} else {
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	}
	return dst
}

// round computes a*/b rounded to nearest, for aspect-preserving dimensions.
func round(a, b int) int { return int(math.Round(float64(a) / float64(b))) }
