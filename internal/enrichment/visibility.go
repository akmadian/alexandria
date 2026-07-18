package enrichment

import (
	"context"

	"github.com/akmadian/alexandria/internal/domain"
)

// This file is the engine's seam-facing read surface (task 21): per-asset
// transient + failed state expressed as KIND NAMES, never the internal
// registry-order bitmask. The seam decorates asset responses from these, and the
// bit layout stays an engine implementation detail (D28: hints/visibility never
// leak the engine's internals into the contract).

// RunningKinds resolves a page of assets to the kinds currently in flight for
// each — one tracker lock for the whole page (RunningBatch), then the bitmask →
// kind-name reverse. Sparse: an idle asset is absent from the result. Kinds come
// out in scanOrder (deterministic) so the decorated row is stable.
func (e *Engine) RunningKinds(assetIDs []string) map[string][]domain.EnrichmentKind {
	batch := e.tracker.RunningBatch(assetIDs)
	if len(batch) == 0 {
		return nil
	}
	running := make(map[string][]domain.EnrichmentKind, len(batch))
	for assetID, mask := range batch {
		var kinds []domain.EnrichmentKind
		for _, kind := range e.scanOrder {
			if mask&e.kindBits[kind] != 0 {
				kinds = append(kinds, domain.EnrichmentKind(kind))
			}
		}
		running[assetID] = kinds
	}
	return running
}

// FailedKinds resolves a page of assets to the kinds terminally failed for each
// (DLQ row at the attempt ceiling) — the "not an eternal spinner" state (D25). A
// non-exhausted DLQ row is still being retried, so it is NOT failed here; it reads
// as pending until the scan gives up. Sparse, like RunningKinds.
func (e *Engine) FailedKinds(ctx context.Context, assetIDs []string) (map[string][]domain.EnrichmentKind, error) {
	if !e.failuresPossible.Load() {
		return nil, nil // no failure ever recorded — the DLQ query would return nothing
	}
	exhausted, err := e.enrichmentRepo.ExhaustedKinds(ctx, assetIDs, MaxAttempts)
	if err != nil {
		return nil, err
	}
	if len(exhausted) == 0 {
		return nil, nil
	}
	failed := make(map[string][]domain.EnrichmentKind, len(exhausted))
	for assetID, kinds := range exhausted {
		converted := make([]domain.EnrichmentKind, len(kinds))
		for index, kind := range kinds {
			converted[index] = domain.EnrichmentKind(kind)
		}
		failed[assetID] = converted
	}
	return failed, nil
}
