package importer

import (
	"context"
	"errors"
	"io/fs"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/settings"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/gospan"
	"github.com/charmbracelet/log"
)

// Importer indexes the files under a source into the catalog. It holds only
// injected dependencies (no per-run state), so one Importer is safe to reuse
// across imports.
//
// The single-file and reconcile paths use the writer-scoped catalog interfaces
// (docs/data-model.md §1): a reader and the observation writer. They are given
// NO judgment or sync writer — ingest cannot touch ratings/flags/notes, so a
// reimport can never clobber user judgment. That guarantee is structural (the
// types), not a convention. The one sanctioned derived-class write is the
// reimport staleness clear (D28), which runs through the Store transaction.
//
// Thumbnails are not ingest's job (D25): they are produced post-commit by the
// enrichment engine. OnAssetCommitted is where the composition root wires the
// dispatcher nudge.
//
// The batched pipeline path (Run/RunJob) holds the sqlite Store: each 50-item
// commit is one Store.InTx, the transaction seam impl/02 built. Within that one
// function it uses only the observation/derived/dup/sidecar/import repos on
// Repos — the "one cook" that owns every catalog mutation.
type Importer struct {
	Reader  catalog.AssetReader
	Obs     catalog.AssetObservationWriter
	Dups    catalog.DuplicateRepository
	Store   *sqlite.Store      // transaction boundary (pipeline WRITE batches + the reimport staleness clear)
	Imports *sqlite.ImportRepo // session lifecycle (Start/Finish outside the batch txns)
	Log     *log.Logger

	// Tracer, if set, instruments the pipeline path with gospan spans: one run
	// root, one trace per item (import.asset / import.sidecar) with per-stage
	// child spans, and one tiny trace per WRITE batch (the fan-in recipe). Nil is
	// off — every call on a nil tracer is a ~4ns no-op, so untraced runs pay
	// nothing. The trace file is observational exhaust, never catalog state.
	Tracer *gospan.Tracer

	// Settings is the catalog's settings snapshot, injected by the composition root.
	// SCAN consults it for the D18 ignore list (Settings.MatchIgnore) and the WRITE
	// batch size (Settings.ImportBatchSize) — settings owns those, so the importer
	// holds no copy. The zero value is safe: empty patterns match nothing and a
	// zero batch size falls back to defaultBatchSize (a bare Importer{} still runs).
	Settings settings.Settings

	// Machine is the machine-scoped config snapshot (worker-pool sizes), injected by
	// the composition root. resolvePools reads Machine.Workers.Ingest; a zero count
	// falls back to defaultPools. HASH is I/O-bound (raise for fast SSDs); EXTRACT
	// is CPU-bound (raise toward NumCPU).
	Machine settings.Machine

	// OnProgress, if set, fires per batch commit and at walk completion. Nil-safe.
	OnProgress func(Progress)

	// OnAssetCommitted, if set, fires after an asset is committed (batch or
	// single-file) with the asset ID, its volume, and its volume-relative path.
	// The XMP sync trigger uses this to run SyncSidecar for assets that have a
	// companion .xmp sidecar. Errors from the hook are logged, never fatal — sync
	// is best-effort at ingest time.
	OnAssetCommitted func(ctx context.Context, volumeID string, assetID string, relativePath string)
}

// Target is what a run walks and how its assets key (the D24 rekey). The walk's
// filesystem (fsys) is rooted at the volume's mount point, so every walk-relative
// path IS volume-relative and assets/sidecars key on (VolumeID, that path).
// WalkRoot is the tracked folder's volume-relative path — the subtree to walk;
// "" walks the whole volume.
type Target struct {
	VolumeID string
	WalkRoot string // volume-relative; "" = the whole volume
	Name     string // display, for logs
}

// walkStart is the fs.WalkDir start path for a target: "." at the volume root,
// else the folder's volume-relative path.
func (t Target) walkStart() string {
	if t.WalkRoot == "" {
		return "."
	}
	return t.WalkRoot
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

// Run scans fsys — the volume-rooted filesystem for target — and indexes every
// supported file under target.WalkRoot into the catalog through the concurrent
// pipeline. The volume must already exist (the path resolver mints it); the
// importer neither creates nor resolves volumes. Only catastrophic failures
// return an error; per-file failures land in the result (and the DLQ) and the
// scan continues.
func (imp *Importer) Run(ctx context.Context, target Target, fsys fs.FS) (ImportResult, error) {
	return imp.RunJob(ctx, "", target, fsys)
}

// IngestFile indexes a single file (the watcher path): the same stages as the
// pipeline, minus the walk, the skip gate, and batching. A batch of one, run
// sequentially — the hint consumer for impl/05. name is volume-relative (the
// walk fsys is volume-rooted). A gone path marks the asset missing (D20: no
// move detection); a present file is ingested.
func (imp *Importer) IngestFile(ctx context.Context, target Target, fsys fs.FS, name string) error {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		// Gone → the SAME action the walk-end diff takes: mark missing. This is what
		// makes a watcher-fed delete heal identically to a walk-detected one (impl/05
		// corrected model).
		if errors.Is(err, fs.ErrNotExist) {
			return imp.markGone(ctx, target, name)
		}
		return err // transient (perms, EIO) — leave status as-is; reconcile heals
	}
	scanned, ok := scan(name, info)
	if !ok {
		imp.Log.Debug("ignored unsupported file", "path", name)
		return nil
	}
	fileLogger := imp.Log.With("asset", scanned.filename)
	hash, err := hashFile(fsys, &scanned)
	if err != nil {
		return err
	}
	verdict, existing, err := imp.classify(ctx, target, &scanned, hash, nil, fileLogger)
	if err != nil {
		return err
	}
	extractedMetadata := imp.metadataFor(fsys, &scanned, verdict, fileLogger)
	assetID, err := imp.persist(ctx, target, &scanned, hash, &extractedMetadata, verdict, existing, fileLogger)
	if err != nil {
		return err
	}
	fileLogger.Info("ingested file", "volume", target.Name, "path", scanned.relPath, "verdict", verdict, "assetID", assetID)
	if imp.OnAssetCommitted != nil {
		imp.OnAssetCommitted(ctx, target.VolumeID, assetID, scanned.relPath)
	}
	return nil
}
