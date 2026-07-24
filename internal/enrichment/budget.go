package enrichment

import (
	"context"
	"sync"
	"sync/atomic"

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
//   - volumeReadTokens — per-VOLUME read-depth caps (see settings.EnrichmentConfig for
//     why per-volume, an approximation of per-device).

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
//
// Reclaim is a BLOCKING, fair Acquire, one token at a time — NOT TryAcquire. This
// is load-bearing: x/sync's TryAcquire is refused whenever any goroutine is queued
// in Acquire (it requires waiters.Len()==0), and Release hands each freed token to
// those FIFO waiters — so under a churning backlog a polling manager never wins a
// token and the dial silently fails to throttle. Queuing as a waiter makes the
// manager a fair participant that actually reclaims its reservation; one token per
// turn keeps it responsive to a level change. (No ticker: the loop now blocks only
// on real events — a freed token, a level change, or shutdown.)
func (b *cpuBudget) manageReservation(ctx context.Context, initialLevel string) {
	var held int64
	level := initialLevel
	b.usable.Store(b.tokensFor(level))
	defer func() {
		if held > 0 {
			b.semaphore.Release(held)
		}
	}()
	for {
		target := b.capacity - b.tokensFor(level)
		switch {
		case held > target: // dial-up: hand the surplus back at once
			b.semaphore.Release(held - target)
			held = target
		case held < target: // dial-down: reclaim toward the reservation, fairly
			select {
			case <-ctx.Done():
				return
			case level = <-b.levels:
				b.applyLevel(level)
			default:
				if err := b.semaphore.Acquire(ctx, 1); err != nil {
					return // ctx canceled while queued for a token
				}
				held++
			}
		default: // held == target: idle until the dial moves
			select {
			case <-ctx.Done():
				return
			case level = <-b.levels:
				b.applyLevel(level)
			}
		}
	}
}

// applyLevel resizes the usable budget (the per-job acquire clamp) to a new effort
// level and logs it; the reservation the manager holds reconciles toward
// capacity−usable on the next loop turn.
func (b *cpuBudget) applyLevel(level string) {
	b.usable.Store(b.tokensFor(level))
	b.log.Info("enrichment: effort level applied", "level", level, "tokens", b.tokensFor(level), "capacity", b.capacity)
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

// volumeReadTokens caps concurrent producer reads per volume, lazily minting one
// semaphore per volume ID. A volume approximates a physical device strictly
// better than a source did (D41 / DEFERRED §11) — real device detection stays
// deferred.
type volumeReadTokens struct {
	depth    int64
	mu       sync.Mutex
	byVolume map[string]*semaphore.Weighted
}

func newVolumeReadTokens(depth int64) *volumeReadTokens {
	if depth < 1 {
		depth = 1
	}
	return &volumeReadTokens{depth: depth, byVolume: make(map[string]*semaphore.Weighted)}
}

func (pool *volumeReadTokens) acquire(ctx context.Context, volumeID string) error {
	return pool.volumeSemaphore(volumeID).Acquire(ctx, 1)
}

func (pool *volumeReadTokens) release(volumeID string) {
	pool.volumeSemaphore(volumeID).Release(1)
}

func (pool *volumeReadTokens) volumeSemaphore(volumeID string) *semaphore.Weighted {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.byVolume[volumeID] == nil {
		pool.byVolume[volumeID] = semaphore.NewWeighted(pool.depth)
	}
	return pool.byVolume[volumeID]
}
