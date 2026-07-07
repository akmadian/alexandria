package assettype

import (
	"bytes"
	"strings"
)

// The byte signatures below are drawn from these authoritative references:
//
//   - WHATWG MIME Sniffing Standard — https://mimesniff.spec.whatwg.org/
//     (the standardized pattern-matching algorithm; the same signatures Go's
//     own net/http.DetectContentType implements in net/http/sniff.go)
//   - Wikipedia, "List of file signatures" —
//     https://en.wikipedia.org/wiki/List_of_file_signatures
//
// Container formats follow their own specs: ISO/IEC 14496-12 (the `ftyp` box
// shared by MP4/MOV/HEIC), EBML/Matroska (https://www.matroska.org/), and the
// ID3 tag (https://id3.org/) plus the MPEG-1 audio frame sync for bare MP3s.

// ContentFamily is the coarse type a file's header reveals. It is deliberately
// coarser than the extension: every TIFF-based RAW (CR2/NEF/ARW/DNG) reads as
// one family, and the extension picks the dialect. It exists to catch mislabeled
// files — content validates, the extension refines.
type ContentFamily string

const (
	FamilyJPEG       ContentFamily = "jpeg"
	FamilyPNG        ContentFamily = "png"
	FamilyGIF        ContentFamily = "gif"
	FamilyTIFF       ContentFamily = "tiff" // includes CR2/NEF/ARW/DNG containers
	FamilyBMP        ContentFamily = "bmp"
	FamilyWebP       ContentFamily = "webp"
	FamilyISOBMFF    ContentFamily = "isobmff" // MP4/MOV/M4A/M4V/HEIC (ftyp box)
	FamilyPDF        ContentFamily = "pdf"
	FamilyPSD        ContentFamily = "psd"
	FamilyXML        ContentFamily = "xml" // SVG and other XML
	FamilyMP3        ContentFamily = "mp3"
	FamilyFLAC       ContentFamily = "flac"
	FamilyWAV        ContentFamily = "wav"
	FamilyMatroska   ContentFamily = "matroska"   // MKV
	FamilyPostScript ContentFamily = "postscript" // AI/EPS
)

// Sniff reports the content family from a file's leading bytes. The hash stage's
// 64KB buffer is far more than enough — only the first ~16 bytes are read. A
// short, empty, or unrecognized head returns ("", false) without panicking.
//
// Policy (D7): the extension classifies provisionally (no I/O); Sniff validates
// against the bytes we already read for hashing. On disagreement the content
// wins the family and the caller badges the mismatch.
func Sniff(head []byte) (ContentFamily, bool) {
	at := func(off int, sig ...byte) bool {
		return len(head) >= off+len(sig) && bytes.Equal(head[off:off+len(sig)], sig)
	}

	switch {
	case at(0, 0xFF, 0xD8, 0xFF):
		return FamilyJPEG, true
	case at(0, 0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A):
		return FamilyPNG, true
	case at(0, 'G', 'I', 'F', '8'):
		return FamilyGIF, true
	case at(0, 'R', 'I', 'F', 'F'):
		// RIFF container — disambiguated at offset 8.
		switch {
		case at(8, 'W', 'E', 'B', 'P'):
			return FamilyWebP, true
		case at(8, 'W', 'A', 'V', 'E'):
			return FamilyWAV, true
		}
		return "", false
	case at(4, 'f', 't', 'y', 'p'):
		return FamilyISOBMFF, true // MP4/MOV/M4A/M4V/HEIC; brand (offset 8) picks dialect
	case at(0, 'I', 'I', 0x2A, 0x00), at(0, 'M', 'M', 0x00, 0x2A):
		return FamilyTIFF, true // little-/big-endian TIFF; RAW containers included
	case at(0, 'B', 'M'):
		return FamilyBMP, true
	case at(0, '%', 'P', 'D', 'F'):
		return FamilyPDF, true
	case at(0, '8', 'B', 'P', 'S'):
		return FamilyPSD, true
	case at(0, 'f', 'L', 'a', 'C'):
		return FamilyFLAC, true
	case at(0, 0x1A, 0x45, 0xDF, 0xA3):
		return FamilyMatroska, true
	case at(0, '%', '!', 'P', 'S'):
		return FamilyPostScript, true
	case at(0, 'I', 'D', '3'), mp3FrameSync(head):
		return FamilyMP3, true
	case looksLikeXML(head):
		return FamilyXML, true
	}
	return "", false
}

// mp3FrameSync matches an MPEG audio frame sync (11 set bits) for MP3s without an
// ID3 tag.
func mp3FrameSync(head []byte) bool {
	return len(head) >= 2 && head[0] == 0xFF && head[1]&0xE0 == 0xE0
}

// looksLikeXML reports whether the head begins (after an optional BOM and
// whitespace) with an XML or SVG opener — enough to recognize .svg assets.
func looksLikeXML(head []byte) bool {
	b := head
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:] // UTF-8 BOM
	}
	b = bytes.TrimLeft(b, " \t\r\n")
	return hasPrefixFold(b, "<?xml") || hasPrefixFold(b, "<svg")
}

func hasPrefixFold(b []byte, prefix string) bool {
	if len(b) < len(prefix) {
		return false
	}
	return strings.EqualFold(string(b[:len(prefix)]), prefix)
}
