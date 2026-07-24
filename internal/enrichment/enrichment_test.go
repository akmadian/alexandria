package enrichment_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/assettype"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/enrichment"
	"github.com/akmadian/alexandria/internal/settings"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
	"github.com/akmadian/alexandria/internal/thumbnailer"
	"github.com/charmbracelet/log"
)

// The fake kind is the engine's permanent test instrument: it can be poisoned
// on demand (DLQ), sized jumbo (budget), gated (pause/tracker), and made
// instant (hot lane) — none of which a real decoder does deterministically.
// It writes thumbnail_at, the one derived artifact column task 18 has, through
// the same ApplyFunc/derived-writer path every real kind will use.

// fakeKind returns a valid convergent kind over image assets. produce may be
// nil for a default instant success.
func fakeDefinition(name string, produce enrichment.ProduceFunc) enrichment.JobDefinition {
	if produce == nil {
		produce = func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
			return applyThumbnailAt(asset.ID), nil
		}
	}
	return enrichment.JobDefinition{
		Kind:           name,
		Lane:           enrichment.LaneConvergent,
		Applicable:     func(handler assettype.Handler) bool { return handler.Type == domain.FileTypeImage },
		ArtifactColumn: "thumbnail_at",
		DefaultWorkers: 2,
		TimeoutPolicy:  func(sizeBytes int64, fileType domain.FileType) time.Duration { return 5 * time.Second },
		Produce:        produce,
	}
}

func applyThumbnailAt(assetID string) enrichment.ApplyFunc {
	completedAt := time.Now().UTC()
	return func(ctx context.Context, writer catalog.AssetDerivedWriter) error {
		return writer.SetThumbnailAt(ctx, assetID, completedAt)
	}
}

// produceLog records the order producers ran in, across goroutines.
type produceLog struct {
	mu    sync.Mutex
	order []string
}

func (p *produceLog) record(assetID string) {
	p.mu.Lock()
	p.order = append(p.order, assetID)
	p.mu.Unlock()
}

func (p *produceLog) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.order...)
}

// harness wires an engine over a migrated in-memory catalog with count assets.
type harness struct {
	db     *sql.DB
	engine *enrichment.Engine
	assets []*domain.Asset
}

func newHarness(t *testing.T, definitions []enrichment.JobDefinition, machine settings.Machine, budgetCapacity int64, count int) *harness {
	t.Helper()
	db := testutil.NewTestDB(t)
	source := testutil.NewTestVolume(t, db, "enrich")
	assets := make([]*domain.Asset, 0, count)
	for index := range count {
		assets = append(assets, testutil.NewTestAsset(t, db, source.ID, fmt.Sprintf("photo-%03d.jpg", index)))
	}
	engine, err := enrichment.New(&enrichment.Config{
		Definitions:    definitions,
		Reader:         &sqlite.AssetRepo{DB: db},
		Store:          sqlite.NewStore(db),
		Enrichment:     &sqlite.EnrichmentRepo{DB: db},
		Log:            log.New(io.Discard),
		Machine:        machine,
		BudgetCapacity: budgetCapacity,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &harness{db: db, engine: engine, assets: assets}
}

func (h *harness) start(t *testing.T) {
	t.Helper()
	h.engine.Start(context.Background())
	t.Cleanup(h.engine.Stop)
}

func (h *harness) missingCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := h.db.QueryRow("SELECT COUNT(*) FROM assets WHERE thumbnail_at IS NULL").Scan(&count); err != nil {
		t.Fatalf("count missing: %v", err)
	}
	return count
}

func (h *harness) dlqAttempts(t *testing.T, assetID, kind string) int {
	t.Helper()
	var attempts int
	err := h.db.QueryRow("SELECT attempts FROM enrichment_errors WHERE asset_id = ? AND kind = ?", assetID, kind).Scan(&attempts)
	if err == sql.ErrNoRows {
		return 0
	}
	if err != nil {
		t.Fatalf("dlq attempts: %v", err)
	}
	return attempts
}

func waitUntil(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func normalMachine() settings.Machine {
	machine := settings.DefaultMachine()
	return machine
}

func pausedMachine() settings.Machine {
	machine := settings.DefaultMachine()
	machine.Enrichment.Effort = settings.EffortPaused
	return machine
}

// --- Registry validation -----------------------------------------------------

func TestRegistryValidation(t *testing.T) {
	valid := fakeDefinition("alpha", nil)
	cases := []struct {
		name    string
		mutate  func(definitions []enrichment.JobDefinition) []enrichment.JobDefinition
		wantErr string
	}{
		{"valid row passes", nil, ""},
		{"cycle fails", func(definitions []enrichment.JobDefinition) []enrichment.JobDefinition {
			second := fakeDefinition("beta", nil)
			definitions[0].Prerequisites = []string{"beta"}
			second.Prerequisites = []string{"alpha"}
			return append(definitions, second)
		}, "cycle"},
		{"self-cycle fails", func(definitions []enrichment.JobDefinition) []enrichment.JobDefinition {
			definitions[0].Prerequisites = []string{"alpha"}
			return definitions
		}, "cycle"},
		{"dangling prerequisite fails", func(definitions []enrichment.JobDefinition) []enrichment.JobDefinition {
			definitions[0].Prerequisites = []string{"ghost"}
			return definitions
		}, "unknown prerequisite"},
		{"applicable to nothing fails", func(definitions []enrichment.JobDefinition) []enrichment.JobDefinition {
			definitions[0].Applicable = func(assettype.Handler) bool { return false }
			return definitions
		}, "applicable to no"},
		{"unknown artifact column fails", func(definitions []enrichment.JobDefinition) []enrichment.JobDefinition {
			definitions[0].ArtifactColumn = "rating" // a judgment column must never pass
			return definitions
		}, "not a derived artifact column"},
		{"duplicate name fails", func(definitions []enrichment.JobDefinition) []enrichment.JobDefinition {
			return append(definitions, fakeDefinition("alpha", nil))
		}, "duplicate"},
		{"missing producer fails", func(definitions []enrichment.JobDefinition) []enrichment.JobDefinition {
			definitions[0].Produce = nil
			return definitions
		}, "no producer"},
		{"missing timeout policy fails", func(definitions []enrichment.JobDefinition) []enrichment.JobDefinition {
			definitions[0].TimeoutPolicy = nil
			return definitions
		}, "no timeout policy"},
		{"non-positive workers fails", func(definitions []enrichment.JobDefinition) []enrichment.JobDefinition {
			definitions[0].DefaultWorkers = 0
			return definitions
		}, "non-positive default workers"},
		{"unknown lane fails", func(definitions []enrichment.JobDefinition) []enrichment.JobDefinition {
			definitions[0].Lane = "sideways"
			return definitions
		}, "unknown lane"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			definitions := []enrichment.JobDefinition{valid}
			if testCase.mutate != nil {
				definitions = testCase.mutate(definitions)
			}
			err := enrichment.Validate(definitions)
			if testCase.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: unexpected error %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("Validate: got %v, want error containing %q", err, testCase.wantErr)
			}
		})
	}
}

func TestMustValidatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustValidate: expected panic on cyclic registry")
		}
	}()
	first := fakeDefinition("alpha", nil)
	second := fakeDefinition("beta", nil)
	first.Prerequisites = []string{"beta"}
	second.Prerequisites = []string{"alpha"}
	enrichment.MustValidate([]enrichment.JobDefinition{first, second})
}

// nopVolumeResolver is a placeholder VolumeResolver for validation-only tests
// (producers are never invoked, so Absolute is never called).
type nopVolumeResolver struct{}

func (nopVolumeResolver) Absolute(context.Context, string, string) (string, error) {
	return "", nil
}

func TestCanonicalRegistryValidates(t *testing.T) {
	// The boot-time sweep as a table test (C10): the canonical rows must
	// always pass. The dependencies are placeholders — validation exercises
	// applicability and shape, never the producers.
	definitions := enrichment.Definitions(thumbnailer.New(t.TempDir()), nopVolumeResolver{})
	if err := enrichment.Validate(definitions); err != nil {
		t.Fatalf("canonical registry invalid: %v", err)
	}
}

// --- Convergence -------------------------------------------------------------

func TestConvergence_ScanProducesCommitsAndSettles(t *testing.T) {
	recorded := &produceLog{}
	definition := fakeDefinition("fake", func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
		recorded.record(asset.ID)
		return applyThumbnailAt(asset.ID), nil
	})
	harness := newHarness(t, []enrichment.JobDefinition{definition}, normalMachine(), 4, 8)
	harness.start(t)

	waitUntil(t, "all artifacts present", func() bool { return harness.missingCount(t) == 0 })

	// Convergence: a fresh on-demand scan finds nothing and re-runs nobody.
	produced := len(recorded.snapshot())
	if produced != 8 {
		t.Fatalf("produced %d artifacts, want 8", produced)
	}
	harness.engine.RequestScan()
	time.Sleep(100 * time.Millisecond)
	if again := len(recorded.snapshot()); again != produced {
		t.Fatalf("re-scan re-produced artifacts: %d → %d", produced, again)
	}
	var dlqCount int
	if err := harness.db.QueryRow("SELECT COUNT(*) FROM enrichment_errors").Scan(&dlqCount); err != nil {
		t.Fatalf("count dlq: %v", err)
	}
	if dlqCount != 0 {
		t.Fatalf("clean run left %d DLQ rows", dlqCount)
	}
}

// --- Prerequisite gating + edge emission ---------------------------------------

func TestPrerequisiteChain_GatesAndEmitsWithoutSpuriousWork(t *testing.T) {
	// A two-node chain sharing the one derived column task 18 has: the child
	// declares the parent as prerequisite, so its scan is gated on the SAME
	// column its own artifact lives in — meaning the child must NEVER produce
	// (missing implies prerequisite-absent; present implies ineligible). What
	// this proves: the scan's prerequisite gate holds, edge emission fires on
	// the parent's commits and lands harmlessly (enqueue → pop → recheck →
	// skip), nothing deadlocks, and no spurious DLQ rows appear. Task 20's
	// real multi-column graph carries the positive convergence proof.
	parentRuns := &produceLog{}
	childRuns := &produceLog{}
	parent := fakeDefinition("parent", func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
		parentRuns.record(asset.ID)
		return applyThumbnailAt(asset.ID), nil
	})
	child := fakeDefinition("child", func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
		childRuns.record(asset.ID)
		return applyThumbnailAt(asset.ID), nil
	})
	child.Prerequisites = []string{"parent"}
	harness := newHarness(t, []enrichment.JobDefinition{parent, child}, normalMachine(), 4, 4)
	harness.start(t)

	waitUntil(t, "parent artifacts converged", func() bool { return harness.missingCount(t) == 0 })
	time.Sleep(150 * time.Millisecond) // let emitted child jobs pop and skip
	if runs := len(parentRuns.snapshot()); runs != 4 {
		t.Fatalf("parent produced %d artifacts, want 4", runs)
	}
	if runs := childRuns.snapshot(); len(runs) != 0 {
		t.Fatalf("gated child produced %v — the prerequisite gate leaked", runs)
	}
	var dlqCount int
	if err := harness.db.QueryRow("SELECT COUNT(*) FROM enrichment_errors").Scan(&dlqCount); err != nil {
		t.Fatalf("count dlq: %v", err)
	}
	if dlqCount != 0 {
		t.Fatalf("chain run minted %d DLQ rows", dlqCount)
	}
}

// --- DLQ ---------------------------------------------------------------------

func TestDLQ_FailingProducerRecordsAndExhausts(t *testing.T) {
	recorded := &produceLog{}
	definition := fakeDefinition("fake", func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
		recorded.record(asset.ID)
		if asset.Filename == "photo-000.jpg" {
			return nil, enrichment.Fail("poisoned", fmt.Errorf("decode exploded"))
		}
		return applyThumbnailAt(asset.ID), nil
	})
	harness := newHarness(t, []enrichment.JobDefinition{definition}, normalMachine(), 4, 3)
	harness.start(t)
	poisoned := harness.assets[0].ID

	// The healthy assets converge; the poisoned one lands in the DLQ.
	waitUntil(t, "healthy assets converged", func() bool { return harness.missingCount(t) == 1 })
	waitUntil(t, "first DLQ row", func() bool { return harness.dlqAttempts(t, poisoned, "fake") == 1 })

	// Each on-demand scan is one retry (D13: the scan IS the retry machinery)
	// until the attempt budget exhausts.
	for attempt := 2; attempt <= 5; attempt++ {
		harness.engine.RequestScan()
		waitUntil(t, fmt.Sprintf("attempt %d recorded", attempt), func() bool {
			return harness.dlqAttempts(t, poisoned, "fake") == attempt
		})
	}

	// Exhausted: further scans skip the row — the producer is never called again.
	calls := len(recorded.snapshot())
	harness.engine.RequestScan()
	time.Sleep(150 * time.Millisecond)
	if again := len(recorded.snapshot()); again != calls {
		t.Fatalf("exhausted asset was retried: %d → %d producer calls", calls, again)
	}

	failures, err := (&sqlite.EnrichmentRepo{DB: harness.db}).ListFailures(context.Background(), poisoned)
	if err != nil {
		t.Fatalf("ListFailures: %v", err)
	}
	if len(failures) != 1 || failures[0].ReasonCode != "poisoned" || failures[0].Attempts != 5 {
		t.Fatalf("DLQ row = %+v, want kind fake / reason poisoned / attempts 5", failures[0])
	}
}

// --- Hot lane ----------------------------------------------------------------

func TestHotLane_HintJumpsQueueAndReplacesWholesale(t *testing.T) {
	recorded := &produceLog{}
	definition := fakeDefinition("fake", func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
		recorded.record(asset.ID)
		return applyThumbnailAt(asset.ID), nil
	})
	definition.DefaultWorkers = 1 // serialize so produce order IS dispatch order
	harness := newHarness(t, []enrichment.JobDefinition{definition}, pausedMachine(), 4, 10)
	harness.start(t)

	// Born paused: the open scan filled the cold backlog, nothing dispatched.
	time.Sleep(50 * time.Millisecond)
	if calls := len(recorded.snapshot()); calls != 0 {
		t.Fatalf("paused engine produced %d jobs", calls)
	}

	// Hint an arbitrary mid-pack asset, then REPLACE the hint before anything
	// dispatches: replace-wholesale means only the second hint is hot.
	discarded := harness.assets[3].ID
	hinted := harness.assets[7].ID
	harness.engine.Hint([]string{discarded})
	harness.engine.Hint([]string{hinted})
	time.Sleep(50 * time.Millisecond) // let the dispatcher absorb the hints before resuming
	harness.engine.SetEffort(settings.EffortNormal)

	waitUntil(t, "full convergence", func() bool { return harness.missingCount(t) == 0 })
	order := recorded.snapshot()
	if order[0] != hinted {
		t.Fatalf("first produced = %s, want the hinted asset %s (order %v)", order[0], hinted, order)
	}
	if order[1] == discarded {
		t.Fatalf("discarded hint %s still dispatched hot (order %v)", discarded, order)
	}
}

// --- Pause / resume ----------------------------------------------------------

func TestPause_StopsDispatchWhileInFlightFinishes(t *testing.T) {
	started := make(chan string, 16)
	gate := make(chan struct{})
	recorded := &produceLog{}
	definition := fakeDefinition("fake", func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
		recorded.record(asset.ID)
		started <- asset.ID
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return applyThumbnailAt(asset.ID), nil
	})
	definition.DefaultWorkers = 1
	harness := newHarness(t, []enrichment.JobDefinition{definition}, normalMachine(), 4, 5)
	harness.start(t)

	// One job in flight (the single worker holds it), then pause everything.
	inFlight := <-started
	harness.engine.PauseAll()
	close(gate) // the in-flight job finishes and must commit despite the pause

	waitUntil(t, "in-flight job committed under pause", func() bool { return harness.missingCount(t) == 4 })
	time.Sleep(100 * time.Millisecond) // grace: nothing new may dispatch
	if calls := recorded.snapshot(); len(calls) != 1 || calls[0] != inFlight {
		t.Fatalf("pause leaked dispatches: %v", calls)
	}

	harness.engine.ResumeAll()
	waitUntil(t, "resume drains the backlog", func() bool { return harness.missingCount(t) == 0 })
}

func TestPauseKind_AndStaleHintSkipsWithoutProducing(t *testing.T) {
	recorded := &produceLog{}
	definition := fakeDefinition("fake", func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
		recorded.record(asset.ID)
		return applyThumbnailAt(asset.ID), nil
	})
	harness := newHarness(t, []enrichment.JobDefinition{definition}, pausedMachine(), 4, 3)
	harness.start(t)

	// A per-kind pause holds even after the effort pause lifts.
	harness.engine.PauseKind("fake")
	harness.engine.SetEffort(settings.EffortNormal)
	time.Sleep(100 * time.Millisecond)
	if calls := len(recorded.snapshot()); calls != 0 {
		t.Fatalf("kind-paused engine produced %d jobs", calls)
	}
	harness.engine.ResumeKind("fake")
	waitUntil(t, "convergence after kind resume", func() bool { return harness.missingCount(t) == 0 })

	// A hint for an already-enriched asset is a stale hint: the dispatch-time
	// eligibility recheck skips it — no producer call, no tracker residue.
	produced := len(recorded.snapshot())
	harness.engine.Hint([]string{harness.assets[0].ID})
	time.Sleep(150 * time.Millisecond)
	if again := len(recorded.snapshot()); again != produced {
		t.Fatalf("stale hint reached a producer: %d → %d calls", produced, again)
	}
	if leftover := harness.engine.Tracker().Running(harness.assets[0].ID); leftover != 0 {
		t.Fatalf("stale hint left tracker bits %b", leftover)
	}
}

// --- Budget ------------------------------------------------------------------

func TestBudget_ConcurrentWeightNeverExceedsCapacity(t *testing.T) {
	const capacity = 4
	const weight = 2
	var inUse, maxObserved atomicMax
	definition := fakeDefinition("fake", func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
		maxObserved.observe(inUse.add(weight))
		defer inUse.add(-weight)
		time.Sleep(20 * time.Millisecond) // force overlap
		return applyThumbnailAt(asset.ID), nil
	})
	definition.Weight = func(sizeBytes int64) int64 { return weight }
	definition.DefaultWorkers = 8 // more workers than the budget can admit

	machine := normalMachine()
	machine.Enrichment.Effort = settings.EffortFull
	harness := newHarness(t, []enrichment.JobDefinition{definition}, machine, capacity, 12)
	harness.start(t)

	waitUntil(t, "convergence", func() bool { return harness.missingCount(t) == 0 })
	if observed := maxObserved.load(); observed > capacity {
		t.Fatalf("concurrent weight peaked at %d, budget capacity is %d", observed, capacity)
	}
	if observed := maxObserved.load(); observed <= weight {
		t.Fatalf("no concurrency observed (peak %d) — the test proved nothing", observed)
	}
}

// The jumbo-blocks-until-room-frees half of the budget acceptance is a unit
// test (budget_internal_test.go): choreographing two engine workers into a
// deterministic acquire order is a coin flip, and the property under test —
// weighted acquisition with clamping — is the budget's, not the dispatcher's.

// --- Tracker -----------------------------------------------------------------

func TestTracker_BitsSetDuringRunClearedAfterCommit(t *testing.T) {
	started := make(chan string, 4)
	gate := make(chan struct{})
	definition := fakeDefinition("fake", func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
		started <- asset.ID
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return applyThumbnailAt(asset.ID), nil
	})
	definition.DefaultWorkers = 1
	harness := newHarness(t, []enrichment.JobDefinition{definition}, normalMachine(), 4, 2)
	harness.start(t)
	stage := harness.engine.KindBit("fake")
	if stage == 0 {
		t.Fatal("StageOf returned zero for a registered kind")
	}

	running := <-started
	if got := harness.engine.Tracker().Running(running); got != stage {
		t.Fatalf("Running(%s) = %b during produce, want bit %b", running, got, stage)
	}
	// RunningBatch is sparse: only the in-flight asset appears.
	allIDs := []string{harness.assets[0].ID, harness.assets[1].ID, "asset-not-real"}
	batch := harness.engine.Tracker().RunningBatch(allIDs)
	if len(batch) != 1 || batch[running] != stage {
		t.Fatalf("RunningBatch = %v, want exactly {%s: %b}", batch, running, stage)
	}

	close(gate)
	waitUntil(t, "convergence", func() bool { return harness.missingCount(t) == 0 })
	// Cleared only after commit — by convergence, nothing may remain in flight.
	waitUntil(t, "tracker drained", func() bool {
		return len(harness.engine.Tracker().RunningBatch(allIDs)) == 0
	})
}

// --- Watchdog ----------------------------------------------------------------

func TestDLQ_ReasonTaxonomy(t *testing.T) {
	// The classify switch IS the DLQ taxonomy contract: each failure shape must
	// land under its own reason code (the UI's failed state and task 21's error
	// surface key off these).
	cases := []struct {
		name       string
		produce    enrichment.ProduceFunc
		watchdog   bool
		timeout    time.Duration
		wantReason string
	}{
		{
			name: "wall-clock deadline is timeout",
			produce: func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
			timeout:    50 * time.Millisecond,
			wantReason: "timeout",
		},
		{
			name: "nil apply with nil error is producer_defect",
			produce: func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
				return nil, nil
			},
			timeout:    5 * time.Second,
			wantReason: "producer_defect",
		},
		{
			name: "bare error is produce_failed",
			produce: func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
				return nil, fmt.Errorf("something ordinary broke")
			},
			timeout:    5 * time.Second,
			wantReason: "produce_failed",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			definition := fakeDefinition("fake", testCase.produce)
			definition.Watchdog = testCase.watchdog
			timeout := testCase.timeout
			definition.TimeoutPolicy = func(sizeBytes int64, fileType domain.FileType) time.Duration { return timeout }
			harness := newHarness(t, []enrichment.JobDefinition{definition}, normalMachine(), 4, 1)
			harness.start(t)
			waitUntil(t, "DLQ reason "+testCase.wantReason, func() bool {
				failures, err := (&sqlite.EnrichmentRepo{DB: harness.db}).ListFailures(context.Background(), harness.assets[0].ID)
				return err == nil && len(failures) == 1 && failures[0].ReasonCode == testCase.wantReason
			})
		})
	}
}

func TestWatchdog_StallLandsInDLQAsStalled(t *testing.T) {
	definition := fakeDefinition("fake", func(ctx context.Context, asset *domain.Asset, heartbeat func()) (enrichment.ApplyFunc, error) {
		<-ctx.Done() // go silent: never heartbeat, never finish
		return nil, ctx.Err()
	})
	definition.Watchdog = true
	definition.TimeoutPolicy = func(sizeBytes int64, fileType domain.FileType) time.Duration { return 50 * time.Millisecond }
	harness := newHarness(t, []enrichment.JobDefinition{definition}, normalMachine(), 4, 1)
	harness.start(t)

	waitUntil(t, "stall recorded in DLQ", func() bool {
		failures, err := (&sqlite.EnrichmentRepo{DB: harness.db}).ListFailures(context.Background(), harness.assets[0].ID)
		return err == nil && len(failures) == 1 && failures[0].ReasonCode == "stalled"
	})
}

// atomicMax tracks a running value and its high-water mark.
type atomicMax struct {
	mu      sync.Mutex
	current int64
	peak    int64
}

func (a *atomicMax) add(delta int64) int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.current += delta
	return a.current
}

func (a *atomicMax) observe(value int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if value > a.peak {
		a.peak = value
	}
}

func (a *atomicMax) load() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.peak
}
