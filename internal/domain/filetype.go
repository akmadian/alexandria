package domain

import "strings"

// FileTypeInfo describes one recognized file extension: how it maps to a MIME
// type and a FileType category.
type FileTypeInfo struct {
	Extension string
	MIME      string
	Type      FileType
}

// supportedFileTypes is the single source of truth for which file extensions the
// app recognizes and how they map to MIME types and FileType categories.
// Import classification, filters, and validation all derive from this table —
// add a new format here and nowhere else. (Header sniffing, when added, should
// map its results back to these same FileType categories.)
var supportedFileTypes = []FileTypeInfo{
	// images
	{"jpg", "image/jpeg", FileTypeImage},
	{"jpeg", "image/jpeg", FileTypeImage},
	{"png", "image/png", FileTypeImage},
	{"gif", "image/gif", FileTypeImage},
	{"webp", "image/webp", FileTypeImage},
	{"tif", "image/tiff", FileTypeImage},
	{"tiff", "image/tiff", FileTypeImage},
	{"heic", "image/heic", FileTypeImage},
	{"bmp", "image/bmp", FileTypeImage},
	// camera raw
	{"cr2", "image/x-canon-cr2", FileTypeRaw},
	{"cr3", "image/x-canon-cr3", FileTypeRaw},
	{"nef", "image/x-nikon-nef", FileTypeRaw},
	{"arw", "image/x-sony-arw", FileTypeRaw},
	{"dng", "image/x-adobe-dng", FileTypeRaw},
	{"orf", "image/x-olympus-orf", FileTypeRaw},
	{"raf", "image/x-fuji-raf", FileTypeRaw},
	{"rw2", "image/x-panasonic-rw2", FileTypeRaw},
	// video
	{"mov", "video/quicktime", FileTypeVideo},
	{"mp4", "video/mp4", FileTypeVideo},
	{"m4v", "video/x-m4v", FileTypeVideo},
	{"avi", "video/x-msvideo", FileTypeVideo},
	{"mkv", "video/x-matroska", FileTypeVideo},
	// audio
	{"mp3", "audio/mpeg", FileTypeAudio},
	{"wav", "audio/wav", FileTypeAudio},
	{"flac", "audio/flac", FileTypeAudio},
	{"aac", "audio/aac", FileTypeAudio},
	{"m4a", "audio/mp4", FileTypeAudio},
	// vector
	{"svg", "image/svg+xml", FileTypeVector},
	{"ai", "application/postscript", FileTypeVector},
	{"eps", "application/postscript", FileTypeVector},
	// documents
	{"pdf", "application/pdf", FileTypeDocument},
	{"psd", "image/vnd.adobe.photoshop", FileTypeDocument},
	{"indd", "application/x-indesign", FileTypeDocument},
}

// byExtension indexes supportedFileTypes for lookup; built once at startup.
var byExtension = func() map[string]FileTypeInfo {
	m := make(map[string]FileTypeInfo, len(supportedFileTypes))
	for _, t := range supportedFileTypes {
		m[t.Extension] = t
	}
	return m
}()

// Classify maps a file extension (with or without a leading dot, any case) to
// its registry entry. ok is false for unsupported extensions.
func Classify(ext string) (FileTypeInfo, bool) {
	info, ok := byExtension[normalizeExt(ext)]
	return info, ok
}

// IsSupported reports whether the extension is a recognized asset type.
func IsSupported(ext string) bool {
	_, ok := byExtension[normalizeExt(ext)]
	return ok
}

func normalizeExt(ext string) string {
	return strings.ToLower(strings.TrimPrefix(ext, "."))
}
