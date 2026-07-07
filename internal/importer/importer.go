package importer

import (
	"context"
	"io/fs"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/thumbnailer"
	"github.com/charmbracelet/log"
)

// Importer indexes the files under a source into the catalog. It holds only
// injected dependencies (no per-run state), so one Importer is safe to reuse
// across imports.
//
// The single-file and reconcile paths use the writer-scoped catalog interfaces
// (docs/v2/.../03-data-model.md §1): a reader, the observation writer, and the
// derived writer (for the thumbnail marker). They are given NO judgment or sync
// writer — ingest cannot touch ratings/flags/notes, so a reimport can never
// clobber user judgment. That guarantee is structural (the types), not a
// convention.
//
// The batched pipeline path (Run/RunJob) additionally holds the sqlite Store:
// each 50-item commit is one Store.InTx, the transaction seam impl/02 built.
// Within that one function it uses only the observation/derived/dup/sidecar/
// import repos on Repos — the "one cook" that owns every catalog mutation.
type Importer struct {
	Reader    catalog.AssetReader
	Obs       catalog.AssetObservationWriter
	Derived   catalog.AssetDerivedWriter
	Dups      catalog.DuplicateRepository
	Thumbnail thumbnailer.Thumbnailer
	Store     *sqlite.Store      // batched-write transaction boundary (pipeline WRITE)
	Imports   *sqlite.ImportRepo // session lifecycle (Start/Finish outside the batch txns)
	Log       *log.Logger

	// Worker-pool overrides for the concurrent pipeline. Zero means "use the
	// built-in default" (defaultPools). These are the per-machine tuning knob
	// until machine.json lands; the dev harness exposes them as flags. HASH is
	// I/O-bound (raise for fast SSDs), EXTRACT and THUMB are CPU-bound (raise
	// toward NumCPU — but each in-flight THUMB holds a fully-decoded image, so
	// more workers cost proportionally more memory).
	HashWorkers    int
	ExtractWorkers int
	ThumbWorkers   int

	// OnProgress, if set, fires per batch commit and at walk completion. Nil-safe.
	OnProgress func(Progress)
}

// ImportError records one file that failed a stage. Per-file failures never
// abort the run — they accumulate in ImportResult.Errors and, durably, as
// import_errors DLQ rows.
type ImportError struct {
	Path  string
	Stage string
	Err   error
}

// ImportResult summarizes a completed run.
type ImportResult struct {
	SessionID string
	Added     int
	Updated   int
	Moved     int
	Skipped   int
	Dups      int
	Missing   int // walk-end diff: known files no longer on disk (full walks only)
	Errors    []ImportError
}

// Run scans fsys — the resolved filesystem for source — and indexes every
// supported file into the catalog through the concurrent pipeline. source must
// already be registered; the importer neither creates nor resolves sources.
// Only catastrophic failures return an error; per-file failures land in the
// result (and the DLQ) and the scan continues.
func (imp *Importer) Run(ctx context.Context, source *domain.Source, fsys fs.FS) (ImportResult, error) {
	return imp.RunJob(ctx, "", source, fsys)
}

// IngestFile indexes a single file (the watcher path): the same stages as the
// pipeline, minus the walk, the skip gate, and batching. A batch of one, run
// sequentially — the hint consumer for impl/05.
func (imp *Importer) IngestFile(ctx context.Context, source *domain.Source, fsys fs.FS, name string) error {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return err
	}
	scanned, ok := scan(name, info)
	if !ok {
		imp.Log.Debug("ignored unsupported file", "path", name)
		return nil
	}
	hash, err := hashFile(fsys, scanned)
	if err != nil {
		return err
	}
	verdict, existing, err := imp.classify(ctx, source, scanned, hash, nil)
	if err != nil {
		return err
	}
	extractedMetadata := imp.metadataFor(fsys, scanned, verdict)
	assetID, err := imp.persist(ctx, source, scanned, hash, extractedMetadata, verdict, existing)
	if err != nil {
		return err
	}
	imp.thumbnail(ctx, fsys, scanned, assetID, verdict)
	return nil
}

// recordError logs a per-file failure at warn level and appends it to the given
// error slice. Shared by the pipeline and Reconcile.
func (imp *Importer) recordError(errs *[]ImportError, path, stage string, err error) {
	imp.Log.Warn("file skipped after error", "path", path, "stage", stage, "err", err)
	*errs = append(*errs, ImportError{Path: path, Stage: stage, Err: err})
}
