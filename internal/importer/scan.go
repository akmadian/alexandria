package importer

import (
	"io/fs"
	"strings"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

// scannedFile is the file-level facts gathered before hashing.
type scannedFile struct {
	relPath  string
	filename string
	ext      string
	mime     string
	fileType domain.FileType
	size     int64
	mtime    time.Time
}

// scan turns a filesystem entry into a scannedFile, reporting ok=false for
// hidden or unsupported files (which are skipped, not errors).
func scan(path string, info fs.FileInfo) (scannedFile, bool) {
	name := info.Name()
	if isHidden(name) {
		return scannedFile{}, false
	}
	e := ext(name)
	mime, ft, ok := classify(e)
	if !ok {
		return scannedFile{}, false
	}
	return scannedFile{
		relPath:  path,
		filename: name,
		ext:      e,
		mime:     mime,
		fileType: ft,
		size:     info.Size(),
		mtime:    info.ModTime(),
	}, true
}

type fileKind struct {
	mime     string
	fileType domain.FileType
}

// classify maps a lowercase extension to its MIME type and FileType. Extension
// is the fast path; header sniffing is deferred (see coding-guidelines §1).
func classify(extension string) (mime string, ft domain.FileType, ok bool) {
	t, ok := extToType[extension]
	return t.mime, t.fileType, ok
}

var extToType = map[string]fileKind{
	// images
	"jpg":  {"image/jpeg", domain.FileTypeImage},
	"jpeg": {"image/jpeg", domain.FileTypeImage},
	"png":  {"image/png", domain.FileTypeImage},
	"gif":  {"image/gif", domain.FileTypeImage},
	"webp": {"image/webp", domain.FileTypeImage},
	"tif":  {"image/tiff", domain.FileTypeImage},
	"tiff": {"image/tiff", domain.FileTypeImage},
	"heic": {"image/heic", domain.FileTypeImage},
	"bmp":  {"image/bmp", domain.FileTypeImage},
	// camera raw
	"cr2": {"image/x-canon-cr2", domain.FileTypeRaw},
	"cr3": {"image/x-canon-cr3", domain.FileTypeRaw},
	"nef": {"image/x-nikon-nef", domain.FileTypeRaw},
	"arw": {"image/x-sony-arw", domain.FileTypeRaw},
	"dng": {"image/x-adobe-dng", domain.FileTypeRaw},
	"orf": {"image/x-olympus-orf", domain.FileTypeRaw},
	"raf": {"image/x-fuji-raf", domain.FileTypeRaw},
	"rw2": {"image/x-panasonic-rw2", domain.FileTypeRaw},
	// video
	"mov": {"video/quicktime", domain.FileTypeVideo},
	"mp4": {"video/mp4", domain.FileTypeVideo},
	"m4v": {"video/x-m4v", domain.FileTypeVideo},
	"avi": {"video/x-msvideo", domain.FileTypeVideo},
	"mkv": {"video/x-matroska", domain.FileTypeVideo},
	// audio
	"mp3":  {"audio/mpeg", domain.FileTypeAudio},
	"wav":  {"audio/wav", domain.FileTypeAudio},
	"flac": {"audio/flac", domain.FileTypeAudio},
	"aac":  {"audio/aac", domain.FileTypeAudio},
	"m4a":  {"audio/mp4", domain.FileTypeAudio},
	// vector
	"svg": {"image/svg+xml", domain.FileTypeVector},
	"ai":  {"application/postscript", domain.FileTypeVector},
	"eps": {"application/postscript", domain.FileTypeVector},
	// documents
	"pdf":  {"application/pdf", domain.FileTypeDocument},
	"psd":  {"image/vnd.adobe.photoshop", domain.FileTypeDocument},
	"indd": {"application/x-indesign", domain.FileTypeDocument},
}

// mtimeTolerance absorbs filesystem timestamp-resolution differences: FAT/exFAT
// stores 2-second mtimes and some SMB servers truncate sub-second precision.
// Compare mtimes within this tolerance; size must still match exactly.
const mtimeTolerance = 2 * time.Second

// unchanged reports whether a scanned file matches a known catalog entry closely
// enough to skip: exact size and mtime within tolerance. This is the idempotency
// gate — re-running on an unchanged source hashes nothing.
func unchanged(sf scannedFile, known map[string]domain.FileStat) bool {
	prev, ok := known[sf.relPath]
	if !ok {
		return false
	}
	return prev.SizeBytes == sf.size && absDuration(sf.mtime.Sub(prev.MTime)) <= mtimeTolerance
}

func isHidden(name string) bool { return strings.HasPrefix(name, ".") }

func ext(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return strings.ToLower(name[i+1:])
	}
	return ""
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
