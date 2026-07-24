// Package enrichment is the convergent-lane background engine (D25/D28). The
// mental model is a directed acyclic graph managed as a build system, never a
// dataflow pipeline: nodes are job definitions, edges are their prerequisite
// declarations, queues hold jobs — work orders, an asset ID and a priority,
// never a payload — and ground truth lives only in the catalog. The engine
// schedules jobs over assets; the catalog accumulates artifacts.
//
// Each (asset, kind) artifact walks missing → queued → running → present |
// failed(n), and three of those states are DERIVED — no artifact / artifact
// exists / DLQ row — while queued/running live only in memory. There is no
// job journal and no run identity: crash recovery is a rescan, cancel
// dissolves into pause, and "the missing artifact IS the queue" (D17,
// generalized). Every enqueue is a claim; every pop rechecks the catalog —
// so a wrong or stale queue degrades to suboptimal order, never wrong data.
//
// Execution shape: one dispatcher goroutine owning per-node priority queues
// (container/heap; hinted band over import recency), per-definition worker
// pools running one uniform job template, a weighted CPU budget with a
// user-facing effort dial, per-volume I/O tokens, and ONE batched writer
// goroutine — the one-cook rule; ingest's writer and this one take orderly
// turns at the WAL lock. Write ordering is a contract: DB write → clear
// tracker bit → emit, so an invalidation never races a stale read.
package enrichment

import (
	"context"
	"runtime"
	"slices"
	"sort"
	"sync/atomic"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/settings"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/gospan"
	"github.com/charmbracelet/log"
	"golang.org/x/sync/errgroup"
)

const (
	// scanPageSize bounds one missing-artifact scan pass; a full page means
	// "more work likely — rescan when the queue drains".
	scanPageSize = 512
	// MaxAttempts is the DLQ exhaustion threshold: at this many recorded
	// failures nothing re-enqueues the job and the asset reads "failed".
	// Exported for consumers of the failed state (the dev harness's
	// convergence check; task 21's per-asset decoration).
	MaxAttempts = 5
	// writeBatchSize / writeLull mirror the ingest WRITE stage's batching.
	writeBatchSize = 50
)

// JobKey identifies one job: which definition (Kind) over which asset. It
// keys the dispatcher's ledger and mirrors the DLQ's (asset_id, kind) natural
// key; the committed-batch hook hands it out. The job's OPERAND is the asset
// — the artifact is what a completed job leaves behind in the catalog, so
// "artifact" vocabulary stays on the catalog side (repo, DLQ, state model).
type JobKey struct {
	AssetID string
	Kind    string
}

// Config wires an Engine. Definitions is the job registry (the canonical rows
// from Definitions(), or a test's fakes — both flow through identical
// validation).
type Config struct {
	Definitions []JobDefinition
	Reader      catalog.AssetReader
	Store       *sqlite.Store
	Enrichment  *sqlite.EnrichmentRepo
	Log         *log.Logger
	// Tracer instruments jobs with gospan spans (enrichment.<kind> roots,
	// enrichment.produce children, enrichment.scan and enrichment.write-batch
	// passes). Nil is off — a ~4ns no-op per call (D30).
	Tracer *gospan.Tracer
	// Machine supplies the worker-count overrides (Workers.Enrichment.<kind>),
	// the effort dial's starting level, and the per-volume I/O depth.
	Machine settings.Machine
	// BudgetCapacity overrides the CPU-budget token capacity; 0 means NumCPU.
	// Tests pin it for determinism; production leaves it 0.
	BudgetCapacity int64
	// OnBatchCommitted, if set, fires after each writer batch commits, AFTER
	// the tracker bits clear (the ordering contract). Nil-safe. It runs on the
	// writer goroutine, and calling back into the engine (QueueDepths — the seam
	// composition does exactly that) is safe by design: completions are handed
	// to the dispatcher on a buffered channel before the hook fires, and the
	// dispatcher never blocks on the writer. Keep it that way.
	OnBatchCommitted func(committed []JobKey)
}

// Engine is the running enrichment system: the runtime that reifies the
// definition graph. Construct with New, launch with Start, stop with Stop;
// one Engine per open catalog.
type Engine struct {
	reader           catalog.AssetReader
	store            *sqlite.Store
	enrichmentRepo   *sqlite.EnrichmentRepo
	log              *log.Logger
	tracer           *gospan.Tracer
	onBatchCommitted func(committed []JobKey)

	definitions         map[string]*JobDefinition
	scanOrder           []string // Priority-class order for scan passes: Priority, then row order
	kindBits            map[string]KindSet
	extensionsByKind    map[string][]string
	applicableByKind    map[string]map[string]bool // kind → extension set, for the dispatch recheck
	prerequisiteColumns map[string][]string
	dependentsByKind    map[string][]string // outgoing edges: kind → kinds listing it as prerequisite
	workerCounts        map[string]int
	initialEffort       string
	currentEffort       atomic.Value // string: the live dial level, for the debug snapshot (task 22)
	// failuresPossible gates the DLQ read (FailedKinds decoration) and the
	// post-apply DLQ clear: false means no enrichment failure has ever been
	// recorded, so both are pure no-ops and skipped. Seeded from the catalog at
	// Start, latched true by the writer on the first LogFailure. Monotonic — a
	// catalog that once had failures keeps paying the queries, which is fine.
	failuresPossible atomic.Bool

	tracker    *InFlightTracker
	budget     *cpuBudget
	readTokens *volumeReadTokens

	requests         chan workRequest
	results          chan *jobResult
	hints            chan []string
	pauses           chan pauseChange
	scanRequests     chan struct{}
	completions      chan []completion
	depthRequests    chan chan map[string]int // seam queue-depth reads (task 21)
	snapshotRequests chan chan Snapshot       // debug-surface state reads (task 22)

	runCtx context.Context
	stop   context.CancelFunc
	group  *errgroup.Group
}

// New validates the registry and builds an Engine (boot-time validation, C10:
// an invalid registry fails construction, never a user session).
func New(config *Config) (*Engine, error) {
	if err := Validate(config.Definitions); err != nil {
		return nil, err
	}
	capacity := config.BudgetCapacity
	if capacity <= 0 {
		capacity = int64(runtime.NumCPU())
	}
	initialEffort := config.Machine.Enrichment.Effort
	if initialEffort == "" {
		initialEffort = settings.EffortNormal
	}
	readDepth := int64(config.Machine.Enrichment.IOTokens)
	if readDepth <= 0 {
		readDepth = int64(settings.DefaultMachine().Enrichment.IOTokens)
	}

	engine := &Engine{
		reader:              config.Reader,
		store:               config.Store,
		enrichmentRepo:      config.Enrichment,
		log:                 config.Log,
		tracer:              config.Tracer,
		onBatchCommitted:    config.OnBatchCommitted,
		definitions:         make(map[string]*JobDefinition, len(config.Definitions)),
		kindBits:            make(map[string]KindSet, len(config.Definitions)),
		extensionsByKind:    make(map[string][]string, len(config.Definitions)),
		applicableByKind:    make(map[string]map[string]bool, len(config.Definitions)),
		prerequisiteColumns: make(map[string][]string, len(config.Definitions)),
		workerCounts:        make(map[string]int, len(config.Definitions)),
		initialEffort:       initialEffort,
		tracker:             NewInFlightTracker(),
		budget:              newCPUBudget(capacity, config.Log),
		readTokens:          newVolumeReadTokens(readDepth),
		requests:            make(chan workRequest),
		results:             make(chan *jobResult, 64),
		hints:               make(chan []string, 1),
		pauses:              make(chan pauseChange, 8),
		scanRequests:        make(chan struct{}, 1),
		completions:         make(chan []completion, 64),
		depthRequests:       make(chan chan map[string]int),
		snapshotRequests:    make(chan chan Snapshot),
	}
	engine.currentEffort.Store(initialEffort)

	definitions := make([]JobDefinition, len(config.Definitions))
	copy(definitions, config.Definitions)
	for index := range definitions {
		definition := &definitions[index]
		engine.definitions[definition.Kind] = definition
		engine.kindBits[definition.Kind] = KindSet(1) << index
		engine.extensionsByKind[definition.Kind] = applicableExtensions(definition)
		applicableSet := make(map[string]bool, len(engine.extensionsByKind[definition.Kind]))
		for _, extension := range engine.extensionsByKind[definition.Kind] {
			applicableSet[extension] = true
		}
		engine.applicableByKind[definition.Kind] = applicableSet
		engine.workerCounts[definition.Kind] = definition.DefaultWorkers
		if override := config.Machine.Workers.Enrichment[definition.Kind]; override > 0 {
			engine.workerCounts[definition.Kind] = override
		}
		engine.scanOrder = append(engine.scanOrder, definition.Kind)
	}
	// The reverse index IS the outgoing-edge table: completing a prerequisite's job
	// emits into these dependents' queues. Same relation the graph renderer builds,
	// so share the one pure function rather than open-code it twice.
	engine.dependentsByKind = dependentsByKind(definitions)
	for index := range definitions {
		definition := &definitions[index]
		columns := make([]string, 0, len(definition.Prerequisites))
		for _, prerequisite := range definition.Prerequisites {
			columns = append(columns, engine.definitions[prerequisite].ArtifactColumn)
		}
		engine.prerequisiteColumns[definition.Kind] = columns
	}
	sort.SliceStable(engine.scanOrder, func(left, right int) bool {
		return engine.definitions[engine.scanOrder[left]].Priority <
			engine.definitions[engine.scanOrder[right]].Priority
	})
	return engine, nil
}

// Start launches the dispatcher, the per-definition worker pools, the writer,
// and the budget's reservation manager; the dispatcher's first act is the
// on-open missing-artifact scan. Non-blocking; call Stop to shut down.
// Single-use.
func (e *Engine) Start(ctx context.Context) {
	if e.runCtx != nil {
		panic("enrichment: Start called twice")
	}
	runCtx, cancel := context.WithCancel(ctx)
	e.runCtx = runCtx
	e.stop = cancel
	group, groupCtx := errgroup.WithContext(runCtx)
	e.group = group

	// Seed the failures gate: on a catalog with no enrichment errors the DLQ read
	// (decoration) and the post-apply DLQ clear are no-ops and skip their queries.
	// Default armed; only a clean probe disarms it (a probe error leaves it armed).
	e.failuresPossible.Store(true)
	if anyFailures, err := e.enrichmentRepo.AnyFailures(ctx); err != nil {
		e.log.Warn("enrichment: failure-gate probe failed; DLQ reads stay armed", "err", err)
	} else {
		e.failuresPossible.Store(anyFailures)
	}

	group.Go(func() error { e.budget.manageReservation(groupCtx, e.initialEffort); return nil })
	group.Go(func() error { return e.runDispatcher(groupCtx) })
	group.Go(func() error { return e.runWriter(groupCtx) })
	totalWorkers := 0
	for _, kind := range e.scanOrder {
		definition := e.definitions[kind]
		for range e.workerCounts[kind] {
			group.Go(func() error { return e.runWorker(groupCtx, definition) })
		}
		totalWorkers += e.workerCounts[kind]
	}
	e.log.Info("enrichment: engine started",
		"definitions", len(e.scanOrder), "workers", totalWorkers,
		"effort", e.initialEffort, "budget", e.budget.capacity)
}

// Stop pauses by dying (D28: app quit = pause): dispatch ends, in-flight
// producers see cancellation, the writer commits its current batch, and the
// queues simply cease to exist — the next Start's scan re-derives them.
func (e *Engine) Stop() {
	e.mustBeStarted("Stop")
	e.stop()
	_ = e.group.Wait() // goroutines return nil on shutdown; real errors were logged in place
	e.log.Info("enrichment: engine stopped")
}

// Hint replaces the hint set wholesale with the given assets, in order —
// latest hint wins, older unserved hints demote back to the normal band
// (D28). A hint is never truth: ineligible or already-enriched assets are
// skipped at dispatch, so a confused queue degrades to suboptimal ordering,
// never incorrectness.
func (e *Engine) Hint(assetIDs []string) {
	e.mustBeStarted("Hint")
	hinted := slices.Clone(assetIDs) // the dispatcher reads this long after we return
	for {
		select {
		case e.hints <- hinted:
			return
		case <-e.runCtx.Done():
			return
		default:
			select { // full: discard the older unserved hint — latest wins
			case <-e.hints:
			default:
			}
		}
	}
}

// RequestScan asks the dispatcher for a full missing-artifact scan pass — the
// on-demand half of "on catalog open + on demand" (task 19 wires ingest's
// post-commit nudge here). Coalesces: a pending request absorbs new ones.
func (e *Engine) RequestScan() {
	e.mustBeStarted("RequestScan")
	select {
	case e.scanRequests <- struct{}{}:
	default:
	}
}

// PauseAll stops dispatching everywhere; in-flight jobs finish and commit.
func (e *Engine) PauseAll() { e.sendPause(pauseChange{paused: true}) }

// ResumeAll resumes global dispatch (per-kind pauses keep their own state).
func (e *Engine) ResumeAll() { e.sendPause(pauseChange{paused: false}) }

// PauseKind stops dispatching one definition's jobs.
func (e *Engine) PauseKind(kind string) { e.sendPause(pauseChange{kind: kind, paused: true}) }

// ResumeKind resumes one definition's jobs.
func (e *Engine) ResumeKind(kind string) { e.sendPause(pauseChange{kind: kind, paused: false}) }

// SetEffort applies the user-facing dial: paused | low | normal | full.
// "paused" is a dispatch pause (the budget keeps its last real level); any
// other level resumes the effort pause and resizes the usable budget. An
// unknown level is rejected loudly — settings sanitize guards the persisted
// value, but a live caller (the task-21 seam setter) gets no silent remap.
func (e *Engine) SetEffort(level string) {
	switch level {
	case settings.EffortPaused, settings.EffortLow, settings.EffortNormal, settings.EffortFull:
	default:
		e.log.Warn("enrichment: unknown effort level ignored", "level", level)
		return
	}
	e.currentEffort.Store(level) // the live dial, for the debug snapshot (task 22)
	paused := level == settings.EffortPaused
	e.sendPause(pauseChange{effort: true, paused: paused})
	if !paused {
		e.budget.setLevel(level)
	}
}

// Tracker exposes the in-flight ledger for seam decoration (task 21).
func (e *Engine) Tracker() *InFlightTracker { return e.tracker }

// QueueDepths returns a per-kind snapshot of jobs not yet complete — queued OR
// in-flight — the backlog signal the seam's jobs/progress payload carries (task
// 21); a fully-drained kind is absent (sparse). Answered by the dispatcher
// goroutine, so it never races queue mutation. Returns an empty map after Stop
// (the dispatcher no longer answers; the read falls through on the stopped context).
func (e *Engine) QueueDepths() map[string]int {
	e.mustBeStarted("QueueDepths")
	reply := make(chan map[string]int, 1)
	select {
	case e.depthRequests <- reply:
		return <-reply
	case <-e.runCtx.Done():
		return map[string]int{}
	}
}

// Snapshot returns the live debug-surface state (D28 commitment #4, task 22):
// the dispatcher builds its scheduling half on its own goroutine (queue depths,
// in-flight, pause state), and the engine fills the effort dial, the budget gauge
// (from atomics), and the DLQ rollup (one catalog query). After Stop the
// dispatcher no longer answers, so the scheduling half comes back empty while the
// budget and DLQ — which need no dispatcher — still report.
func (e *Engine) Snapshot(ctx context.Context) (Snapshot, error) {
	e.mustBeStarted("Snapshot")
	reply := make(chan Snapshot, 1)
	var snapshot Snapshot
	select {
	case e.snapshotRequests <- reply:
		snapshot = <-reply
	case <-e.runCtx.Done():
	}
	snapshot.Effort = e.effortLevel()
	snapshot.Budget = e.budget.gauge()
	dlq, err := e.enrichmentRepo.FailureCounts(ctx, MaxAttempts)
	if err != nil {
		return snapshot, err
	}
	snapshot.DLQ = dlq
	return snapshot, nil
}

// effortLevel reads the live dial level set by SetEffort (initialized to the
// engine's starting effort).
func (e *Engine) effortLevel() string {
	level, _ := e.currentEffort.Load().(string)
	return level
}

// KindBit returns the tracker bit assigned to a definition (zero for unknown
// kinds).
func (e *Engine) KindBit(kind string) KindSet { return e.kindBits[kind] }

func (e *Engine) sendPause(change pauseChange) {
	e.mustBeStarted("pause/resume")
	select {
	case e.pauses <- change:
	case <-e.runCtx.Done():
	}
}

func (e *Engine) mustBeStarted(method string) {
	if e.runCtx == nil {
		panic("enrichment: " + method + " called before Start")
	}
}
