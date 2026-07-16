package seam

import (
	"github.com/charmbracelet/log"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/settings"
)

// This file is the seam control surface of the enrichment engine (task 21): the
// write-side dials the frontend drives. Read-side visibility is pull-decorated
// onto asset rows (AssetService), never streamed per asset (D28). The engine is
// held behind a structural interface, so the seam never imports the engine
// package — the composition root passes the concrete *enrichment.Engine.

// enrichmentController is the engine's control slice: the user-facing dials.
// *enrichment.Engine satisfies it structurally.
type enrichmentController interface {
	PauseAll()
	ResumeAll()
	PauseKind(kind string)
	ResumeKind(kind string)
	SetEffort(level string)
	Hint(assetIDs []string)
}

// effortStore persists the effort dial to machine.json so it survives restart —
// the durable half of "persists through settings AND applies to the budget".
type effortStore interface {
	SetEnrichmentEffort(level string) error
}

// EnrichmentEngineService controls the background ENGINE — not the enrichment
// data. It exposes pause/resume (global + per kind), the effort dial (applied live
// and persisted), and the viewport priority hint (hint-never-truth, D28). The
// enrichment values and state are properties of the asset and read through the
// asset path (AssetService decoration), never collected here — this service holds
// engine verbs only, the same way ImportService controls import jobs while the
// imported assets read through AssetService.
type EnrichmentEngineService struct {
	engine enrichmentController
	effort effortStore
}

// NewEnrichmentEngineService binds the control surface over the engine and the effort
// persistence. Constructed after the engine (controls are runtime calls, so
// there is no construction cycle — the aggregate-event hook is the free function
// EmitEnrichmentBatch, wired separately).
func NewEnrichmentEngineService(engine enrichmentController, effort effortStore) *EnrichmentEngineService {
	return &EnrichmentEngineService{engine: engine, effort: effort}
}

func (s *EnrichmentEngineService) PauseAll() {
	log.Info("seam: enrichment paused (all)")
	s.engine.PauseAll()
}

func (s *EnrichmentEngineService) ResumeAll() {
	log.Info("seam: enrichment resumed (all)")
	s.engine.ResumeAll()
}

func (s *EnrichmentEngineService) PauseKind(kind string) {
	log.Info("seam: enrichment kind paused", "kind", kind)
	s.engine.PauseKind(kind)
}

func (s *EnrichmentEngineService) ResumeKind(kind string) {
	log.Info("seam: enrichment kind resumed", "kind", kind)
	s.engine.ResumeKind(kind)
}

// SetEffort persists the dial then applies it live. Persist-first so a crash
// between the two leaves the durable intent (and the on-open budget reflects it);
// an unknown level is rejected before either side effect (the engine would
// silently ignore it, but a live caller deserves a real error).
func (s *EnrichmentEngineService) SetEffort(level string) error {
	switch level {
	case settings.EffortPaused, settings.EffortLow, settings.EffortNormal, settings.EffortFull:
	default:
		return normalizeError(&domain.ValidationError{Field: "effort", Message: "unknown effort level " + level})
	}
	if err := s.effort.SetEnrichmentEffort(level); err != nil {
		log.Error("seam: persist effort failed", "level", level, "err", err)
		return normalizeError(err)
	}
	log.Info("seam: enrichment effort set", "level", level)
	s.engine.SetEffort(level)
	return nil
}

// Hint replaces the viewport priority set (hint-never-truth, D28): the engine
// reorders its hot lane, never changes what work exists, so a stale hint degrades
// to suboptimal order, never wrong data. The frontend debounces; replace-wholesale.
func (s *EnrichmentEngineService) Hint(assetIDs []string) {
	log.Debug("seam: enrichment viewport hint", "count", len(assetIDs))
	s.engine.Hint(assetIDs)
}

const (
	// enrichmentJobID / enrichmentJobKind name the convergent lane in the one Job
	// envelope (C9). It has no run identity (D28), so one stable synthetic "job"
	// represents the whole backlog; done/total stay 0 and the queue depth is the
	// real signal.
	enrichmentJobID   = "enrichment"
	enrichmentJobKind = "enrich"
)

// EmitEnrichmentBatch emits the aggregate events after an enrichment writer batch
// commits (task 21): catalog/changed for TanStack invalidation, plus one
// jobs/progress tick carrying the per-kind backlog. Batch cadence is the natural
// throttle (C8/C9: aggregate events only, never per-asset). The composition root
// wires it as the engine's OnBatchCommitted hook, capturing the engine's
// QueueDepths to pass here — which is why this is a free function, not a method
// needing the engine at construction. A zero total means the backlog drained
// (state done); the ordering contract (DB write → clear bit → emit) holds because
// OnBatchCommitted fires after the tracker bits clear.
//
// The depth is a display signal, not an exact count: the hook fires on the writer
// goroutine, but the just-committed jobs are retired from the dispatcher's count
// only when it drains their completions (slightly later), so a tick can over-report
// by up to one batch and self-corrects on the next. That is fine for a "how much
// left" indicator (D28: a confused count degrades display, never correctness).
func EmitEnrichmentBatch(emitter Emitter, depths map[string]int) {
	if emitter == nil {
		return
	}
	emitter.Emit(EventCatalogChanged, CatalogChange{Scope: ScopeAssets})
	state := JobStateRunning
	if totalDepth(depths) == 0 {
		state = JobStateDone
	}
	emitter.Emit(EventJobProgress, JobProgress{
		JobID:      enrichmentJobID,
		Kind:       enrichmentJobKind,
		Label:      jobLabelKey(enrichmentJobKind),
		State:      state,
		QueueDepth: depths,
	})
}

func totalDepth(depths map[string]int) int {
	total := 0
	for _, count := range depths {
		total += count
	}
	return total
}
