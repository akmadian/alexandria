// Package assettype is the single source of truth for which file extensions
// Alexandria recognizes and what it can do with each: MIME type, category
// (domain.FileType), and the per-type capability functions (metadata extraction,
// thumbnail generation). It also sniffs content to catch mislabeled files.
//
// "Type" (not "Kind") matches the repo's convention: Type is a file's format
// category (FileType/MIMEType/file_type), whereas Kind is a variant within an
// entity (SourceKind, CollectionKind). This package resolves the former.
//
// One explicit table (`registry`) — add a format by adding a row, and nowhere
// else. This deliberately replaces the three parallel maps that used to drift
// (an ext→MIME table in domain, a MIME→extractor map in metadata, a MIME→thumb
// map in thumbnailer). The dispatch key is the normalized extension; MIME is an
// output attribute for the seam, not a key. A nil capability means "no such
// capability yet" — callers degrade gracefully (skip metadata, show a generic
// card), never error.
//
// No init() self-registration: the closed set is small, and a single visible
// table IS the documentation of what's supported and how well.
package assettype

import (
	"strings"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/metadata"
	"github.com/akmadian/alexandria/internal/thumbnailer"
)

// Handler describes one recognized extension: how to categorize it and what the
// pipeline can do with it.
type Handler struct {
	Ext      string               // canonical dispatch key: lowercase, no dot
	MIME     string               // seam/webview attribute, NOT a dispatch key
	Type     domain.FileType      // coarse category
	Metadata metadata.ExtractFunc // nil = no extractor
	Thumb    thumbnailer.GenFunc  // nil = no thumbnail generator
	// Preview, Grouping join here when their features ship (rule of two).
}

// registry is the whole supported-format table. Only the stdlib-decodable raster
// formats (jpeg/png/gif) carry capability funcs today; the rest classify but have
// no extractor/generator until their decoders land (RAW/video/PDF/… via the
// dependency fleet). Add a format = add a row.
var registry = []Handler{
	// images
	{"jpg", "image/jpeg", domain.FileTypeImage, metadata.ExtractImage, thumbnailer.GenerateImage},
	{"jpeg", "image/jpeg", domain.FileTypeImage, metadata.ExtractImage, thumbnailer.GenerateImage},
	{"png", "image/png", domain.FileTypeImage, metadata.ExtractImage, thumbnailer.GenerateImage},
	{"gif", "image/gif", domain.FileTypeImage, metadata.ExtractImage, thumbnailer.GenerateImage},
	{"webp", "image/webp", domain.FileTypeImage, nil, nil},
	{"tif", "image/tiff", domain.FileTypeImage, nil, nil},
	{"tiff", "image/tiff", domain.FileTypeImage, nil, nil},
	{"heic", "image/heic", domain.FileTypeImage, nil, nil},
	{"bmp", "image/bmp", domain.FileTypeImage, nil, nil},
	// camera raw
	{"cr2", "image/x-canon-cr2", domain.FileTypeRaw, nil, nil},
	{"cr3", "image/x-canon-cr3", domain.FileTypeRaw, nil, nil},
	{"nef", "image/x-nikon-nef", domain.FileTypeRaw, nil, nil},
	{"arw", "image/x-sony-arw", domain.FileTypeRaw, nil, nil},
	{"dng", "image/x-adobe-dng", domain.FileTypeRaw, nil, nil},
	{"orf", "image/x-olympus-orf", domain.FileTypeRaw, nil, nil},
	{"raf", "image/x-fuji-raf", domain.FileTypeRaw, nil, nil},
	{"rw2", "image/x-panasonic-rw2", domain.FileTypeRaw, nil, nil},
	// video
	{"mov", "video/quicktime", domain.FileTypeVideo, nil, nil},
	{"mp4", "video/mp4", domain.FileTypeVideo, nil, nil},
	{"m4v", "video/x-m4v", domain.FileTypeVideo, nil, nil},
	{"avi", "video/x-msvideo", domain.FileTypeVideo, nil, nil},
	{"mkv", "video/x-matroska", domain.FileTypeVideo, nil, nil},
	// audio
	{"mp3", "audio/mpeg", domain.FileTypeAudio, nil, nil},
	{"wav", "audio/wav", domain.FileTypeAudio, nil, nil},
	{"flac", "audio/flac", domain.FileTypeAudio, nil, nil},
	{"aac", "audio/aac", domain.FileTypeAudio, nil, nil},
	{"m4a", "audio/mp4", domain.FileTypeAudio, nil, nil},
	// vector
	{"svg", "image/svg+xml", domain.FileTypeVector, nil, nil},
	{"ai", "application/postscript", domain.FileTypeVector, nil, nil},
	{"eps", "application/postscript", domain.FileTypeVector, nil, nil},
	// documents
	{"pdf", "application/pdf", domain.FileTypeDocument, nil, nil},
	{"psd", "image/vnd.adobe.photoshop", domain.FileTypeDocument, nil, nil},
	{"indd", "application/x-indesign", domain.FileTypeDocument, nil, nil},
}

// byExtension indexes registry for lookup; built once at startup.
var byExtension = func() map[string]Handler {
	m := make(map[string]Handler, len(registry))
	for _, h := range registry {
		m[h.Ext] = h
	}
	return m
}()

// Classify maps a file extension (with or without a leading dot, any case) to its
// handler. ok is false for unsupported extensions.
func Classify(ext string) (Handler, bool) {
	h, ok := byExtension[normalizeExt(ext)]
	return h, ok
}

// IsSupported reports whether the extension is a recognized asset type.
func IsSupported(ext string) bool {
	_, ok := byExtension[normalizeExt(ext)]
	return ok
}

// sidecarExts are companion files tracked but never treated as assets. v1 parses
// only .xmp; the rest are tracked for future features (Apple edits, camera
// thumbnails, GoPro proxies, other editors' sidecars).
var sidecarExts = map[string]bool{
	"xmp": true, "aae": true, "thm": true, "lrv": true,
	"pp3": true, "dop": true, "on1": true,
}

// IsSidecar reports whether ext is a known sidecar/companion extension.
func IsSidecar(ext string) bool {
	return sidecarExts[normalizeExt(ext)]
}

func normalizeExt(ext string) string {
	return strings.ToLower(strings.TrimPrefix(ext, "."))
}
