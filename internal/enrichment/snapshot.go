package enrichment

import (
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

// Snapshot is the live engine state for the debug surface (D28 commitment #4,
// task 22): aggregate and domain-vocabulary (asset / kind / artifact / queue) by
// construction — never a generic dump, the pprof anti-lesson. It is the JSON
// contract the dev-harness page renders now and the in-app dev corner will
// consume later, so the shape IS the contract (json tags are the wire).
//
// Everything here is HARVESTED from state the engine already holds — gauges, not
// distributions. Per-kind duration histograms, per-(kind, asset) token cost, and
// any slice-and-dice are gospan's job (D30), read post-hoc from the trace file;
// DEFERRED §14 records the line.
type Snapshot struct {
	// Effort is the current dial level (paused | low | normal | full). Distinct
	// from Paused: Effort == "paused" is the dial at rest; Paused is a global
	// PauseAll on top of any dial level.
	Effort   string        `json:"effort"`
	Paused   bool          `json:"paused"`
	Budget   BudgetGauge   `json:"budget"`
	Kinds    []KindGauge   `json:"kinds"`    // one per registry row, scan-priority order
	InFlight []InFlightJob `json:"inFlight"` // jobs running now (bounded by the budget)
	// DLQ is the enrichment-error table rolled up by (kind, reason) — the only
	// part read from the catalog, not the in-memory scheduler.
	DLQ []domain.EnrichmentFailureCount `json:"dlq"`
}

// BudgetGauge is the weighted CPU budget's live position: the effort dial caps
// Usable at or below Capacity, and InUse is what running jobs currently hold.
// InUse ≤ Capacity always; it settles to ≤ Usable as jobs finish after a
// dial-down.
type BudgetGauge struct {
	Capacity int64 `json:"capacity"` // the semaphore's full size (≈ NumCPU)
	Usable   int64 `json:"usable"`   // the current effort level's cap
	InUse    int64 `json:"inUse"`    // tokens held by running jobs
}

// KindGauge is one registry row's live queue position. QueuedHot / QueuedCold
// split the pending band (viewport hint vs cold backlog); Running is in-flight
// for this kind; Workers is the pool size (Running / Workers is utilization);
// More reports the last scan hit its page limit, so the real backlog is deeper
// than QueuedCold — the missing artifact IS the queue (D28).
type KindGauge struct {
	Kind       string `json:"kind"`
	QueuedHot  int    `json:"queuedHot"`
	QueuedCold int    `json:"queuedCold"`
	Running    int    `json:"running"`
	Workers    int    `json:"workers"`
	Paused     bool   `json:"paused"` // this kind individually paused
	More       bool   `json:"more"`   // backlog beyond what is queued
}

// InFlightJob is one running job. Started is when the dispatcher handed it to a
// worker (in-flight-since). Tokens-held per job is deliberately not tracked — the
// aggregate Budget.InUse covers admission (DEFERRED §14).
type InFlightJob struct {
	AssetID string    `json:"assetId"`
	Kind    string    `json:"kind"`
	Started time.Time `json:"started"`
	Hinted  bool      `json:"hinted"`
}
