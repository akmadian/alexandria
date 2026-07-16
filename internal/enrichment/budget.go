package enrichment

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/akmadian/alexandria/internal/settings"
	"github.com/charmbracelet/log"
	"golang.org/x/sync/semaphore"
)

// The two admission-control layers above the per-kind pools (D28):
//
//   - budget — the global weighted CPU semaphore. Per-kind pools shape
//     fairness; this caps the SUM, so enrichment can never oversubscribe the
//     machine. Go cannot nice a goroutine, so admission is the only throttle —
//     which is why the user-facing effort dial maps onto it.
//   - sourceReadTokens — per-SOURCE read-depth caps (see settings.EnrichmentConfig for
//     why per-source, not per-device).

// budget wraps a weighted semaphore whose usable capacity shrinks and grows
// with the effort dial. The semaphore itself is fixed at full capacity; the
// dial works by RESERVING tokens (a manager goroutine holds capacity−target),
// which dodges the resize-a-semaphore problem entirely: dialing down simply
// waits for in-flight jobs to finish, exactly the pause semantics D28 wants.
type cpuBudget struct {
	semaphore *semaphore.Weighted
	capacity  int64
	usable    atomic.Int64 // tokensFor(current level); what acquire clamps to
	inUse     atomic.Int64 // tokens held by running jobs — the debug gauge (task 22)
	levels    chan string  // effort-level changes, latest wins
	log       *log.Logger
}

func newCPUBudget(capacity int64, logger *log.Logger) *cpuBudget {
	if capacity < 1 {
		capacity = 1
	}
	budget := &cpuBudget{
		semaphore: semaphore.NewWeighted(capacity),
		capacity:  capacity,
		levels:    make(chan string, 1),
		log:       logger,
	}
	budget.usable.Store(capacity)
	return budget
}

// acquire takes the job's weight, blocking until the dialed-down capacity has
// room. Returns the tokens actually held — clamped to [1, currently-usable]:
// a jumbo job degrades to "the whole dialed budget", so it serializes against
// everything else but can never deadlock waiting for tokens the effort level
// will not release.
func (b *cpuBudget) acquire(ctx context.Context, tokens int64) (int64, error) {
	if tokens < 1 {
		tokens = 1
	}
	if usable := b.usable.Load(); tokens > usable {
		tokens = usable
	}
	if err := b.semaphore.Acquire(ctx, tokens); err != nil {
		return 0, err
	}
	b.inUse.Add(tokens)
	return tokens, nil
}

func (b *cpuBudget) release(tokens int64) {
	b.inUse.Add(-tokens)
	b.semaphore.Release(tokens)
}

// gauge is the budget's live position for the debug snapshot (task 22). InUse
// counts only JOB holds (the reservation manager bypasses acquire/release), so
// InUse ≤ Capacity always and settles to ≤ Usable as running jobs finish after a
// dial-down.
func (b *cpuBudget) gauge() BudgetGauge {
	return BudgetGauge{Capacity: b.capacity, Usable: b.usable.Load(), InUse: b.inUse.Load()}
}

// setLevel requests an effort level; the newest request wins (an unread older
// one is discarded).
func (b *cpuBudget) setLevel(level string) {
	for {
		select {
		case b.levels <- level:
			return
		default:
			select {
			case <-b.levels:
			default:
			}
		}
	}
}

// manageReservation is the dial: it holds capacity−tokensFor(level) tokens so
// jobs can only use what the level allows. Runs for the engine's lifetime.
// ponytail: dialing DOWN polls TryAcquire on a short tick instead of a
// cancellable blocking acquire — tens of milliseconds of latency on an
// operation whose effect is inherently "as jobs finish" costs nothing, and it
// keeps this loop trivially correct.
func (b *cpuBudget) manageReservation(ctx context.Context, initialLevel string) {
	var held int64
	b.usable.Store(b.tokensFor(initialLevel))
	target := b.capacity - b.tokensFor(initialLevel)
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	defer func() {
		if held > 0 {
			b.semaphore.Release(held)
		}
	}()
	for {
		for held > target {
			b.semaphore.Release(1)
			held--
		}
		for held < target && b.semaphore.TryAcquire(1) {
			held++
		}
		select {
		case <-ctx.Done():
			return
		case level := <-b.levels:
			b.usable.Store(b.tokensFor(level))
			target = b.capacity - b.tokensFor(level)
			b.log.Info("enrichment: effort level applied", "level", level, "tokens", b.tokensFor(level), "capacity", b.capacity)
		case <-tick.C:
		}
	}
}

// tokensFor maps an effort level to usable budget tokens. ponytail: stated
// assumptions (full = every core, halving down from there) pending real
// calibration — gospan's samples table is the appointed instrument (D30).
func (b *cpuBudget) tokensFor(level string) int64 {
	switch level {
	case settings.EffortLow:
		return max(b.capacity/4, 1)
	case settings.EffortFull:
		return b.capacity
	default: // normal — also what "paused" leaves in place (pause lives in the dispatcher)
		return max(b.capacity/2, 1)
	}
}

// sourceReadTokens caps concurrent producer reads per source, lazily minting one
// semaphore per source ID.
type sourceReadTokens struct {
	depth    int64
	mu       sync.Mutex
	bySource map[string]*semaphore.Weighted
}

func newSourceReadTokens(depth int64) *sourceReadTokens {
	if depth < 1 {
		depth = 1
	}
	return &sourceReadTokens{depth: depth, bySource: make(map[string]*semaphore.Weighted)}
}

func (pool *sourceReadTokens) acquire(ctx context.Context, sourceID string) error {
	return pool.sourceSemaphore(sourceID).Acquire(ctx, 1)
}

func (pool *sourceReadTokens) release(sourceID string) {
	pool.sourceSemaphore(sourceID).Release(1)
}

func (pool *sourceReadTokens) sourceSemaphore(sourceID string) *semaphore.Weighted {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.bySource[sourceID] == nil {
		pool.bySource[sourceID] = semaphore.NewWeighted(pool.depth)
	}
	return pool.bySource[sourceID]
}
