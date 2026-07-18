package enrichment_test

import (
	"context"
	"sync"
	"testing"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/enrichment"
	"github.com/akmadian/alexandria/internal/settings"
	"github.com/akmadian/alexandria/internal/sqlite"
)

// kindGauge finds a kind's gauge in a snapshot; fails the test if absent.
func kindGauge(t *testing.T, snapshot *enrichment.Snapshot, kind string) enrichment.KindGauge {
	t.Helper()
	for _, gauge := range snapshot.Kinds {
		if gauge.Kind == kind {
			return gauge
		}
	}
	t.Fatalf("snapshot missing kind %q; kinds=%v", kind, snapshot.Kinds)
	return enrichment.KindGauge{}
}

// TestSnapshot_ReflectsPausedBacklog: paused so nothing drains, the on-start scan
// fills the cold band, and the snapshot reports depth, pool size, effort, and a
// zeroed budget in-use.
func TestSnapshot_ReflectsPausedBacklog(t *testing.T) {
	harness := newHarness(t, []enrichment.JobDefinition{fakeDefinition("alpha", nil)}, pausedMachine(), 3, 4)
	harness.start(t)
	waitUntil(t, "backlog enqueued", func() bool {
		return harness.engine.QueueDepths()["alpha"] == 4
	})

	snapshot, err := harness.engine.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Effort != settings.EffortPaused {
		t.Errorf("effort = %q, want paused", snapshot.Effort)
	}
	alpha := kindGauge(t, &snapshot, "alpha")
	if alpha.QueuedCold != 4 || alpha.Running != 0 || alpha.QueuedHot != 0 {
		t.Errorf("alpha gauge = %+v, want 4 cold / 0 running / 0 hot", alpha)
	}
	if alpha.Workers != 2 {
		t.Errorf("alpha workers = %d, want 2 (fakeDefinition default)", alpha.Workers)
	}
	if snapshot.Budget.Capacity != 3 {
		t.Errorf("budget capacity = %d, want 3", snapshot.Budget.Capacity)
	}
	if snapshot.Budget.InUse != 0 {
		t.Errorf("budget in-use = %d, want 0 (nothing running while paused)", snapshot.Budget.InUse)
	}
	if len(snapshot.InFlight) != 0 {
		t.Errorf("in-flight = %v, want none while paused", snapshot.InFlight)
	}
}

// TestSnapshot_DLQByReason: a logged failure surfaces in the snapshot's DLQ
// rollup with its kind and reason code (the poisoned-fixture acceptance).
func TestSnapshot_DLQByReason(t *testing.T) {
	harness := newHarness(t, []enrichment.JobDefinition{fakeDefinition("alpha", nil)}, pausedMachine(), 1, 1)
	harness.start(t)
	repo := &sqlite.EnrichmentRepo{DB: harness.db}
	if err := repo.LogFailure(context.Background(), harness.assets[0].ID, "alpha", "decode_failed", "bad pixels"); err != nil {
		t.Fatal(err)
	}

	snapshot, err := harness.engine.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.DLQ) != 1 {
		t.Fatalf("dlq = %v, want one bucket", snapshot.DLQ)
	}
	bucket := snapshot.DLQ[0]
	if bucket.Kind != "alpha" || bucket.Reason != "decode_failed" || bucket.Count != 1 || bucket.Exhausted != 0 {
		t.Errorf("dlq bucket = %+v, want alpha/decode_failed count 1 exhausted 0", bucket)
	}
}

// TestSnapshot_EmptyAfterStop: once the dispatcher is gone the scheduling half
// comes back empty, but the budget and DLQ (which need no dispatcher) still read.
func TestSnapshot_EmptyAfterStop(t *testing.T) {
	harness := newHarness(t, []enrichment.JobDefinition{fakeDefinition("alpha", nil)}, pausedMachine(), 2, 2)
	harness.engine.Start(context.Background())
	harness.engine.Stop()

	snapshot, err := harness.engine.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Kinds) != 0 || len(snapshot.InFlight) != 0 {
		t.Errorf("scheduling half = %+v / %v, want empty after stop", snapshot.Kinds, snapshot.InFlight)
	}
	if snapshot.Budget.Capacity != 2 {
		t.Errorf("budget still reports after stop: capacity = %d, want 2", snapshot.Budget.Capacity)
	}
}

// TestSnapshot_InFlightAndHotBand: a gated producer pins exactly one job running
// (single worker), so the snapshot's InFlight carries that job with real content
// (asset, kind, non-zero Started) and the budget gauge moves off zero; hinting the
// rest promotes them into the hot band, so QueuedHot reflects the viewport set.
// This is the populated-shape half the empty-state tests can't reach.
func TestSnapshot_InFlightAndHotBand(t *testing.T) {
	started := make(chan string, 8)
	gate := make(chan struct{})
	definition := fakeDefinition("fake", func(ctx context.Context, asset *domain.Asset, _ func()) (enrichment.ApplyFunc, error) {
		started <- asset.ID
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return applyThumbnailAt(asset.ID), nil
	})
	definition.DefaultWorkers = 1 // one worker → exactly one job in flight
	harness := newHarness(t, []enrichment.JobDefinition{definition}, normalMachine(), 4, 5)
	harness.start(t)

	pinned := <-started // one job held in Produce; the other four sit queued (cold)
	snapshot, err := harness.engine.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.InFlight) != 1 {
		t.Fatalf("in-flight = %+v, want exactly one running job", snapshot.InFlight)
	}
	job := snapshot.InFlight[0]
	if job.AssetID != pinned || job.Kind != "fake" || job.Started.IsZero() {
		t.Errorf("in-flight job = %+v, want asset %s / kind fake / non-zero Started", job, pinned)
	}
	if snapshot.Budget.InUse < 1 {
		t.Errorf("budget in-use = %d, want ≥1 while a job holds a token", snapshot.Budget.InUse)
	}
	fake := kindGauge(t, &snapshot, "fake")
	if fake.Running != 1 || fake.QueuedCold != 4 || fake.QueuedHot != 0 {
		t.Errorf("gauge = %+v, want 1 running / 4 cold / 0 hot before hinting", fake)
	}

	// Hint every asset: the four queued jobs promote into the hot band; the running
	// one is skipped (already dispatched), so hot settles at four.
	ids := make([]string, len(harness.assets))
	for index, asset := range harness.assets {
		ids[index] = asset.ID
	}
	harness.engine.Hint(ids)
	waitUntil(t, "hint promotes the queued band to hot", func() bool {
		snapshot, err := harness.engine.Snapshot(context.Background())
		return err == nil && kindGauge(t, &snapshot, "fake").QueuedHot == 4
	})

	close(gate) // release the pinned job; the backlog drains
	waitUntil(t, "backlog drains after release", func() bool { return harness.missingCount(t) == 0 })
}

// TestSnapshot_RaceUnderDispatch: reads the snapshot in a tight loop while a real
// backlog drains. Under -race this proves the snapshot never touches
// dispatcher-owned state off the dispatcher goroutine.
func TestSnapshot_RaceUnderDispatch(t *testing.T) {
	harness := newHarness(t, []enrichment.JobDefinition{fakeDefinition("alpha", nil)}, normalMachine(), 4, 40)
	harness.start(t)

	var group sync.WaitGroup
	stop := make(chan struct{})
	for range 3 {
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if _, err := harness.engine.Snapshot(context.Background()); err != nil {
						t.Errorf("Snapshot during dispatch: %v", err)
						return
					}
				}
			}
		}()
	}
	waitUntil(t, "backlog drains", func() bool { return harness.missingCount(t) == 0 })
	close(stop)
	group.Wait()
}
