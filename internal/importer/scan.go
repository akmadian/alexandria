package importer

import (
	"io/fs"
	"strings"
	"time"

	"github.com/akmadian/alexandria/internal/assettype"
	"github.com/akmadian/alexandria/internal/domain"
)

// scannedFile is the file-level facts gathered before hashing. handler carries
// the per-type capability funcs (metadata/thumbnail) so the extract and thumbnail
// stages dispatch off the row we already resolved here — no second lookup.
type scannedFile struct {
	relPath  string
	filename string
	ext      string
	mime     string
	fileType domain.FileType
	handler  assettype.Handler
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
	kind, ok := assettype.Classify(e)
	if !ok {
		return scannedFile{}, false
	}
	return scannedFile{
		relPath:  path,
		filename: name,
		ext:      e,
		mime:     kind.MIME,
		fileType: kind.Type,
		handler:  kind,
		size:     info.Size(),
		mtime:    info.ModTime(),
	}, true
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
