package thumbnailer

import (
	"context"
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

	"golang.org/x/image/draw"
)

// resizeKernel is the interpolation used to downscale into a thumbnail. The
// x/image/draw interpolators, ordered fast → sharp: NearestNeighbor,
// ApproxBiLinear, BiLinear, CatmullRom. The resize dominates thumbnail cost, and
// for a 512px thumbnail from a multi-megapixel source ApproxBiLinear is several
// times faster than CatmullRom at a quality difference that's negligible at
// thumbnail scale — so it's the default here. The full quality ladder is a
// one-word change: dial up to draw.BiLinear or draw.CatmullRom if large-downscale
// aliasing on very textured images ever shows. The real fix for huge sources is
// decode-downscale (DCT / subprocess), which lands with full RAW decode support;
// this kernel choice is orthogonal to that.
var resizeKernel draw.Interpolator = draw.ApproxBiLinear

// generateRaster is the GenerateRaster strategy: open the source file, decode it
// with a standard decoder, and write one JPEG per configured size.
//
// "Raster" here means the formats a standard decoder can turn straight into
// pixels — JPEG/PNG/GIF via Go's stdlib today. It is deliberately NOT the RAW
// path: GenerateRawPreview extracts a RAW file's embedded JPEG preview and feeds
// those bytes through the same resizeAndEncode backend, so RAW thumbnailing
// reuses the raster backend instead of duplicating resize/encode.
func (thumb *Thumbnailer) generateRaster(_ context.Context, sourcePath string, assetID string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = sourceFile.Close() }()
	return thumb.resizeAndEncode(sourceFile, assetID)
}

// resizeAndEncode is the shared backend both strategies funnel into: decode a
// stream of pixels once, write one JPEG per configured size, each scaled to fit
// that long edge.
func (thumb *Thumbnailer) resizeAndEncode(reader io.Reader, assetID string) error {
	src, _, err := image.Decode(reader)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if err := thumb.ensureDirs(assetID); err != nil {
		return err
	}
	for _, size := range thumb.Sizes {
		if err := encodeJPEG(thumb.Path(assetID, size), fit(src, size), thumb.Quality); err != nil {
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
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	targetWidth, targetHeight := width, height
	if width > long || height > long { // downscale only
		// max(_, 1): an extreme aspect ratio (>~1024:1) rounds the short edge to 0,
		// and jpeg.Encode accepts a zero-dimension image silently — a "successful"
		// but undecodable thumbnail that later fails/degrades every signal job.
		if width >= height {
			targetWidth, targetHeight = long, max(round(height*long, width), 1)
		} else {
			targetWidth, targetHeight = max(round(width*long, height), 1), long
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	if targetWidth == width && targetHeight == height {
		draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Over) // 1:1, no resample
	} else {
		resizeKernel.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	}
	return dst
}

// round computes a*long/b rounded to nearest, for aspect-preserving dimensions.
func round(a, b int) int { return int(math.Round(float64(a) / float64(b))) }
