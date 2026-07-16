package signals

import (
	"image"
	"image/color"
	"testing"
)

// grayImage builds a width×height image whose luma at each pixel comes from lum.
func grayImage(width, height int, lum func(column, row int) uint8) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for row := 0; row < height; row++ {
		for column := 0; column < width; column++ {
			value := lum(column, row)
			img.SetRGBA(column, row, color.RGBA{R: value, G: value, B: value, A: 255})
		}
	}
	return img
}

// TestSharpness_OrdersByDetail is the golden ordering: a 1px checkerboard (an edge
// at every pixel) must score above an 8px-block checkerboard (edges only at block
// seams), which must score above a flat field (no edges → ~zero variance). This is
// the "sharp beats blurred" contract expressed as high-frequency vs low-frequency
// content; absolute values are deliberately not asserted (ranking is the contract).
func TestSharpness_OrdersByDetail(t *testing.T) {
	const size = 64
	fine := grayImage(size, size, func(column, row int) uint8 {
		if (column+row)%2 == 0 {
			return 255
		}
		return 0
	})
	coarse := grayImage(size, size, func(column, row int) uint8 {
		if ((column/8)+(row/8))%2 == 0 {
			return 255
		}
		return 0
	})
	flat := grayImage(size, size, func(_, _ int) uint8 { return 128 })

	fineScore, coarseScore, flatScore := Sharpness(fine), Sharpness(coarse), Sharpness(flat)
	if !(fineScore > coarseScore && coarseScore > flatScore) {
		t.Fatalf("want fine > coarse > flat, got %.2f, %.2f, %.2f", fineScore, coarseScore, flatScore)
	}
	if flatScore > 1e-6 {
		t.Fatalf("flat field must have ~zero sharpness, got %.6f", flatScore)
	}
}

// TestClipping_ExtremesAndNeutral pins the histogram extremes: all-white is fully
// blown highlights, all-black fully crushed shadows, mid-gray neither.
func TestClipping_ExtremesAndNeutral(t *testing.T) {
	const size = 32
	white := grayImage(size, size, func(_, _ int) uint8 { return 255 })
	black := grayImage(size, size, func(_, _ int) uint8 { return 0 })
	mid := grayImage(size, size, func(_, _ int) uint8 { return 128 })

	whiteHigh, whiteLow := Clipping(white)
	if whiteHigh < 99 || whiteLow > 1 {
		t.Fatalf("all-white: highlights=%.1f shadows=%.1f, want ~100/0", whiteHigh, whiteLow)
	}
	blackHigh, blackLow := Clipping(black)
	if blackLow < 99 || blackHigh > 1 {
		t.Fatalf("all-black: highlights=%.1f shadows=%.1f, want ~0/100", blackHigh, blackLow)
	}
	midHigh, midLow := Clipping(mid)
	if midHigh > 1 || midLow > 1 {
		t.Fatalf("mid-gray: highlights=%.1f shadows=%.1f, want ~0/0", midHigh, midLow)
	}
}

// TestPerceptualHash_NearDuplicateVsDistinct is the golden near-dup check: the same
// texture under a global brightness lift stays within a small hamming distance
// (dHash keys on relative gradients, which brightness preserves), while a
// structurally different image lands farther away.
func TestPerceptualHash_NearDuplicateVsDistinct(t *testing.T) {
	const size = 64
	texture := func(column, row int) uint8 { return uint8(((column * 4) ^ (row * 4)) & 0xff) }
	base := grayImage(size, size, texture)
	brighter := grayImage(size, size, func(column, row int) uint8 {
		value := int(texture(column, row)) + 20
		if value > 255 {
			value = 255
		}
		return uint8(value)
	})
	distinct := grayImage(size, size, func(column, row int) uint8 {
		return uint8(((column * 7) + (row * 3)) & 0xff)
	})

	near := Hamming(DHash(base), DHash(brighter))
	far := Hamming(DHash(base), DHash(distinct))
	if near > 8 {
		t.Fatalf("near-duplicate hamming=%d, want ≤8", near)
	}
	if far <= near {
		t.Fatalf("distinct hamming=%d must exceed near-dup hamming=%d", far, near)
	}
}

// TestSignals_DegenerateInputs covers the guard branches: an image too small to
// convolve a 3×3 kernel yields zero sharpness, and a zero-pixel image yields zero
// clipping (rather than dividing by zero).
func TestSignals_DegenerateInputs(t *testing.T) {
	tiny := grayImage(2, 2, func(_, _ int) uint8 { return 200 })
	if score := Sharpness(tiny); score != 0 {
		t.Fatalf("sub-3px image sharpness = %v, want 0", score)
	}
	empty := image.NewRGBA(image.Rect(0, 0, 0, 0))
	if high, low := Clipping(empty); high != 0 || low != 0 {
		t.Fatalf("empty image clipping = %v/%v, want 0/0", high, low)
	}
}

func TestHash_HexRoundTrip(t *testing.T) {
	const original = uint64(0xdeadbeefcafef00d)
	parsed, err := ParseHash(FormatHash(original))
	if err != nil || parsed != original {
		t.Fatalf("round-trip: got %x err=%v, want %x", parsed, err, original)
	}
}
