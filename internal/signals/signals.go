// Package signals computes the cheap culling signals (task 20, D28) from a
// decoded thumbnail image: sharpness (variance of Laplacian), highlight/shadow
// clipping (luma histogram extremes), and a perceptual hash for near-duplicate
// detection. Every function is a pure transform over an image.Image — the
// enrichment producers in internal/enrichment own reading the 512px thumbnail
// off disk and committing the result; this package owns only the math, so the
// algorithms stay legible and golden-testable in isolation.
//
// The signals read the thumbnail artifact, never the original bytes (D28: the
// signal's prerequisite is the thumbnail on disk — re-decoding a giant RAW/TIFF
// is exactly the cost the enrichment engine exists to avoid). At the fixed
// thumbnail size the absolute scales are comparable across images. Changing which
// tier signals read is a deliberate rebuild — clear the signal columns and the
// scan re-derives them — NOT something the engine auto-detects, so a tier change
// must be driven as a migration.
//
// ponytail: hand-rolled pure-Go pixel math (~1 kernel + 1 histogram + 1 hash),
// stdlib plus the x/image/draw we already depend on. No imaging library: gocv is
// cgo + a system OpenCV install (banned here), and the pure-Go options (bild,
// imaging) are not load-bearing for this little math. Revisit bild as a shared
// foundation if focus-peaking (FR, Loupe) plus more classical-CV signals pile up.
package signals

import (
	"fmt"
	"image"
	"math/bits"
	"strconv"

	"golang.org/x/image/draw"
)

// grayField extracts the Rec.601 luma of every pixel as a row-major float64 grid
// scaled to 0..255. Float (not 8-bit) intermediate storage keeps the Laplacian
// variance free of quantization noise.
func grayField(img image.Image) (values []float64, width, height int) {
	bounds := img.Bounds()
	width, height = bounds.Dx(), bounds.Dy()
	values = make([]float64, width*height)
	index := 0
	for pixelY := bounds.Min.Y; pixelY < bounds.Max.Y; pixelY++ {
		for pixelX := bounds.Min.X; pixelX < bounds.Max.X; pixelX++ {
			// RGBA returns 16-bit channels (0..65535); /257 maps that back to 0..255.
			red, green, blue, _ := img.At(pixelX, pixelY).RGBA()
			values[index] = (0.299*float64(red) + 0.587*float64(green) + 0.114*float64(blue)) / 257.0
			index++
		}
	}
	return values, width, height
}

// Sharpness returns the variance of the Laplacian of the image's luma — the
// standard focus/blur measure. Crisp edges spread the Laplacian response wide
// (high variance); blur collapses it (low variance). The value is RAW and
// unnormalized: only relative ordering is meaningful (the epic's contract is
// "ranking, not absolute values"), and at a fixed thumbnail size the scale is
// comparable across images.
//
// ponytail: raw variance. Two documented upgrades, each local to this function
// with no caller impact, neither with a consumer yet: (1) contrast-normalize as
// var/mean² if cross-scene comparison ever needs to divide out that the Laplacian
// also measures contrast; (2) a light Gaussian pre-blur if q80 JPEG block
// artifacts inflate the score on the real library.
func Sharpness(img image.Image) float64 {
	gray, width, height := grayField(img)
	if width < 3 || height < 3 {
		return 0 // too small to convolve a 3×3 kernel
	}
	// 4-connected Laplacian kernel [[0,1,0],[1,-4,1],[0,1,0]] over interior pixels
	// (the 1px border is skipped rather than edge-padded).
	var sum, sumSquares float64
	count := 0
	for row := 1; row < height-1; row++ {
		for column := 1; column < width-1; column++ {
			center := gray[row*width+column]
			laplacian := gray[(row-1)*width+column] + gray[(row+1)*width+column] +
				gray[row*width+column-1] + gray[row*width+column+1] - 4*center
			sum += laplacian
			sumSquares += laplacian * laplacian
			count++
		}
	}
	mean := sum / float64(count)
	return sumSquares/float64(count) - mean*mean
}

const (
	// highlightFloor / shadowCeil are the luma cutoffs (0..255) for "blown" and
	// "crushed" pixels — near-max/near-min rather than exactly 255/0 so a hair of
	// JPEG noise on a truly clipped pixel still counts.
	highlightFloor = 250.0
	shadowCeil     = 5.0
)

// Clipping returns the percentage of pixels that are blown-out highlights
// (luma ≥ highlightFloor) and crushed shadows (luma ≤ shadowCeil), each in 0..100.
// Both fall out of one histogram pass — one job kind, two columns (D28). Luma is
// computed inline rather than via grayField: this is a single linear pass with no
// random access, so materializing a width*height float64 buffer (~2 MB at the
// analysis tier) would be pure allocation churn (Sharpness needs the buffer for
// its neighbour reads; this does not).
func Clipping(img image.Image) (highlights, shadows float64) {
	bounds := img.Bounds()
	total := bounds.Dx() * bounds.Dy()
	if total == 0 {
		return 0, 0
	}
	highCount, lowCount := 0, 0
	for pixelY := bounds.Min.Y; pixelY < bounds.Max.Y; pixelY++ {
		for pixelX := bounds.Min.X; pixelX < bounds.Max.X; pixelX++ {
			// Same Rec.601 luma as grayField: RGBA is 16-bit (0..65535); /257 maps to 0..255.
			red, green, blue, _ := img.At(pixelX, pixelY).RGBA()
			luma := (0.299*float64(red) + 0.587*float64(green) + 0.114*float64(blue)) / 257.0
			switch {
			case luma >= highlightFloor:
				highCount++
			case luma <= shadowCeil:
				lowCount++
			}
		}
	}
	return 100 * float64(highCount) / float64(total), 100 * float64(lowCount) / float64(total)
}

// PerceptualHasher computes a 64-bit perceptual hash of an image. It is a
// strategy value: swapping the algorithm (dHash → DCT pHash, aHash, …) is
// reassigning DefaultHasher, never touching the enrichment producer or the
// registry row (the modularity requirement, decision 2).
type PerceptualHasher func(image.Image) uint64

// DefaultHasher is the perceptual-hash strategy the phash enrichment kind uses.
// dHash (difference hash) is pure Go, needs no DCT, and is robust for the
// near-duplicate ranking phash exists to serve. Reach for a DCT-based pHash
// (a dependency, e.g. goimagehash) only if near-dup recall on the real library
// proves insufficient — a one-line swap here.
var DefaultHasher PerceptualHasher = DHash

// DHash computes the 8×8 difference hash: downscale to 9×8 luma, then for each
// row emit one bit per adjacent-pixel comparison (left brighter than right).
// 8 rows × 8 comparisons = 64 bits. Robust to scale, aspect, brightness, and
// mild edits — the properties near-duplicate detection needs.
func DHash(img image.Image) uint64 {
	const columns, rows = 9, 8
	small := image.NewRGBA(image.Rect(0, 0, columns, rows))
	draw.ApproxBiLinear.Scale(small, small.Bounds(), img, img.Bounds(), draw.Over, nil)
	gray, _, _ := grayField(small)
	var hash uint64
	position := 0
	for row := 0; row < rows; row++ {
		for column := 0; column < columns-1; column++ {
			if gray[row*columns+column] > gray[row*columns+column+1] {
				hash |= 1 << uint(position)
			}
			position++
		}
	}
	return hash
}

// FormatHash renders a hash as the 16-hex-digit form stored in the phash column.
func FormatHash(hash uint64) string { return fmt.Sprintf("%016x", hash) }

// ParseHash reads the stored hex form back to a hash.
func ParseHash(text string) (uint64, error) { return strconv.ParseUint(text, 16, 64) }

// Hamming is the bit distance between two hashes — the near-duplicate metric
// (0 = identical, 64 = opposite). Used by the golden tests now; the query
// surface that would expose it to users is deferred with phash (DEFERRED §12).
func Hamming(left, right uint64) int { return bits.OnesCount64(left ^ right) }
