package enrichment

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/settings"
	"github.com/charmbracelet/log"
)

// Internal tests for the budget's weighted acquisition and the effort dial's
// reservation mechanics — the two behaviors too timing-sensitive to prove
// through full engine choreography (the engine-level budget acceptance lives
// in enrichment_test.go).

// TestBudgetReservationReclaimsUnderLoad: the effort dial must actually cap
// concurrency under a sustained backlog — the case a TryAcquire-polling reservation
// silently failed (x/sync refuses TryAcquire while any worker is queued in Acquire,
// and hands freed tokens to those FIFO waiters). With the blocking-Acquire manager,
// no more than `usable` jobs may hold tokens at once even while workers churn.
func TestBudgetReservationReclaimsUnderLoad(t *testing.T) {
	const capacity = 4
	budget := newCPUBudget(capacity, log.New(io.Discard))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go budget.manageReservation(ctx, settings.EffortLow) // usable = capacity/4 = 1

	// 2×capacity acquirers keep at least one goroutine queued in Acquire at all
	// times (the waiters-present condition), each briefly holding one token.
	stop := make(chan struct{})
	var workers sync.WaitGroup
	for range capacity * 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				held, err := budget.acquire(ctx, 1)
				if err != nil {
					return
				}
				time.Sleep(500 * time.Microsecond)
				budget.release(held)
			}
		}()
	}
	t.Cleanup(func() { close(stop); workers.Wait() })

	// Let the manager reclaim its reservation (capacity−usable tokens) under load —
	// a few ms is ample; 300 is slack. A defeated reclaim never gives tokens back.
	time.Sleep(300 * time.Millisecond)
	usable := budget.usable.Load()
	var maxSeen int64
	for range 500 {
		if inUse := budget.inUse.Load(); inUse > maxSeen {
			maxSeen = inUse
		}
		time.Sleep(200 * time.Microsecond)
	}
	if maxSeen > usable {
		t.Fatalf("effort dial defeated under load: saw %d concurrent token holders, dial (low) allows %d", maxSeen, usable)
	}
}

func TestBudgetJumboClampsAndBlocksUntilRoomFrees(t *testing.T) {
	tokenBudget := newCPUBudget(4, log.New(io.Discard))
	ctx := context.Background()

	lightHeld, err := tokenBudget.acquire(ctx, 1)
	if err != nil {
		t.Fatalf("light acquire: %v", err)
	}
	if lightHeld != 1 {
		t.Fatalf("light held %d tokens, want 1", lightHeld)
	}

	// A jumbo request over capacity clamps to the whole budget instead of
	// deadlocking — and therefore must WAIT while anything else holds tokens.
	jumboAcquired := make(chan int64, 1)
	go func() {
		held, acquireErr := tokenBudget.acquire(ctx, 100)
		if acquireErr != nil {
			close(jumboAcquired)
			return
		}
		jumboAcquired <- held
	}()
	select {
	case held := <-jumboAcquired:
		t.Fatalf("jumbo acquired %d tokens while the light job held budget", held)
	case <-time.After(100 * time.Millisecond): // blocked, as it must be
	}

	tokenBudget.release(lightHeld)
	select {
	case held := <-jumboAcquired:
		if held != 4 {
			t.Fatalf("jumbo held %d tokens, want the full capacity 4", held)
		}
		tokenBudget.release(held)
	case <-time.After(2 * time.Second):
		t.Fatal("jumbo never acquired after room freed")
	}
}

func TestBudgetEffortDialResizesUsableCapacity(t *testing.T) {
	tokenBudget := newCPUBudget(8, log.New(io.Discard))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	managerDone := make(chan struct{})
	go func() {
		tokenBudget.manageReservation(ctx, "low")
		close(managerDone)
	}()

	// At low, only a quarter of the capacity is usable; a request for
	// everything clamps to that quarter.
	waitForUsable(t, tokenBudget, 2)
	held, err := tokenBudget.acquire(ctx, 8)
	if err != nil {
		t.Fatalf("acquire at low: %v", err)
	}
	if held != 2 {
		t.Fatalf("held %d tokens at low effort, want 2 (capacity 8 / 4)", held)
	}
	tokenBudget.release(held)

	// Dial to full: the reservation drains and the whole budget opens up.
	tokenBudget.setLevel("full")
	waitForUsable(t, tokenBudget, 8)
	acquired := make(chan int64, 1)
	go func() {
		fullHeld, acquireErr := tokenBudget.acquire(ctx, 8)
		if acquireErr == nil {
			acquired <- fullHeld
		}
	}()
	select {
	case fullHeld := <-acquired:
		if fullHeld != 8 {
			t.Fatalf("held %d tokens at full effort, want 8", fullHeld)
		}
		tokenBudget.release(fullHeld)
	case <-time.After(2 * time.Second):
		t.Fatal("full-capacity acquire never succeeded after dialing to full")
	}

	cancel()
	<-managerDone // the manager must release its reservation and exit cleanly
}

func waitForUsable(t *testing.T, tokenBudget *cpuBudget, want int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if tokenBudget.usable.Load() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("usable capacity never reached %d (at %d)", want, tokenBudget.usable.Load())
}
