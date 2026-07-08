package importer

import (
	"context"
	"io/fs"
	"strings"
	"time"

	"github.com/akmadian/alexandria/internal/assettype"
	"github.com/akmadian/alexandria/internal/domain"
)

// SCAN is the bouncer at the front door (D18/D13): it walks the tree and emits a
// pipelineItem per candidate file, filtering out hidden files, ignore-list hits
// (tallied by pattern), unknown extensions (tallied by extension), empty files,
// and unchanged files (the skip gate). Sidecars are recognized here and routed
// through as sidecar items. It also records every visited path for the walk-end
// missing diff, and flips progress to determinate when the walk completes.

func (pipe *pipeline) scan(ctx context.Context, out chan<- *pipelineItem) error {
	err := fs.WalkDir(pipe.fsys, ".", func(relativePath string, entry fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			pipe.addRunError(relativePath, "scan", walkErr) // one unreadable entry never aborts the walk
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if relativePath != "." && (isHidden(name) || pipe.importer.Settings.Ignored(name)) {
				return fs.SkipDir
			}
			return nil
		}

		pipe.visited[relativePath] = struct{}{} // every file present on disk (for the missing diff)

		if isHidden(name) {
			return nil
		}
		if pattern := pipe.importer.Settings.MatchIgnore(name); pattern != "" {
			pipe.ignoredTally[pattern]++
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			pipe.addRunError(relativePath, "scan", err)
			return nil
		}
		if info.Size() == 0 {
			pipe.importer.Log.Debug("empty file skipped", "path", relativePath, "asset", name)
			return nil // empty file is not an asset; self-heals when it gains content
		}
		extension := ext(name)
		if assettype.IsSidecar(extension) {
			pipe.importer.Log.Debug("sidecar detected", "path", relativePath, "ext", extension)
			return pipe.emit(ctx, out, &pipelineItem{scanned: sidecarScan(relativePath, name, extension, info), isSidecar: true})
		}
		handler, ok := assettype.Classify(extension)
		if !ok {
			pipe.unknownTally[extension]++ // counted, never a row (D13)
			return nil
		}
		scanned := scannedFile{
			relPath: relativePath, filename: name, ext: extension, mime: handler.MIME,
			fileType: handler.Type, handler: handler, size: info.Size(), mtime: info.ModTime(),
		}
		if unchanged(scanned, pipe.known) {
			pipe.skippedCount++
			return nil
		}
		pipe.total.Add(1)
		return pipe.emit(ctx, out, &pipelineItem{scanned: scanned})
	})
	if err == nil {
		pipe.walkDone.Store(true) // Total is now final → progress upgrades to determinate
	}
	return err
}

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
// hidden or unsupported files (which are skipped, not errors). This is the
// single-file entry the watcher path (IngestFile) uses; the SCAN stage above
// inlines the same checks so it can also route sidecars and tally skips.
func scan(path string, info fs.FileInfo) (scannedFile, bool) {
	name := info.Name()
	if isHidden(name) {
		return scannedFile{}, false
	}
	extension := ext(name)
	handler, ok := assettype.Classify(extension)
	if !ok {
		return scannedFile{}, false
	}
	return scannedFile{
		relPath:  path,
		filename: name,
		ext:      extension,
		mime:     handler.MIME,
		fileType: handler.Type,
		handler:  handler,
		size:     info.Size(),
		mtime:    info.ModTime(),
	}, true
}

// sidecarScan builds the file-level facts for a companion file (XMP/AAE/THM/…).
// Sidecars carry no handler (they are never decoded as assets); they HASH for
// change detection and route straight to WRITE.
func sidecarScan(relativePath, name, extension string, info fs.FileInfo) scannedFile {
	return scannedFile{
		relPath:  relativePath,
		filename: name,
		ext:      extension,
		size:     info.Size(),
		mtime:    info.ModTime(),
	}
}

// mtimeTolerance absorbs filesystem timestamp-resolution differences: FAT/exFAT
// stores 2-second mtimes and some SMB servers truncate sub-second precision.
// Compare mtimes within this tolerance; size must still match exactly.
const mtimeTolerance = 2 * time.Second

// unchanged reports whether a scanned file matches a known catalog entry closely
// enough to skip: exact size and mtime within tolerance. This is the idempotency
// gate — re-running on an unchanged source hashes nothing.
func unchanged(scanned scannedFile, known map[string]domain.FileStat) bool {
	previous, ok := known[scanned.relPath]
	if !ok {
		return false
	}
	return previous.SizeBytes == scanned.size && absDuration(scanned.mtime.Sub(previous.MTime)) <= mtimeTolerance
}

func isHidden(name string) bool { return strings.HasPrefix(name, ".") }

func ext(name string) string {
	if lastDot := strings.LastIndexByte(name, '.'); lastDot >= 0 {
		return strings.ToLower(name[lastDot+1:])
	}
	return ""
}

func absDuration(duration time.Duration) time.Duration {
	if duration < 0 {
		return -duration
	}
	return duration
}
