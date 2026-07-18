package enrichment

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/gospan"
)

// Workers are a definition's parallelism, and every worker at every node runs
// the SAME template — the node machinery is abstract and identical; only the
// definition's data (Produce, policies) differs:
//
//	pop job → fetch asset → recheck eligibility (catalog truth) → I/O token
//	→ weighted budget → Produce → result to writer → (writer: commit, clear
//	bit, edge-emit)
//
// Production is the slow, parallel half; the catalog mutation happens later
// in the single writer goroutine — a worker never touches the DB write path.

// resultStatus classifies how a job ended.
type resultStatus int

const (
	resultApplied  resultStatus = iota // artifact produced; apply pending commit
	resultFailed                       // producer failed; a DLQ row records why
	resultSkipped                      // ineligible at dispatch (already present, gone, prereq missing)
	resultCanceled                     // engine shutdown mid-flight; the rescan re-derives it
)

// jobResult is one finished job on its way through the writer. It carries the
// asset facts (extension, ingest time) edge emission needs so the dispatcher
// never re-reads the catalog to enqueue dependents.
type jobResult struct {
	key              JobKey
	bit              KindSet
	status           resultStatus
	apply            ApplyFunc
	reasonCode       string
	message          string
	err              error
	span             *gospan.Span // the enrichment.<kind> root; ended after commit
	assetExtension   string
	assetIngestedAt  time.Time
	assetPartialHash string // identity at dispatch; the writer drops the apply if a reimport changed it
}

// errStalled is the watchdog's cancel cause: the producer went silent longer
// than its budget (distinct from a wall-clock timeout in the DLQ taxonomy).
var errStalled = errors.New("enrichment: producer stalled past its watchdog budget")

func (e *Engine) runWorker(ctx context.Context, definition *JobDefinition) error {
	for {
		reply := make(chan *job, 1)
		select {
		case e.requests <- workRequest{kind: definition.Kind, reply: reply}:
		case <-ctx.Done():
			return nil
		}
		select {
		case assignment := <-reply:
			e.runJob(ctx, assignment)
		case <-ctx.Done():
			return nil
		}
	}
}

// runJob executes one assignment end to end. The root span starts here and
// ends in the writer after commit — the gap between produce's end and the
// root's end IS the await-commit time, the same reading recipe as import.
func (e *Engine) runJob(ctx context.Context, assignment *job) {
	definition := assignment.definition
	rootCtx, rootSpan := e.tracer.Start(ctx, "enrichment."+definition.Kind,
		slog.String("asset", assignment.assetID), slog.Bool("hinted", assignment.hinted))
	result := &jobResult{
		key:  JobKey{AssetID: assignment.assetID, Kind: definition.Kind},
		bit:  e.kindBits[definition.Kind],
		span: rootSpan,
	}

	asset, err := e.reader.Get(ctx, assignment.assetID)
	if err != nil || asset == nil {
		e.finishSkipped(ctx, result, "asset unavailable", err)
		return
	}
	result.assetExtension = asset.Extension
	result.assetIngestedAt = asset.IngestedAt
	result.assetPartialHash = asset.PartialHash
	// Applicability is part of the recheck: hints enqueue speculatively, so a
	// definition must never produce for a type it didn't register for — that
	// would mint a DLQ "failed" for an asset that is simply not applicable.
	if !e.applicableByKind[definition.Kind][asset.Extension] {
		e.finishSkipped(ctx, result, "definition not applicable to type", nil)
		return
	}
	eligible, err := e.enrichmentRepo.MissingAndEligible(ctx, &sqlite.EligibilityProbe{
		AssetID:             asset.ID,
		Kind:                definition.Kind,
		ArtifactColumn:      definition.ArtifactColumn,
		PrerequisiteColumns: e.prerequisiteColumns[definition.Kind],
		MaxAttempts:         MaxAttempts,
	})
	if err != nil {
		e.finishSkipped(ctx, result, "eligibility check failed", err)
		return
	}
	if !eligible {
		e.finishSkipped(ctx, result, "no longer eligible", nil)
		return
	}

	// Admission: the source's I/O token first (cheap, plentiful), then the
	// weighted CPU budget — never hold scarce CPU tokens while queuing on disk.
	if err := e.readTokens.acquire(ctx, asset.SourceID); err != nil {
		result.status = resultCanceled
		e.sendResult(ctx, result)
		return
	}
	tokens := int64(1)
	if definition.Weight != nil {
		tokens = definition.Weight(asset.SizeBytes)
	}
	held, err := e.budget.acquire(ctx, tokens)
	if err != nil {
		e.readTokens.release(asset.SourceID)
		result.status = resultCanceled
		e.sendResult(ctx, result)
		return
	}
	rootSpan.SetAttrs(slog.Int64("tokens", held), slog.Int64("size_bytes", asset.SizeBytes))

	e.tracker.SetRunning(asset.ID, result.bit)
	produceCtx, finish, heartbeat := jobContext(rootCtx, definition, asset.SizeBytes, asset.FileType)
	_, produceSpan := e.tracer.Start(rootCtx, "enrichment.produce")
	apply, produceErr := definition.Produce(produceCtx, asset, heartbeat)
	e.classify(result, produceCtx, apply, produceErr)
	finish()
	if produceErr != nil {
		produceSpan.Fail(produceErr)
	}
	produceSpan.End()
	e.budget.release(held)
	e.readTokens.release(asset.SourceID)

	switch result.status {
	case resultApplied:
		e.log.Debug("enrichment: artifact produced", "kind", definition.Kind, "asset", asset.ID, "hinted", assignment.hinted)
	case resultFailed:
		e.log.Warn("enrichment: producer failed", "kind", definition.Kind, "asset", asset.ID,
			"reason", result.reasonCode, "err", produceErr)
	case resultCanceled, resultSkipped: // narrated at shutdown/skip sites
	}
	e.sendResult(ctx, result)
}

// classify maps a producer outcome onto a result: applied, canceled (engine
// shutdown — never a DLQ row; the rescan re-derives the work), or failed with
// a DLQ reason code (stalled / timeout / the producer's own Fail code /
// produce_failed).
func (e *Engine) classify(result *jobResult, produceCtx context.Context, apply ApplyFunc, produceErr error) {
	if produceErr == nil {
		if apply == nil {
			result.status = resultFailed
			result.reasonCode = "producer_defect"
			result.message = "producer returned no apply and no error"
			result.err = errors.New(result.message)
			return
		}
		result.status = resultApplied
		result.apply = apply
		return
	}
	result.err = produceErr
	result.message = produceErr.Error()
	var reasonError *ReasonError
	switch {
	case errors.Is(context.Cause(produceCtx), errStalled):
		result.status = resultFailed
		result.reasonCode = "stalled"
	case errors.Is(produceErr, context.DeadlineExceeded):
		result.status = resultFailed
		result.reasonCode = "timeout"
	case errors.Is(produceErr, context.Canceled):
		result.status = resultCanceled // engine shutdown, not a failure
	case errors.As(produceErr, &reasonError):
		result.status = resultFailed
		result.reasonCode = reasonError.ReasonCode
	default:
		result.status = resultFailed
		result.reasonCode = "produce_failed"
	}
}

// finishSkipped closes out a job that never reached production. A skip is
// normal traffic (speculative hints, stale queues, raced scans), so it is
// Debug, not Warn.
func (e *Engine) finishSkipped(ctx context.Context, result *jobResult, why string, err error) {
	if ctx.Err() != nil {
		result.status = resultCanceled
		e.sendResult(ctx, result)
		return
	}
	result.status = resultSkipped
	e.log.Debug("enrichment: job skipped at dispatch", "kind", result.key.Kind, "asset", result.key.AssetID, "why", why, "err", err)
	e.sendResult(ctx, result)
}

// sendResult forwards to the writer; on shutdown the result is dropped and the
// tracker bit cleared here instead (the writer may already be flushing out).
func (e *Engine) sendResult(ctx context.Context, result *jobResult) {
	select {
	case e.results <- result:
	case <-ctx.Done():
		e.tracker.ClearRunning(result.key.AssetID, result.bit)
		result.span.Fail(context.Canceled)
		result.span.End()
	}
}

// jobContext applies the definition's time budget to a producer context.
// Wall-clock definitions get a plain deadline; watchdog definitions get a
// stall timer the heartbeat resets — elapsed time is meaningless for a
// two-hour transcode, silence is not. finish releases the timer/context;
// heartbeat is never nil.
func jobContext(ctx context.Context, definition *JobDefinition, sizeBytes int64, fileType domain.FileType) (produceCtx context.Context, finish func(), heartbeat func()) {
	timeBudget := definition.TimeoutPolicy(sizeBytes, fileType)
	if timeBudget <= 0 {
		return ctx, func() {}, func() {}
	}
	if definition.Watchdog {
		watchdogCtx, cancel := context.WithCancelCause(ctx)
		timer := time.AfterFunc(timeBudget, func() { cancel(errStalled) })
		return watchdogCtx,
			func() { timer.Stop(); cancel(nil) },
			func() { timer.Reset(timeBudget) }
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, timeBudget)
	return deadlineCtx, cancel, func() {}
}
