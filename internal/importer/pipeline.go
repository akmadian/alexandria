package importer

import (
	"context"
	"fmt"
	"io/fs"
	"sync"
	"sync/atomic"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/sqlite"
	"golang.org/x/sync/errgroup"
)

// This file is the pipeline's plumbing: the channel wiring, the per-run state,
// and the run-level lifecycle (start a session, run the stages, fuse the
// walk-end missing diff, finalize). The six stages themselves live one-per-file
// in stage_*.go; the item they thread through is in item.go.
//
//	walk ─► SCAN ─► HASH ─► MATCH ─► EXTRACT ─► THUMB ─► WRITE ─► post-commit
//	       1 grtn   pool     1        pool       pool     1 grtn
//
// All channels are created, wired, and closed in ONE function (run). Stages are
// plain methods taking directional channels; they never make channels. Bounded
// channels make blocking sends the backpressure. MATCH is a singleton so the
// identity matrix reads a serializable view of the catalog it is mutating; WRITE
// is a singleton because SQLite is single-writer — the goroutine IS the batch
// point.

const (
	writeBatchSize = 50
	writeLull      = 500 * time.Millisecond
)

// moveHealWindow bounds the delete-side merge (impl/05): only a duplicate minted
// within this window of an asset going missing is treated as the same file having
// "moved" (copy-then-delete by an external app), not an unrelated copy.
// ponytail: fixed 10min heuristic; widen or key off the import session if real
// moves ever slip past it.
const moveHealWindow = 10 * time.Minute

// poolSizes are the per-stage worker counts. Hardcoded defaults for now; the
// machine.json config that tunes them per host arrives in a later milestone.
type poolSizes struct{ hash, extract, thumb int }

var defaultPools = poolSizes{hash: 4, extract: 2, thumb: 2}

// resolvePools applies the Importer's per-field overrides on top of the defaults
// (a zero override keeps the default).
func resolvePools(imp *Importer) poolSizes {
	pools := defaultPools
	if imp.HashWorkers > 0 {
		pools.hash = imp.HashWorkers
	}
	if imp.ExtractWorkers > 0 {
		pools.extract = imp.ExtractWorkers
	}
	if imp.ThumbWorkers > 0 {
		pools.thumb = imp.ThumbWorkers
	}
	return pools
}

// pipeline is the per-run state. Field ownership is by goroutine, which is what
// keeps most of it lock-free: SCAN owns visited/tallies/skipped; WRITE owns the
// added/updated/moved/duplicates counters; the run-level error slice is the one
// thing both touch, so it alone is mutex-guarded.
type pipeline struct {
	importer  *Importer
	source    *domain.Source
	fsys      fs.FS
	known     map[string]domain.FileStat
	sessionID string
	jobID     string
	pools     poolSizes

	// SCAN-owned (read after the run drains).
	visited      map[string]struct{}
	unknownTally map[string]int
	ignoredTally map[string]int
	skippedCount int

	// WRITE-owned.
	addedCount, updatedCount, movedCount, duplicateCount, errorCount int
	missingCount                                                     int

	// cross-goroutine progress (atomics).
	total    atomic.Int64 // asset items emitted by SCAN
	done     atomic.Int64 // assets committed by WRITE
	walkDone atomic.Bool

	errorsMu  sync.Mutex // guards runErrors
	runErrors []ImportError
}

func newPipeline(importer *Importer, source *domain.Source, fsys fs.FS, known map[string]domain.FileStat, sessionID, jobID string) *pipeline {
	return &pipeline{
		importer:     importer,
		source:       source,
		fsys:         fsys,
		known:        known,
		sessionID:    sessionID,
		jobID:        jobID,
		pools:        resolvePools(importer),
		visited:      make(map[string]struct{}, len(known)),
		unknownTally: map[string]int{},
		ignoredTally: map[string]int{},
	}
}

// RunJob is Run with a job id stamped onto progress events (see Jobs). It loads
// the source's known-file map, opens an import session, runs the pipeline, fuses
// the walk-end missing diff, and finalizes the session. Only catastrophic
// failures return an error; per-file failures are DLQ rows.
func (imp *Importer) RunJob(ctx context.Context, jobID string, source *domain.Source, fsys fs.FS) (ImportResult, error) {
	if imp.Store == nil || imp.Imports == nil {
		panic("importer: Store and Imports must be set for the pipeline path")
	}
	known, err := imp.Reader.ListKnownFiles(ctx, source.ID)
	if err != nil {
		return ImportResult{}, err
	}
	sessionID, err := imp.Imports.Start(ctx, source.ID, "import")
	if err != nil {
		return ImportResult{}, err
	}
	pipe := newPipeline(imp, source, fsys, known, sessionID, jobID)
	imp.Log.Info("import started", "source", source.Name, "known", len(known), "session", sessionID,
		"pools", fmt.Sprintf("hash=%d extract=%d thumb=%d", pipe.pools.hash, pipe.pools.extract, pipe.pools.thumb))

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

	imp.Log.Info("import finished", "source", source.Name, "session", sessionID,
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
	thumbOut := make(chan *pipelineItem, pipe.pools.thumb*2)

	group.Go(func() error { defer close(scanOut); return pipe.scan(ctx, scanOut) })
	fanStage(group, pipe.pools.hash, hashOut, func() error { return pipe.hash(ctx, scanOut, hashOut) })
	group.Go(func() error { defer close(matchOut); return pipe.match(ctx, hashOut, matchOut) })
	fanStage(group, pipe.pools.extract, extractOut, func() error { return pipe.extract(ctx, matchOut, extractOut) })
	fanStage(group, pipe.pools.thumb, thumbOut, func() error { return pipe.thumb(ctx, extractOut, thumbOut) })
	group.Go(func() error { return pipe.write(ctx, thumbOut) })

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
// callback wired to Wails later; for now, progress is the only live hook.
func (pipe *pipeline) postCommit() {
	// grouping recompute stub (no-op)
	pipe.emitProgress("write")
	// catalog-changed callback (wired later)
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
	case actionMove:
		pipe.movedCount++
	case actionDuplicate:
		pipe.addedCount++
		pipe.duplicateCount++
	}
}

// markMissing flips online→missing for known assets whose paths the walk didn't
// visit. It re-reads status AFTER the run so a relinked asset (path updated
// mid-run) shows its new, visited path and is correctly left alone.
//
// Before marking a candidate missing it tries a delete-side merge (impl/05): the
// file may not be gone, just copy-then-deleted to a new path that this same walk
// already ingested as a fresh duplicate. If so, that duplicate is absorbed and
// the identity stays online — so nothing is marked missing for a plain "move".
func (pipe *pipeline) markMissing(ctx context.Context) error {
	pathStatuses, err := pipe.importer.Reader.ListPathsStatus(ctx, pipe.source.ID)
	if err != nil {
		return err
	}
	var candidateIDs []string
	for _, pathStatus := range pathStatuses {
		if pathStatus.FileStatus != domain.FileStatusOnline {
			continue
		}
		if _, seen := pipe.visited[pathStatus.RelativePath]; !seen {
			candidateIDs = append(candidateIDs, pathStatus.ID)
		}
	}
	if len(candidateIDs) == 0 {
		return nil
	}
	mintedAfter := time.Now().Add(-moveHealWindow)
	var healed, missing int
	err = pipe.importer.Store.InTx(ctx, func(repos sqlite.Repos) error {
		healed, missing = 0, 0 // reset per attempt (InTx may retry)
		for _, id := range candidateIDs {
			ok, err := pipe.importer.healMovedAway(ctx, repos, pipe.source.ID, id, mintedAfter)
			if err != nil {
				return err
			}
			if ok {
				healed++
				continue
			}
			if err := repos.Assets.SetFileStatus(ctx, id, domain.FileStatusMissing); err != nil {
				return err
			}
			missing++
		}
		return nil
	})
	if err != nil {
		return err
	}
	pipe.missingCount += missing
	pipe.movedCount += healed
	return nil
}

// markGone is the single-path gone branch (impl/05 corrected model): a path the
// watcher fed that no longer exists on disk. It performs the SAME action as the
// walk-end diff — mark the asset at that path missing, attempting a delete-side
// merge first (a copy-then-delete "move" absorbs the freshly-minted copy and
// stays online). Nothing known at the path, or a row that is already not online,
// is a no-op — so a duplicate gone-event never double-marks.
func (imp *Importer) markGone(ctx context.Context, source *domain.Source, relPath string) error {
	existing, err := imp.Reader.FindBySourcePath(ctx, source.ID, relPath)
	if err != nil {
		return err
	}
	if existing == nil || existing.FileStatus != domain.FileStatusOnline {
		return nil
	}
	mintedAfter := time.Now().Add(-moveHealWindow)
	return imp.Store.InTx(ctx, func(repos sqlite.Repos) error {
		healed, err := imp.healMovedAway(ctx, repos, source.ID, existing.ID, mintedAfter)
		if err != nil || healed {
			return err
		}
		return repos.Assets.SetFileStatus(ctx, existing.ID, domain.FileStatusMissing)
	})
}

// healMovedAway implements impl/05's delete-side merge. A going-missing asset may
// have been copy-then-deleted to a new path an external app chose; if that copy
// was just ingested as a fresh, still-unjudged duplicate, adopt its path onto
// THIS identity (preserving all judgments and history) and delete the throwaway
// duplicate row. The zero-judgment guard (in FindMoveHealCandidate) is what makes
// this always safe — a copy the user has already rated is never absorbed. Returns
// true if a merge happened (the identity stays online), false to fall through to
// marking missing. Shared by the walk-end diff (markMissing) and the single-path
// gone branch (markGone), so both heal a "move" identically.
func (imp *Importer) healMovedAway(ctx context.Context, repos sqlite.Repos, sourceID, missingID string, mintedAfter time.Time) (bool, error) {
	gone, err := repos.Assets.Get(ctx, missingID)
	if err != nil {
		return false, err
	}
	young, err := repos.Assets.FindMoveHealCandidate(ctx, gone.PartialHash, gone.SizeBytes, gone.Filename, gone.ID, mintedAfter)
	if err != nil || young == nil {
		return false, err
	}
	// Delete the throwaway identity first — the (source_id, relative_path) unique
	// index forbids two live rows at the same path, so the old identity can only
	// adopt young's path once young's row is gone (FK cascade drops its dup rows).
	if err := repos.Assets.DeleteByID(ctx, young.ID); err != nil {
		return false, err
	}
	if err := repos.Assets.UpdatePath(ctx, gone.ID, sourceID, young.RelativePath); err != nil {
		return false, err
	}
	imp.Log.Info("delete-side merge: absorbed moved copy",
		"identity", gone.ID, "from", gone.RelativePath, "to", young.RelativePath)
	return true, nil
}

func (pipe *pipeline) addRunError(relativePath, stage string, err error) {
	pipe.importer.Log.Warn("file skipped after error", "path", relativePath, "stage", stage, "err", err)
	pipe.errorsMu.Lock()
	pipe.runErrors = append(pipe.runErrors, ImportError{Path: relativePath, Stage: stage, Err: err})
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
		Skipped: pipe.skippedCount,
		Dups:    pipe.duplicateCount,
		Missing: pipe.missingCount,
		Errors:  runErrors,
	}
}

func (pipe *pipeline) sessionSnapshot() domain.ImportSession {
	return domain.ImportSession{
		Added:          pipe.addedCount,
		Updated:        pipe.updatedCount,
		Moved:          pipe.movedCount,
		Skipped:        pipe.skippedCount,
		Dups:           pipe.duplicateCount,
		Errors:         pipe.errorCount,
		SkippedUnknown: pipe.unknownTally,
		SkippedIgnored: pipe.ignoredTally,
	}
}
