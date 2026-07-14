package enrichment

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
)

// Internal tests for the budget's weighted acquisition and the effort dial's
// reservation mechanics — the two behaviors too timing-sensitive to prove
// through full engine choreography (the engine-level budget acceptance lives
// in enrichment_test.go).

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
