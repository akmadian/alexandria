package importer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
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
	// One span for the whole walk (a sibling of the item traces, not their
	// parent). Its duration includes backpressure from a full SCAN channel —
	// honest: that IS what the walk spends its time on.
	_, walkSpan := pipe.importer.Tracer.Start(ctx, "import.scan")
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
			sidecarItem := &pipelineItem{scanned: sidecarScan(relativePath, name, extension, info), isSidecar: true}
			return pipe.emit(ctx, out, pipe.traceItem(ctx, sidecarItem))
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
		if unchanged(&scanned, pipe.known) {
			pipe.skippedCount.Add(1)
			return nil
		}
		if !pipe.walkDone.Load() {
			pipe.total.Add(1) // fallback denominator when the pre-count didn't run (see countAssets)
		}
		return pipe.emit(ctx, out, pipe.traceItem(ctx, &pipelineItem{scanned: scanned}))
	})
	if err == nil {
		pipe.walkDone.Store(true) // Total is now final → progress upgrades to determinate
	}
	if err != nil {
		walkSpan.Fail(err)
	}
	walkSpan.SetAttrs(slog.Int64("emitted", pipe.total.Load()), slog.Int64("skipped", pipe.skippedCount.Load()))
	walkSpan.End()
	return err
}

// traceItem mints the item's root trace span, a child of the run span. Assets
// and sidecars get distinct span names so per-name aggregates (Summary, SQL
// GROUP BY) never mix their timings. Stages hang child spans off item.ctx; the
// root ends after WRITE commits the item (stage_write.go endItemTrace).
func (pipe *pipeline) traceItem(ctx context.Context, item *pipelineItem) *pipelineItem {
	name := "import.asset"
	if item.isSidecar {
		name = "import.sidecar"
	}
	item.ctx, _ = pipe.importer.Tracer.Start(ctx, name, slog.String("path", item.scanned.relPath))
	return item
}

// countAssets walks the tree once, up front, counting the asset items SCAN will
// emit — so progress starts determinate ("N / total") instead of climbing from
// "N / ?". The walk's own Total only settles at the very end, because SCAN
// blocks on downstream backpressure, so it stays indeterminate for nearly the
// whole run. This pass is cheap (readdir + one Stat per candidate; no hashing,
// no decode) relative to the real work. Sidecars and unknown/ignored/hidden/
// empty/unchanged files are excluded for the same reasons SCAN skips them, so
// the count equals what SCAN emits.
//
// ponytail: mirrors SCAN's emit filter above. The two can only drift if the tree
// changes between this pass and the walk — that shifts a progress denominator,
// never a catalog fact (Finish writes the authoritative counts).
func (pipe *pipeline) countAssets(ctx context.Context) (int64, error) {
	var count int64
	err := fs.WalkDir(pipe.fsys, ".", func(relativePath string, entry fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			return nil // SCAN records the unreadable entry; the pre-count just skips it
		}
		name := entry.Name()
		if entry.IsDir() {
			if relativePath != "." && (isHidden(name) || pipe.importer.Settings.Ignored(name)) {
				return fs.SkipDir
			}
			return nil
		}
		if isHidden(name) || pipe.importer.Settings.MatchIgnore(name) != "" {
			return nil
		}
		extension := ext(name)
		if assettype.IsSidecar(extension) {
			return nil
		}
		if _, ok := assettype.Classify(extension); !ok {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() == 0 {
			return nil
		}
		if unchanged(&scannedFile{relPath: relativePath, size: info.Size(), mtime: info.ModTime()}, pipe.known) {
			return nil
		}
		count++
		return nil
	})
	return count, err
}

// preflightReadable probes the source root before the pipeline opens a session,
// turning a permission failure into an actionable error instead of a silent
// empty import. This matters twice: on macOS, TCC withholds access to protected
// and removable/network volumes (a /Volumes path fails with EPERM) until the app
// — or, for the dev harness, the terminal running it — is granted access; and a
// denied root would otherwise walk nothing, so the walk-end diff (markMissing)
// would mark every known asset in the source missing. Failing fast here prevents
// both. Only the root is probed — a deeper unreadable subtree is per-entry noise
// SCAN already records as run errors, not a reason to abort the whole import.
func preflightReadable(fsys fs.FS, displayPath string) error {
	if _, err := fs.ReadDir(fsys, "."); err != nil {
		if isPermissionDenied(err) {
			return fmt.Errorf("cannot scan %q: macOS is withholding permission for this location. "+
				"Grant Full Disk Access to Alexandria (or, running the dev harness, your terminal) in "+
				"System Settings › Privacy & Security › Full Disk Access, then retry: %w", displayPath, err)
		}
		return fmt.Errorf("cannot scan %q: %w", displayPath, err)
	}
	return nil
}

// isPermissionDenied reports whether err is a filesystem permission failure —
// classic EACCES or macOS's EPERM (TCC withholding a protected or removable/
// network volume). Both surface as fs.ErrPermission through io/fs.
func isPermissionDenied(err error) bool { return errors.Is(err, fs.ErrPermission) }

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
func unchanged(scanned *scannedFile, known map[string]domain.FileStat) bool {
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
