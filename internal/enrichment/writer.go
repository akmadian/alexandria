package enrichment

import (
	"context"
	"log/slog"
	"time"

	"github.com/akmadian/alexandria/internal/sqlite"
)

// The writer is the engine's single catalog mutator — the one-cook rule
// (D28): every enrichment result, success or DLQ row, commits through this
// goroutine's batched transactions, so ingest's writer and this one take
// orderly turns at the WAL lock. It also owns the post-commit ordering
// contract: DB write → clear tracker bit → notify/emit — in that order, so a
// frontend invalidation can never re-fetch state older than what caused it.
// Its completion reports are what drive the dispatcher's edge emission.

const writeLull = 500 * time.Millisecond

func (e *Engine) runWriter(ctx context.Context) error {
	batch := make([]*jobResult, 0, writeBatchSize)
	timer := time.NewTimer(writeLull)
	timer.Stop()
	defer timer.Stop()
	batchSeq := 0

	flush := func(flushCtx context.Context) {
		e.commitBatch(flushCtx, batch, &batchSeq)
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
			// Commit the in-hand batch, then exit (WithoutCancel so the same
			// cancel can't abort it). Results still buffered in the channel are
			// dropped — their artifacts stay missing and the next open's scan
			// re-derives them, the same restart semantics as the tracker.
			flush(context.WithoutCancel(ctx))
			return nil
		case result := <-e.results:
			if len(batch) == 0 {
				timer.Reset(writeLull)
			}
			batch = append(batch, result)
			if len(batch) >= writeBatchSize {
				timer.Stop()
				flush(ctx)
			}
		case <-timer.C:
			flush(ctx)
		}
	}
}

// commitBatch writes one batch in a single transaction (applies + DLQ rows;
// skipped/canceled results ride along with no DB work), then runs the ordered
// post-commit steps.
func (e *Engine) commitBatch(ctx context.Context, batch []*jobResult, batchSeq *int) {
	if len(batch) == 0 {
		return
	}
	var databaseWork []*jobResult
	for _, result := range batch {
		if result.status == resultApplied || result.status == resultFailed {
			databaseWork = append(databaseWork, result)
		}
	}

	var commitErr error
	if len(databaseWork) > 0 {
		*batchSeq++
		// The batch is its own tiny trace (the fan-in recipe): a commit serving
		// N job traces belongs to none of them; batch_seq is the shared attr.
		_, batchSpan := e.tracer.Start(context.Background(), "enrichment.write-batch",
			slog.Int("items", len(databaseWork)), slog.Int("batch_seq", *batchSeq))
		commitErr = e.store.InTx(ctx, func(repos sqlite.Repos) error {
			for _, result := range databaseWork {
				if err := e.writeResult(ctx, repos, result); err != nil {
					return err
				}
			}
			return nil
		})
		if commitErr != nil {
			batchSpan.Fail(commitErr)
			// The tx is poisoned by the first failing statement; the batch's
			// artifacts stay missing and the rescan scheduled below re-derives
			// every one — idempotency is the recovery, same as ingest.
			// ponytail: no per-item re-commit on tx failure. Add it if a real
			// workload starts losing whole batches to one bad apply.
			e.log.Error("enrichment: batch commit failed", "items", len(databaseWork), "err", commitErr)
		}
		batchSpan.End()
	}

	// Post-commit, in contract order: the DB write above → clear bits → emit.
	applied, failed, skipped := 0, 0, 0
	completions := make([]completion, 0, len(batch))
	var committed []JobKey
	for _, result := range batch {
		e.tracker.ClearRunning(result.key.AssetID, result.bit)
		completions = append(completions, completion{
			key:        result.key,
			applied:    result.status == resultApplied && commitErr == nil,
			extension:  result.assetExtension,
			ingestedAt: result.assetIngestedAt,
		})
		e.endJobSpan(result, commitErr, *batchSeq)
		switch result.status {
		case resultApplied:
			if commitErr == nil {
				applied++
				committed = append(committed, result.key)
			}
		case resultFailed:
			failed++
		case resultSkipped, resultCanceled:
			skipped++
		}
	}
	select {
	case e.completions <- completions:
	case <-e.runCtx.Done(): // shutdown flush: the dispatcher may be gone
	}
	if commitErr != nil {
		// The completions above retired the batch from the pending ledger, so
		// without this nudge the lost artifacts would sit missing until an
		// external scan — the rescan IS the recovery, so schedule it ourselves.
		// ponytail: a PERSISTENT commit failure (disk full) makes this churn a
		// produce→fail→rescan cycle, paced by the batch lull and Error-logged
		// every batch; add backoff only if a real workload ever exhibits it.
		select {
		case e.scanRequests <- struct{}{}:
		default:
		}
	}
	if e.onBatchCommitted != nil && len(committed) > 0 {
		e.onBatchCommitted(committed)
	}
	e.log.Debug("enrichment: batch committed", "applied", applied, "failed", failed, "skipped", skipped)
}

// writeResult applies one result inside the batch transaction. An applied
// artifact also clears its DLQ row — a failed state must not outlive the
// artifact that supersedes it.
func (e *Engine) writeResult(ctx context.Context, repos sqlite.Repos, result *jobResult) error {
	switch result.status {
	case resultApplied:
		if err := result.apply(ctx, repos.Assets); err != nil {
			return err
		}
		return repos.Enrichment.ClearFailure(ctx, result.key.AssetID, result.key.Kind)
	case resultFailed:
		return repos.Enrichment.LogFailure(ctx, result.key.AssetID, result.key.Kind, result.reasonCode, result.message)
	default:
		return nil
	}
}

// endJobSpan closes the job's root span with its final disposition: outcome
// attr plus batch_seq for the fan-in join against enrichment.write-batch.
func (e *Engine) endJobSpan(result *jobResult, commitErr error, batchSeq int) {
	span := result.span
	switch result.status {
	case resultApplied:
		span.SetAttrs(slog.Int("batch_seq", batchSeq), slog.String("outcome", "applied"))
		if commitErr != nil {
			span.Fail(commitErr)
		}
	case resultFailed:
		span.SetAttrs(slog.Int("batch_seq", batchSeq), slog.String("outcome", "failed"),
			slog.String("reason", result.reasonCode))
		span.Fail(result.err)
	case resultSkipped:
		span.SetAttrs(slog.String("outcome", "skipped"))
	case resultCanceled:
		span.SetAttrs(slog.String("outcome", "canceled"))
		span.Fail(context.Canceled) // auto-classifies as canceled, not error
	}
	span.End()
}
