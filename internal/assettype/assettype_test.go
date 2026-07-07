package assettype_test

import (
	"testing"

	"github.com/akmadian/alexandria/internal/assettype"
	"github.com/akmadian/alexandria/internal/domain"
)

func TestClassify(t *testing.T) {
	// Case-insensitive and dot-tolerant; capability funcs wired for raster.
	h, ok := assettype.Classify("JPG")
	if !ok || h.Type != domain.FileTypeImage || h.MIME != "image/jpeg" {
		t.Fatalf("JPG: %+v ok=%v", h, ok)
	}
	if h.Metadata == nil || h.Thumb == nil {
		t.Fatal("jpg should carry metadata + thumbnail capability funcs")
	}

	h, ok = assettype.Classify(".cr2")
	if !ok || h.Type != domain.FileTypeRaw {
		t.Fatalf("cr2: %+v ok=%v", h, ok)
	}

	if _, ok := assettype.Classify("exe"); ok {
		t.Fatal("exe should be unsupported")
	}
}

// A supported type with no capabilities is handled gracefully: it classifies, and
// its nil funcs signal "skip" to the caller (no extractor/thumbnailer to call).
// This is the "add a format = one row" contract: rows without funcs just work.
func TestClassify_NilCapabilityDegrades(t *testing.T) {
	h, ok := assettype.Classify("pdf")
	if !ok {
		t.Fatal("pdf should be a supported type")
	}
	if h.Type != domain.FileTypeDocument {
		t.Fatalf("pdf type = %q", h.Type)
	}
	if h.Metadata != nil || h.Thumb != nil {
		t.Fatal("pdf has no extractor/generator yet — funcs must be nil (graceful skip)")
	}
}

func TestIsSupported(t *testing.T) {
	if !assettype.IsSupported("png") {
		t.Fatal("png should be supported")
	}
	if assettype.IsSupported("xyz") {
		t.Fatal("xyz should not be supported")
	}
}

func TestIsSidecar(t *testing.T) {
	for _, ext := range []string{"xmp", ".XMP", "aae", "thm", "lrv"} {
		if !assettype.IsSidecar(ext) {
			t.Errorf("%q should be a sidecar", ext)
		}
	}
	if assettype.IsSidecar("jpg") {
		t.Fatal("jpg is an asset, not a sidecar")
	}
}

func TestSniff_GoldenHeaders(t *testing.T) {
	cases := []struct {
		name string
		head []byte
		want assettype.ContentFamily
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, assettype.FamilyJPEG},
		{"png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, assettype.FamilyPNG},
		{"gif", []byte("GIF89a"), assettype.FamilyGIF},
		{"tiff-le", []byte{'I', 'I', 0x2A, 0x00, 0x08}, assettype.FamilyTIFF},
		{"tiff-be", []byte{'M', 'M', 0x00, 0x2A, 0x00}, assettype.FamilyTIFF},
		{"bmp", []byte("BM....."), assettype.FamilyBMP},
		{"pdf", []byte("%PDF-1.7"), assettype.FamilyPDF},
		{"psd", []byte("8BPS...."), assettype.FamilyPSD},
		{"flac", []byte("fLaC...."), assettype.FamilyFLAC},
		{"mkv", []byte{0x1A, 0x45, 0xDF, 0xA3, 0x01}, assettype.FamilyMatroska},
		{"mp3-id3", []byte("ID3\x03\x00"), assettype.FamilyMP3},
		{"mp3-sync", []byte{0xFF, 0xFB, 0x90, 0x00}, assettype.FamilyMP3},
		{"eps", []byte("%!PS-Adobe"), assettype.FamilyPostScript},
		{"svg", []byte(`<?xml version="1.0"?><svg`), assettype.FamilyXML},
		{"svg-bare", []byte(`<svg xmlns="...">`), assettype.FamilyXML},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := assettype.Sniff(c.head)
			if !ok || got != c.want {
				t.Fatalf("Sniff(%s) = %q,%v want %q", c.name, got, ok, c.want)
			}
		})
	}
}

// RIFF is a container: WEBP and WAV share the first 4 bytes, and a RIFF with an
// unknown form type must not be misclassified.
func TestSniff_RIFFDisambiguation(t *testing.T) {
	webp := append([]byte("RIFF"), append([]byte{0, 0, 0, 0}, "WEBP"...)...)
	if got, ok := assettype.Sniff(webp); !ok || got != assettype.FamilyWebP {
		t.Fatalf("webp: got %q,%v", got, ok)
	}
	wav := append([]byte("RIFF"), append([]byte{0, 0, 0, 0}, "WAVE"...)...)
	if got, ok := assettype.Sniff(wav); !ok || got != assettype.FamilyWAV {
		t.Fatalf("wav: got %q,%v", got, ok)
	}
	other := append([]byte("RIFF"), append([]byte{0, 0, 0, 0}, "AVI "...)...)
	if _, ok := assettype.Sniff(other); ok {
		t.Fatal("unknown RIFF form must not be recognized as webp/wav")
	}
}

// The ftyp box marks the whole ISO-BMFF family; the brand (mp42 vs qt) is a
// dialect the extension resolves, not the sniffer.
func TestSniff_ISOBMFFFamilyRegardlessOfBrand(t *testing.T) {
	mp4 := append([]byte{0, 0, 0, 0x18}, append([]byte("ftyp"), "mp42"...)...)
	mov := append([]byte{0, 0, 0, 0x14}, append([]byte("ftyp"), "qt  "...)...)
	for name, head := range map[string][]byte{"mp4": mp4, "mov": mov} {
		if got, ok := assettype.Sniff(head); !ok || got != assettype.FamilyISOBMFF {
			t.Fatalf("%s: got %q,%v want isobmff", name, got, ok)
		}
	}
}

func TestSniff_TruncatedAndEmpty(t *testing.T) {
	for _, head := range [][]byte{nil, {}, {0xFF}, {'R', 'I', 'F'}, []byte("garbage-no-magic")} {
		if fam, ok := assettype.Sniff(head); ok {
			t.Errorf("Sniff(%v) = %q,true; want unrecognized", head, fam)
		}
	}
}

// The renamed-file detection primitive: content that disagrees with the (implied)
// extension is caught by Sniff. Here PNG bytes are recognized as PNG regardless
// of what a .jpg name would claim — the ingest mismatch policy (impl/04) badges it.
func TestSniff_DetectsContentOverName(t *testing.T) {
	pngHead := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	got, ok := assettype.Sniff(pngHead)
	if !ok || got != assettype.FamilyPNG {
		t.Fatalf("PNG bytes must sniff as PNG even under a .jpg name: got %q,%v", got, ok)
	}
}
