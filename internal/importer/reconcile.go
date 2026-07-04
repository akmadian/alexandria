package importer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
)

// ReconcileResult summarizes a reconciliation pass.
type ReconcileResult struct {
	Missing  int // files gone from disk, marked missing
	Restored int // files that reappeared, flipped back online
	Errors   []ImportError
}

// Reconcile compares the catalog's record of a source against what's on fsys now
// and updates file_status. A file gone from disk becomes `missing`; a file that
// reappeared becomes `online`. If the whole source is unreachable, every asset
// is marked `offline` (the files are presumed intact, just unmounted) and no
// per-file work is done.
//
// Reconcile is what activates move detection: a later import that finds the same
// content at a new path relinks to the `missing` record instead of duplicating.
func (imp *Importer) Reconcile(ctx context.Context, source *domain.Source, fsys fs.FS) (ReconcileResult, error) {
	var result ReconcileResult

	// Whole source unreachable → offline, no per-file work.
	if _, err := fs.Stat(fsys, "."); err != nil {
		imp.Log.Warn("source unreachable, marking offline", "source", source.Name, "err", err)
		if err := imp.Assets.MarkOfflineBySource(ctx, source.ID); err != nil {
			return result, fmt.Errorf("marking source %q offline: %w", source.ID, err)
		}
		return result, nil
	}

	assets, err := imp.Assets.List(ctx, catalog.AssetFilter{SourceIDs: []string{source.ID}})
	if err != nil {
		return result, fmt.Errorf("listing assets for source %q: %w", source.ID, err)
	}
	imp.Log.Info("reconcile started", "source", source.Name, "assets", len(assets))

	for _, a := range assets {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		_, statErr := fs.Stat(fsys, a.RelativePath)
		switch {
		case statErr == nil:
			if a.FileStatus != domain.FileStatusOnline {
				if err := imp.Assets.UpdateFileStatus(ctx, a.ID, domain.FileStatusOnline); err != nil {
					imp.recordError(&result.Errors, a.RelativePath, "reconcile", err)
					continue
				}
				result.Restored++
				imp.Log.Info("file restored", "path", a.RelativePath)
			}
		case errors.Is(statErr, fs.ErrNotExist):
			if a.FileStatus != domain.FileStatusMissing {
				if err := imp.Assets.UpdateFileStatus(ctx, a.ID, domain.FileStatusMissing); err != nil {
					imp.recordError(&result.Errors, a.RelativePath, "reconcile", err)
					continue
				}
				result.Missing++
				imp.Log.Warn("file missing", "path", a.RelativePath)
			}
		default:
			// Unexpected stat error (e.g. permissions) — record, leave status as-is.
			imp.recordError(&result.Errors, a.RelativePath, "reconcile", statErr)
		}
	}

	imp.Log.Info("reconcile finished", "source", source.Name,
		"missing", result.Missing, "restored", result.Restored, "errors", len(result.Errors))
	return result, nil
}
