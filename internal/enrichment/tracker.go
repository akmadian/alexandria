package enrichment

import "sync"

// KindSet is a bitmask of job kinds currently running for one asset — one bit
// per registry row, assigned by row order at engine construction. Transient
// and in-memory by design (D28): a process restart empties the map, and that
// is correct — queued/running are the only non-derived states in the artifact
// state machine, and a rescan re-derives the work. All bitwise operations stay
// inside this file; callers pass KindSet values they got from the engine.
type KindSet uint64

// Tracker is the in-flight ledger the seam decorates asset responses from
// (task 21): data present = ready, bit set = enriching, DLQ row = failed,
// none = pending. Write ordering contract (enforced by the writer goroutine):
// DB write → ClearRunning → emit, so an invalidation never races a stale read.
type InFlightTracker struct {
	mu      sync.RWMutex
	running map[string]KindSet
}

func NewInFlightTracker() *InFlightTracker {
	return &InFlightTracker{running: make(map[string]KindSet)}
}

// SetRunning marks stage in-flight for the asset.
func (t *InFlightTracker) SetRunning(assetID string, stage KindSet) {
	t.mu.Lock()
	t.running[assetID] |= stage
	t.mu.Unlock()
}

// ClearRunning clears stage for the asset, dropping the entry at zero so the
// map stays proportional to in-flight work, not catalog size.
func (t *InFlightTracker) ClearRunning(assetID string, stage KindSet) {
	t.mu.Lock()
	remaining := t.running[assetID] &^ stage
	if remaining == 0 {
		delete(t.running, assetID)
	} else {
		t.running[assetID] = remaining
	}
	t.mu.Unlock()
}

// Running returns the asset's in-flight bitmask (zero when idle).
func (t *InFlightTracker) Running(assetID string) KindSet {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.running[assetID]
}

// RunningBatch resolves a page of assets in ONE lock acquisition — the shape
// the seam's per-page decoration needs. The result is sparse: idle assets are
// absent, so an idle page allocates nothing.
func (t *InFlightTracker) RunningBatch(assetIDs []string) map[string]KindSet {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var batch map[string]KindSet
	for _, assetID := range assetIDs {
		if stage, inFlight := t.running[assetID]; inFlight {
			if batch == nil {
				batch = make(map[string]KindSet)
			}
			batch[assetID] = stage
		}
	}
	return batch
}
