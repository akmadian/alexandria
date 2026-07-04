package importer

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/metadata"
	"github.com/akmadian/alexandria/internal/thumbnailer"
	"github.com/charmbracelet/log"
)

// Importer indexes the files under a source into the catalog. It holds only
// injected dependencies (no per-run state), so one Importer is safe to reuse
// across imports.
type Importer struct {
	Assets    catalog.AssetRepository
	Dups      catalog.DuplicateRepository
	Metadata  metadata.Extractor
	Thumbnail thumbnailer.Thumbnailer
	Log       *log.Logger
}

// ImportError records one file that failed a stage. Per-file failures never
// abort the run — they accumulate in ImportResult.Errors.
type ImportError struct {
	Path  string
	Stage string
	Err   error
}

// ImportResult summarizes a completed run.
type ImportResult struct {
	Added   int
	Updated int
	Moved   int
	Skipped int
	Dups    int
	Errors  []ImportError
}

// Run scans fsys — the resolved filesystem for source — and indexes every
// supported file into the catalog. source must already be registered; the
// importer neither creates nor resolves sources. Only catastrophic failures
// return an error; per-file failures land in the result and the scan continues.
func (imp *Importer) Run(ctx context.Context, source *domain.Source, fsys fs.FS) (ImportResult, error) {
	var result ImportResult

	known, err := imp.Assets.ListKnownFiles(ctx, source.ID)
	if err != nil {
		return result, fmt.Errorf("loading known files for source %q: %w", source.ID, err)
	}
	imp.Log.Info("import started", "source", source.Name, "known", len(known))

	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if walkErr != nil {
			imp.recordError(&result.Errors, path, "scan", walkErr)
			return nil // a single unreadable dir shouldn't abort the walk
		}
		if d.IsDir() {
			return nil
		}
		imp.ingestOne(ctx, source, fsys, path, d, known, &result)
		return nil
	})
	if err != nil {
		imp.Log.Warn("import canceled", "source", source.Name, "err", err)
		return result, err
	}

	imp.Log.Info("import finished", "source", source.Name,
		"added", result.Added, "updated", result.Updated, "moved", result.Moved,
		"skipped", result.Skipped, "dups", result.Dups, "errors", len(result.Errors))
	return result, nil
}

// IngestFile indexes a single file (the watcher path): the same stages as Run,
// minus the directory walk and the skip gate. A batch of one.
func (imp *Importer) IngestFile(ctx context.Context, source *domain.Source, fsys fs.FS, name string) error {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return err
	}
	sf, ok := scan(name, info)
	if !ok {
		imp.Log.Debug("ignored unsupported file", "path", name)
		return nil
	}
	hash, err := hashFile(fsys, sf)
	if err != nil {
		return err
	}
	act, existing, err := imp.classifyContent(ctx, source, sf, hash)
	if err != nil {
		return err
	}
	md := imp.metadataFor(fsys, sf, act)
	assetID, err := imp.persist(ctx, source, sf, hash, md, act, existing)
	if err != nil {
		return err
	}
	imp.thumbnail(ctx, fsys, sf, assetID, act)
	return nil
}

func (imp *Importer) ingestOne(ctx context.Context, source *domain.Source, fsys fs.FS, path string, d fs.DirEntry, known map[string]domain.FileStat, result *ImportResult) {
	info, err := d.Info()
	if err != nil {
		imp.recordError(&result.Errors, path, "scan", err)
		return
	}
	sf, ok := scan(path, info)
	if !ok {
		imp.Log.Debug("skipped unsupported file", "path", path)
		return
	}
	if unchanged(sf, known) {
		result.Skipped++
		imp.Log.Debug("skip unchanged", "path", path, "size", sf.size)
		return
	}

	hash, err := hashFile(fsys, sf)
	if err != nil {
		imp.recordError(&result.Errors, path, "hashing", err)
		return
	}
	imp.Log.Debug("hashed", "path", path, "hash", hash, "size", sf.size)

	act, existing, err := imp.classifyContent(ctx, source, sf, hash)
	if err != nil {
		imp.recordError(&result.Errors, path, "dedup", err)
		return
	}
	imp.Log.Debug("classified", "path", path, "action", act, "type", sf.fileType)

	md := imp.metadataFor(fsys, sf, act)
	assetID, err := imp.persist(ctx, source, sf, hash, md, act, existing)
	if err != nil {
		// Write failures are the serious ones — surface loudly, not just in the list.
		imp.Log.Error("catalog write failed", "path", path, "err", err)
		result.Errors = append(result.Errors, ImportError{Path: path, Stage: "write", Err: err})
		return
	}
	imp.thumbnail(ctx, fsys, sf, assetID, act)

	switch act {
	case actionNew:
		result.Added++
		imp.Log.Debug("indexed new asset", "path", path)
	case actionReimport:
		result.Updated++
		imp.Log.Debug("reindexed changed asset", "path", path)
	case actionMove:
		result.Moved++
		imp.Log.Info("relinked moved file", "path", path)
	case actionDuplicate:
		result.Added++
		result.Dups++
		imp.Log.Warn("duplicate content", "path", path)
	}
}

// recordError logs a per-file failure at warn level and appends it to the given
// error slice. Shared by Run and Reconcile.
func (imp *Importer) recordError(errs *[]ImportError, path, stage string, err error) {
	imp.Log.Warn("file skipped after error", "path", path, "stage", stage, "err", err)
	*errs = append(*errs, ImportError{Path: path, Stage: stage, Err: err})
}
