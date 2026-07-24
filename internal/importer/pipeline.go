package importer

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/sqlite"
	"golang.org/x/sync/errgroup"
)

// This file is the pipeline's plumbing: the channel wiring, the per-run state,
// and the run-level lifecycle (start a session, run the stages, fuse the
// walk-end missing diff, finalize). The five stages themselves live one-per-file
// in stage_*.go; the item they thread through is in item.go.
//
//	walk ─► SCAN ─► HASH ─► MATCH ─► EXTRACT ─► WRITE ─► post-commit
//	       1 grtn   pool     1        pool      1 grtn
//
// Thumbnails are NOT a stage (D25): they are the first enrichment job kind,
// produced post-commit by internal/enrichment — ingest sheds its slowest work
// and an asset commits as soon as identity + observation are complete. The
// post-commit hook nudges the enrichment dispatcher (a hint; its scan stays
// the authority).
//
// All channels are created, wired, and closed in ONE function (run). Stages are
// plain methods taking directional channels; they never make channels. Bounded
// channels make blocking sends the backpressure. MATCH is a singleton so the
// identity matrix reads a serializable view of the catalog it is mutating; WRITE
// is a singleton because SQLite is single-writer — the goroutine IS the batch
// point.

const (
	// defaultBatchSize is the fallback rows-per-WRITE-transaction when Settings
	// carries none (a bare Importer{}); the live value is Settings.ImportBatchSize.
	defaultBatchSize = 50
	writeLull        = 500 * time.Millisecond
)

// poolSizes are the per-stage worker counts.
type poolSizes struct{ hash, extract int }

// defaultPools is the fallback per-stage worker count when Machine carries none
// (a zero count for a stage); the live source is Machine.Workers.Ingest.
var defaultPools = poolSizes{hash: 4, extract: 2}

// resolvePools reads the machine-scoped worker counts (settings-owned), falling
// back to defaultPools for any stage left at zero.
func resolvePools(imp *Importer) poolSizes {
	pools := defaultPools
	ingest := imp.Machine.Workers.Ingest
	if ingest.Hash > 0 {
		pools.hash = ingest.Hash
	}
	if ingest.Extract > 0 {
		pools.extract = ingest.Extract
	}
	return pools
}

// resolveBatchSize reads the settings-owned WRITE batch size, falling back to
// defaultBatchSize when unset.
func resolveBatchSize(imp *Importer) int {
	if imp.Settings.ImportBatchSize > 0 {
		return imp.Settings.ImportBatchSize
	}
	return defaultBatchSize
}

// pipeline is the per-run state. Field ownership is by goroutine, which is what
// keeps most of it lock-free: SCAN owns visited/tallies/skipped; WRITE owns the
// added/updated/moved/duplicates counters; the run-level error slice is the one
// thing both touch, so it alone is mutex-guarded.
type pipeline struct {
	importer  *Importer
	target    Target
	fsys      fs.FS
	known     map[string]domain.FileStat
	sessionID string
	jobID     string
	pools     poolSizes
	batchSize int

	// SCAN-owned (read after the run drains).
	visited      map[string]struct{}
	unknownTally map[string]int
	ignoredTally map[string]int

	// WRITE-owned.
	addedCount, updatedCount, movedCount, duplicateCount, errorCount int
	missingCount                                                     int
	batchSeq                                                         int // commit counter; tags batch traces + item spans (fan-in)

	// cross-goroutine progress (atomics).
	total        atomic.Int64 // asset items emitted by SCAN
	done         atomic.Int64 // assets committed by WRITE
	walkDone     atomic.Bool
	skippedCount atomic.Int64 // SCAN increments; WRITE reads it for the mid-import count flush

	errorsMu  sync.Mutex // guards runErrors
	runErrors []ImportError
}

func newPipeline(importer *Importer, target Target, fsys fs.FS, known map[string]domain.FileStat, sessionID, jobID string) *pipeline {
	return &pipeline{
		importer:     importer,
		target:       target,
		fsys:         fsys,
		known:        known,
		sessionID:    sessionID,
		jobID:        jobID,
		pools:        resolvePools(importer),
		batchSize:    resolveBatchSize(importer),
		visited:      make(map[string]struct{}, len(known)),
		unknownTally: map[string]int{},
		ignoredTally: map[string]int{},
	}
}

// RunJob is Run with a job id stamped onto progress events (see Jobs). It loads
// the source's known-file map, opens an import session, runs the pipeline, fuses
// the walk-end missing diff, and finalizes the session. Only catastrophic
// failures return an error; per-file failures are DLQ rows.
func (imp *Importer) RunJob(ctx context.Context, jobID string, target Target, fsys fs.FS) (ImportResult, error) {
	if imp.Store == nil || imp.Imports == nil {
		panic("importer: Store and Imports must be set for the pipeline path")
	}
	// Fail fast on an unreadable root (macOS TCC on a /Volumes path) before opening
	// a session — and before the empty walk would mark every known asset missing.
	if err := preflightReadable(fsys, target.walkStart(), target.Name); err != nil {
		return ImportResult{}, err
	}
	known, err := imp.Reader.ListKnownFiles(ctx, target.VolumeID, target.WalkRoot)
	if err != nil {
		return ImportResult{}, err
	}
	sessionID, err := imp.Imports.Start(ctx, target.VolumeID, "import")
	if err != nil {
		return ImportResult{}, err
	}
	pipe := newPipeline(imp, target, fsys, known, sessionID, jobID)
	// The run root span: every item trace and the SCAN walk span nest under it,
	// so one import run reads as one waterfall. Ends after the session finalizes.
	ctx, runSpan := imp.Tracer.Start(ctx, "import.run",
		slog.String("volume", target.Name), slog.String("session", sessionID))
	imp.Log.Info("import started", "volume", target.Name, "known", len(known), "session", sessionID,
		"pools", fmt.Sprintf("hash=%d extract=%d", pipe.pools.hash, pipe.pools.extract))

	// Pre-count the tree so progress is determinate from the first event. On
	// success the count owns Total and SCAN stops incrementing it (walkDone gates
	// that); if the count fails, walkDone stays false and SCAN falls back to the
	// climbing "N / ?" denominator.
	if count, err := pipe.countAssets(ctx); err != nil {
		imp.Log.Warn("pre-count walk failed; progress total stays indeterminate until walk end", "err", err)
	} else {
		pipe.total.Store(count)
		pipe.walkDone.Store(true)
		pipe.emitProgress("scan")
		imp.Log.Info("pre-count complete", "volume", target.Name, "assets", count)
	}

	runErr := pipe.run(ctx)

	// Walk-end diff (reconcile fused into import): a known file no longer visited
	// is missing. Only on a COMPLETED walk — a canceled walk's visited set is
	// partial, so trusting it would wrongly mark live files missing.
	if runErr == nil {
		if err := pipe.markMissing(ctx); err != nil {
			imp.Log.Warn("walk-end missing diff failed", "err", err)
		}
	}

	result := pipe.result()
	result.SessionID = sessionID
	// Finalize the session even on cancel (the counts of committed work stand);
	// WithoutCancel so a canceled ctx doesn't abort the bookkeeping write.
	if err := imp.Imports.Finish(context.WithoutCancel(ctx), sessionID, pipe.sessionSnapshot()); err != nil {
		imp.Log.Warn("finalize session failed", "session", sessionID, "err", err)
	}

	runSpan.SetAttrs(slog.Int("added", result.Added), slog.Int("updated", result.Updated),
		slog.Int("skipped", result.Skipped), slog.Int("dups", result.Dups),
		slog.Int("missing", result.Missing), slog.Int("errors", pipe.errorCount))
	if runErr != nil {
		runSpan.Fail(runErr) // auto-classifies a canceled run as canceled, not error
	}
	runSpan.End()

	imp.Log.Info("import finished", "volume", target.Name, "session", sessionID,
		"added", result.Added, "updated", result.Updated, "moved", result.Moved,
		"skipped", result.Skipped, "dups", result.Dups, "missing", result.Missing,
		"errors", pipe.errorCount)
	return result, runErr
}

// run wires and closes every channel and owns every goroutine (one errgroup).
func (pipe *pipeline) run(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)

	scanOut := make(chan *pipelineItem, 8)
	hashOut := make(chan *pipelineItem, pipe.pools.hash*2)
	matchOut := make(chan *pipelineItem, 8)
	extractOut := make(chan *pipelineItem, pipe.pools.extract*2)

	group.Go(func() error { defer close(scanOut); return pipe.scan(ctx, scanOut) })
	fanStage(group, pipe.pools.hash, hashOut, func() error { return pipe.hash(ctx, scanOut, hashOut) })
	group.Go(func() error { defer close(matchOut); return pipe.match(ctx, hashOut, matchOut) })
	fanStage(group, pipe.pools.extract, extractOut, func() error { return pipe.extract(ctx, matchOut, extractOut) })
	group.Go(func() error { return pipe.write(ctx, extractOut) })

	return group.Wait()
}

// fanStage runs runStage on workerCount goroutines and closes out once all of
// them finish — the writer-side close that propagates shutdown downstream.
// Channels are still owned by run; this only closes, never creates.
func fanStage(group *errgroup.Group, workerCount int, out chan<- *pipelineItem, runStage func() error) {
	var waitGroup sync.WaitGroup
	waitGroup.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		group.Go(func() error { defer waitGroup.Done(); return runStage() })
	}
	group.Go(func() error { waitGroup.Wait(); close(out); return nil })
}

// emit sends downstream, unblocking on cancellation so no stage wedges on a full
// channel after the run is torn down.
func (pipe *pipeline) emit(ctx context.Context, out chan<- *pipelineItem, item *pipelineItem) error {
	if item.logger == nil {
		item.logger = pipe.importer.Log.With("asset", item.scanned.filename)
	}
	select {
	case out <- item:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// postCommit runs the ordered hooks after a batch commits. Grouping recompute is
// a deliberate no-op stub — sidecar_files rows are already written, so the
// grouping engine backfills cleanly when it lands. catalog-changed emission is a
// callback wired to Wails later; for now, count persistence and progress are the
// live hooks.
func (pipe *pipeline) postCommit(ctx context.Context) {
	// grouping recompute stub (no-op)
	pipe.persistCounts(ctx)
	pipe.emitProgress("write")
	// catalog-changed callback (wired later)
}

// persistCounts flushes the running tallies onto the session row after each batch,
// so a live viewer (dev harness, frontend) sees counts climb mid-import instead of
// only at Finish. Best-effort: a failed progress write is logged and ignored —
// Finish writes the authoritative final counts, and the import is idempotent, so a
// dropped mid-flight update costs nothing. Runs in the WRITE goroutine, so the
// WRITE-owned counters are read without synchronization; skippedCount is atomic
// because SCAN is still writing it.
func (pipe *pipeline) persistCounts(ctx context.Context) {
	session := &domain.ImportSession{
		Added:   pipe.addedCount,
		Updated: pipe.updatedCount,
		Moved:   pipe.movedCount,
		Skipped: int(pipe.skippedCount.Load()),
		Dups:    pipe.duplicateCount,
		Errors:  pipe.errorCount,
	}
	if err := pipe.importer.Imports.UpdateCounts(ctx, pipe.sessionID, session); err != nil {
		pipe.importer.Log.Debug("mid-import count update failed", "session", pipe.sessionID, "err", err)
	}
}

func (pipe *pipeline) emitProgress(stage string) {
	if pipe.importer.OnProgress == nil {
		return
	}
	pipe.importer.OnProgress(Progress{
		JobID:      pipe.jobID,
		Kind:       "import",
		Done:       int(pipe.done.Load()),
		Total:      int(pipe.total.Load()),
		TotalKnown: pipe.walkDone.Load(),
		Stage:      stage,
	})
}

func (pipe *pipeline) tally(item *pipelineItem) {
	pipe.errorCount += len(item.stageErrors)
	if item.isSidecar || item.rejected {
		return
	}
	switch item.verdict {
	case actionNew:
		pipe.addedCount++
	case actionReimport:
		pipe.updatedCount++
	case actionDuplicate:
		pipe.addedCount++
		pipe.duplicateCount++
	}
}

// markMissing flips online→missing for known assets whose paths the walk didn't
// visit. Per D20 the walk never auto-relinks or auto-merges a "move": a file that
// reappeared at a NEW path was already minted as a new asset + a pending review
// row, so here we simply mark the unvisited-but-known assets missing and leave the
// move/duplicate resolution to the user. A file that reappears at its ORIGINAL
// path is visited and restored via reimport, so it is never a candidate here.
func (pipe *pipeline) markMissing(ctx context.Context) error {
	pathStatuses, err := pipe.importer.Reader.ListPathsStatus(ctx, pipe.target.VolumeID, pipe.target.WalkRoot)
	if err != nil {
		return err
	}
	var candidateIDs []string
	for _, pathStatus := range pathStatuses {
		if pathStatus.FileStatus != domain.FileStatusOnline {
			continue
		}
		// visited is keyed by path_key (the NFC comparison form), so compare keys.
		if _, seen := pipe.visited[domain.PathKey(pathStatus.RelativePath)]; !seen {
			candidateIDs = append(candidateIDs, pathStatus.ID)
		}
	}
	if len(candidateIDs) == 0 {
		return nil
	}
	err = pipe.importer.Store.InTx(ctx, func(repos sqlite.Repos) error {
		for _, id := range candidateIDs {
			if err := repos.Assets.SetFileStatus(ctx, id, domain.FileStatusMissing); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	pipe.missingCount += len(candidateIDs)
	pipe.importer.Log.Info("marked assets missing (walk-end diff)", "volume", pipe.target.Name, "count", len(candidateIDs))
	return nil
}

// markGone is the single-path gone branch (the watcher-fed delete): a path that no
// longer exists on disk. Per D20 it simply marks the asset at that path missing —
// no delete-side merge, no move detection. If the content reappeared elsewhere it
// was minted as a new asset + a pending review row; the user resolves the move.
// Nothing known at the path, or a row already not online, is a no-op — so a
// duplicate gone-event never double-marks.
func (imp *Importer) markGone(ctx context.Context, target Target, relPath string) error {
	existing, err := imp.Reader.FindByVolumePath(ctx, target.VolumeID, relPath)
	if err != nil {
		return err
	}
	if existing == nil || existing.FileStatus != domain.FileStatusOnline {
		imp.Log.Debug("gone path is not a tracked online asset — nothing to do", "path", relPath)
		return nil
	}
	if err := imp.Obs.SetFileStatus(ctx, existing.ID, domain.FileStatusMissing); err != nil {
		return err
	}
	imp.Log.Info("marked asset missing", "volume", target.Name, "path", relPath, "asset", existing.ID)
	return nil
}

func (pipe *pipeline) addRunError(relativePath, stage string, err error) {
	pipe.importer.Log.Warn("file skipped after error", "path", relativePath, "stage", stage, "err", err)
	pipe.errorsMu.Lock()
	pipe.runErrors = append(pipe.runErrors, ImportError{Path: relativePath, Stage: stage, Err: err})
	pipe.errorsMu.Unlock()
}

func (pipe *pipeline) addItemError(item *pipelineItem, stage string, err error) {
	item.logger.Warn("file skipped after error", "path", item.scanned.relPath, "stage", stage, "err", err)
	pipe.errorsMu.Lock()
	pipe.runErrors = append(pipe.runErrors, ImportError{Path: item.scanned.relPath, Stage: stage, Err: err})
	pipe.errorsMu.Unlock()
}

func (pipe *pipeline) result() ImportResult {
	pipe.errorsMu.Lock()
	runErrors := pipe.runErrors
	pipe.errorsMu.Unlock()
	return ImportResult{
		Added:   pipe.addedCount,
		Updated: pipe.updatedCount,
		Moved:   pipe.movedCount,
		Skipped: int(pipe.skippedCount.Load()),
		Dups:    pipe.duplicateCount,
		Missing: pipe.missingCount,
		Errors:  runErrors,
	}
}

func (pipe *pipeline) sessionSnapshot() *domain.ImportSession {
	return &domain.ImportSession{
		Added:          pipe.addedCount,
		Updated:        pipe.updatedCount,
		Moved:          pipe.movedCount,
		Skipped:        int(pipe.skippedCount.Load()),
		Dups:           pipe.duplicateCount,
		Errors:         pipe.errorCount,
		SkippedUnknown: pipe.unknownTally,
		SkippedIgnored: pipe.ignoredTally,
	}
}
