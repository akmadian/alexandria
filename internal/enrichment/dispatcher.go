package enrichment

import (
	"context"
	"log/slog"
	"time"

	"github.com/akmadian/alexandria/internal/settings"
	"github.com/akmadian/alexandria/internal/sqlite"
)

// The dispatcher is ONE goroutine that owns every node's input queue and all
// ordering state. Workers rendezvous with it for jobs (request/reply below);
// it answers from the node's priority queue or parks the request until work
// or a resume arrives. Jobs reach a queue three ways — a scan (the authority,
// derived from catalog truth), an edge emission (a completed upstream job
// enqueues its dependents), or a hint (viewport priority, speculative) — and
// the three mix safely because none of them is the truth: every pop is
// rechecked against the catalog before producing, so a confused queue
// degrades to suboptimal ORDER, never to incorrectness.

// job is one assignment handed to a worker: run this definition against this
// asset.
type job struct {
	definition *JobDefinition
	assetID    string
	hinted     bool
}

// workRequest is a worker asking for its node's next job. reply is buffered
// (capacity 1) so the dispatcher never blocks answering.
type workRequest struct {
	kind  string
	reply chan *job
}

// pauseChange toggles one pause flag: global (kind == "", effort == false),
// per-kind, or the effort dial's pause (a separate flag so SetEffort and a
// user's PauseAll don't fight over one bit).
type pauseChange struct {
	kind   string
	effort bool
	paused bool
}

// completion is the writer's post-commit report for one job: what finished,
// and — when the artifact was applied — the asset facts edge emission needs
// to enqueue the node's dependents without a second catalog read.
type completion struct {
	key        JobKey
	applied    bool
	extension  string
	ingestedAt time.Time
}

// dispatcherState is owned by the dispatcher goroutine alone; nothing here is
// locked because nothing else may touch it.
type dispatcherState struct {
	engine            *Engine
	queues            map[string]*jobQueue   // one input queue per node
	ledger            map[JobKey]*pendingJob // queued or running — the dedup ledger
	pendingCount      map[string]int         // per node, for convergence detection
	hints             []string
	hintRanks         map[string]int // assetID → rank in the current hint set
	pausedGlobal      bool
	pausedEffort      bool
	pausedKinds       map[string]bool
	waitingWorkers    map[string][]chan *job // parked worker replies, per node
	moreToScan        map[string]bool        // last scan hit the page limit
	convergenceLogged map[string]bool
}

// newDispatcherState builds the dispatcher's runtime state for an engine —
// one queue per node, empty ledgers, pause flags from the initial effort.
// Shared by runDispatcher and the internal tests so test wiring can't drift.
func newDispatcherState(engine *Engine) *dispatcherState {
	state := &dispatcherState{
		engine:            engine,
		queues:            make(map[string]*jobQueue, len(engine.scanOrder)),
		ledger:            make(map[JobKey]*pendingJob),
		pendingCount:      make(map[string]int),
		hintRanks:         make(map[string]int),
		pausedEffort:      engine.initialEffort == settings.EffortPaused,
		pausedKinds:       make(map[string]bool),
		waitingWorkers:    make(map[string][]chan *job),
		moreToScan:        make(map[string]bool),
		convergenceLogged: make(map[string]bool),
	}
	for _, kind := range engine.scanOrder {
		state.queues[kind] = &jobQueue{}
	}
	return state
}

func (e *Engine) runDispatcher(ctx context.Context) error {
	state := newDispatcherState(e)
	state.scanAll(ctx) // the on-catalog-open scan (D25: crash recovery = rescan)
	for {
		select {
		case <-ctx.Done():
			return nil
		case request := <-e.requests:
			state.handleRequest(request)
		case assetIDs := <-e.hints:
			state.handleHint(assetIDs)
		case change := <-e.pauses:
			state.handlePause(change)
		case <-e.scanRequests:
			state.scanAll(ctx)
		case completions := <-e.completions:
			state.handleCompletions(ctx, completions)
		case reply := <-e.depthRequests:
			reply <- state.snapshotDepths()
		case reply := <-e.snapshotRequests:
			reply <- state.snapshot()
		}
	}
}

// snapshotDepths copies the per-kind count of jobs not yet complete — queued OR
// in-flight (pendingCount is bumped on enqueue and cleared only on completion, so
// a running job still counts) — the backlog signal the seam's progress payload
// rides on (task 21). Zero-count kinds (fully drained but still keyed) are skipped
// so the payload is sparse and a wholly-drained backlog is an empty map. A copy so
// the caller never touches dispatcher-owned state; task 22's fuller snapshot
// extends this read-out.
func (d *dispatcherState) snapshotDepths() map[string]int {
	depths := make(map[string]int, len(d.pendingCount))
	for kind, count := range d.pendingCount {
		if count > 0 {
			depths[kind] = count
		}
	}
	return depths
}

// snapshot builds the dispatcher-owned half of the debug Snapshot (task 22) on
// the dispatcher goroutine, so it reads scheduling state without a lock and never
// races a queue mutation. The engine fills the rest (effort level, budget gauge,
// DLQ) from state it owns directly. One pass over the ledger classifies every
// job — running, hinted-and-queued (hot), or cold-and-queued — and collects the
// in-flight list (bounded by the budget, so it never rivals catalog size).
func (d *dispatcherState) snapshot() Snapshot {
	hot := make(map[string]int, len(d.engine.scanOrder))
	cold := make(map[string]int, len(d.engine.scanOrder))
	running := make(map[string]int, len(d.engine.scanOrder))
	var inFlight []InFlightJob
	for key, pending := range d.ledger {
		switch {
		case pending.running:
			running[key.Kind]++
			inFlight = append(inFlight, InFlightJob{
				AssetID: key.AssetID,
				Kind:    key.Kind,
				Started: pending.startedAt,
				Hinted:  pending.priority == priorityHinted,
			})
		case pending.priority == priorityHinted:
			hot[key.Kind]++
		default:
			cold[key.Kind]++
		}
	}
	kinds := make([]KindGauge, 0, len(d.engine.scanOrder))
	for _, kind := range d.engine.scanOrder {
		kinds = append(kinds, KindGauge{
			Kind:       kind,
			QueuedHot:  hot[kind],
			QueuedCold: cold[kind],
			Running:    running[kind],
			Workers:    d.engine.workerCounts[kind],
			Paused:     d.pausedKinds[kind],
			More:       d.moreToScan[kind],
		})
	}
	return Snapshot{Paused: d.pausedGlobal, Kinds: kinds, InFlight: inFlight}
}

// enqueue records and queues a job for a node unless the ledger already holds
// it. Priority comes from the live hint set, so an edge emission for an asset
// the user is looking at inherits the fast track — priority belongs to the
// asset and carries through the graph at every step (D28).
func (d *dispatcherState) enqueue(kind, assetID string, ingestedAt time.Time) bool {
	key := JobKey{AssetID: assetID, Kind: kind}
	if d.ledger[key] != nil {
		return false
	}
	pending := &pendingJob{key: key, priority: priorityNormal, ingestedAt: ingestedAt}
	if rank, hinted := d.hintRanks[assetID]; hinted {
		pending.priority = priorityHinted
		pending.hintRank = rank
	}
	d.ledger[key] = pending
	d.pendingCount[kind]++
	d.queues[kind].enqueue(pending)
	d.convergenceLogged[kind] = false
	return true
}

// next pops a node's highest-priority job; nil when its queue is empty.
func (d *dispatcherState) next(kind string) *job {
	pending := d.queues[kind].dequeue()
	if pending == nil {
		return nil
	}
	return &job{
		definition: d.engine.definitions[kind],
		assetID:    pending.key.AssetID,
		hinted:     pending.priority == priorityHinted,
	}
}

// handleRequest answers a worker immediately when unpaused work exists, else
// parks the reply until a hint, scan, resume, or completion produces some.
func (d *dispatcherState) handleRequest(request workRequest) {
	if !d.paused(request.kind) {
		if assignment := d.next(request.kind); assignment != nil {
			request.reply <- assignment
			return
		}
	}
	d.waitingWorkers[request.kind] = append(d.waitingWorkers[request.kind], request.reply)
}

// handleHint replaces the hint set wholesale (latest wins): queued jobs from
// the previous generation demote back to the normal band, newly-hinted assets
// promote in place or enqueue speculatively for every node — the pop-time
// recheck discards the misses, so a hint can never cause wrong work.
func (d *dispatcherState) handleHint(assetIDs []string) {
	previousRanks := d.hintRanks
	d.hints = assetIDs
	d.hintRanks = make(map[string]int, len(assetIDs))
	for rank, assetID := range assetIDs {
		if _, seen := d.hintRanks[assetID]; !seen {
			d.hintRanks[assetID] = rank
		}
	}
	for assetID := range previousRanks {
		if _, stillHinted := d.hintRanks[assetID]; stillHinted {
			continue
		}
		for _, kind := range d.engine.scanOrder {
			entry := d.ledger[JobKey{AssetID: assetID, Kind: kind}]
			if entry != nil && !entry.running && entry.priority == priorityHinted {
				d.queues[kind].demote(entry)
			}
		}
	}
	for assetID, rank := range d.hintRanks {
		for _, kind := range d.engine.scanOrder {
			key := JobKey{AssetID: assetID, Kind: kind}
			if entry := d.ledger[key]; entry != nil {
				if !entry.running && (entry.priority != priorityHinted || entry.hintRank != rank) {
					d.queues[kind].promote(entry, rank)
				}
				continue
			}
			// Speculative: the asset's ingest time is unknown here (zero value),
			// which only matters if the job is later demoted — it then sorts
			// oldest, which is fair for speculation.
			d.enqueue(kind, assetID, time.Time{})
		}
	}
	d.engine.log.Info("enrichment: hint set replaced", "assets", len(assetIDs))
	d.drainWaiting()
}

func (d *dispatcherState) handlePause(change pauseChange) {
	switch {
	case change.effort:
		d.pausedEffort = change.paused
	case change.kind == "":
		d.pausedGlobal = change.paused
	default:
		d.pausedKinds[change.kind] = change.paused
	}
	verb := "resumed"
	if change.paused {
		verb = "paused"
	}
	scope := change.kind
	if scope == "" {
		scope = "all"
	}
	if change.effort {
		scope = "effort dial"
	}
	d.engine.log.Info("enrichment: dispatch "+verb, "scope", scope)
	if !change.paused {
		d.drainWaiting()
	}
}

func (d *dispatcherState) paused(kind string) bool {
	return d.pausedGlobal || d.pausedEffort || d.pausedKinds[kind]
}

// handleCompletions retires finished jobs from the ledger, walks the graph's
// outgoing edges for each applied artifact (enqueuing the node's dependents —
// this is how the frontier advances a level without waiting for a scan),
// refills any node whose queue drained below a full scan, and logs
// convergence transitions.
func (d *dispatcherState) handleCompletions(ctx context.Context, completions []completion) {
	for _, done := range completions {
		if _, known := d.ledger[done.key]; known {
			delete(d.ledger, done.key)
			d.pendingCount[done.key.Kind]--
		}
		if !done.applied {
			continue
		}
		for _, dependent := range d.engine.dependentsByKind[done.key.Kind] {
			if !d.engine.applicableByKind[dependent][done.extension] {
				continue
			}
			if d.enqueue(dependent, done.key.AssetID, done.ingestedAt) {
				d.engine.log.Debug("enrichment: edge emission enqueued dependent",
					"from", done.key.Kind, "to", dependent, "asset", done.key.AssetID)
			}
		}
	}
	for _, kind := range d.engine.scanOrder {
		if d.queues[kind].Len() == 0 && d.moreToScan[kind] {
			d.scan(ctx, kind)
		}
		d.noteConvergence(kind)
	}
	d.drainWaiting()
}

// scanAll runs one scan pass per node in priority-class order.
func (d *dispatcherState) scanAll(ctx context.Context) {
	for _, kind := range d.engine.scanOrder {
		d.scan(ctx, kind)
		d.noteConvergence(kind)
	}
	d.drainWaiting()
}

// scan fills one node's queue from the catalog: assets whose artifact is
// missing and whose prerequisites are present, newest ingest first, minus
// what the ledger already holds and minus attempt-exhausted DLQ rows. Runs
// inline in the dispatcher — a paged read is milliseconds, and inline keeps
// every queue mutation single-goroutine.
func (d *dispatcherState) scan(ctx context.Context, kind string) {
	engine := d.engine
	definition := engine.definitions[kind]
	_, span := engine.tracer.Start(ctx, "enrichment.scan", slog.String("kind", kind))
	artifacts, err := engine.enrichmentRepo.ListMissingArtifacts(ctx, &sqlite.MissingArtifactScan{
		Kind:                kind,
		ArtifactColumn:      definition.ArtifactColumn,
		PrerequisiteColumns: engine.prerequisiteColumns[kind],
		Extensions:          engine.extensionsByKind[kind],
		MaxAttempts:         MaxAttempts,
		Limit:               scanPageSize,
	})
	if err != nil {
		span.Fail(err)
		span.End()
		if ctx.Err() == nil {
			engine.log.Error("enrichment: missing-artifact scan failed", "kind", kind, "err", err)
		}
		return
	}
	enqueued := 0
	for _, artifact := range artifacts {
		if d.enqueue(kind, artifact.AssetID, artifact.IngestedAt) {
			enqueued++
		}
	}
	d.moreToScan[kind] = len(artifacts) == scanPageSize
	span.SetAttrs(slog.Int("found", len(artifacts)), slog.Int("enqueued", enqueued))
	span.End()
	if enqueued > 0 {
		engine.log.Info("enrichment: scan pass filled queue", "kind", kind, "found", len(artifacts), "enqueued", enqueued)
	} else {
		engine.log.Debug("enrichment: scan pass found nothing new", "kind", kind, "found", len(artifacts))
	}
}

// noteConvergence logs — once per transition — when a node has nothing
// missing, nothing queued, and nothing running: its artifact machines all
// reached present or failed.
func (d *dispatcherState) noteConvergence(kind string) {
	idle := d.queues[kind].Len() == 0 && d.pendingCount[kind] == 0 && !d.moreToScan[kind]
	if idle && !d.convergenceLogged[kind] {
		d.convergenceLogged[kind] = true
		d.engine.log.Info("enrichment: kind converged", "kind", kind)
	}
}

// drainWaiting answers parked workers while unpaused work exists for their
// node.
func (d *dispatcherState) drainWaiting() {
	for kind, parked := range d.waitingWorkers {
		if d.paused(kind) {
			continue
		}
		served := 0
		for _, reply := range parked {
			assignment := d.next(kind)
			if assignment == nil {
				break
			}
			reply <- assignment
			served++
		}
		if served > 0 {
			d.waitingWorkers[kind] = append([]chan *job(nil), parked[served:]...)
		}
	}
}
