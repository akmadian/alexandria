package enrichment_test

import (
	"context"
	"testing"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/enrichment"
	"github.com/akmadian/alexandria/internal/sqlite"
)

// TestRunningKinds_ReverseAndSparse: the tracker bitmask reverses to kind names,
// only in-flight assets appear (sparse), and clearing a bit removes it.
func TestRunningKinds_ReverseAndSparse(t *testing.T) {
	harness := newHarness(t, []enrichment.JobDefinition{
		fakeDefinition("alpha", nil), fakeDefinition("beta", nil),
	}, pausedMachine(), 1, 3)
	engine := harness.engine
	first, second, idle := harness.assets[0].ID, harness.assets[1].ID, harness.assets[2].ID

	engine.Tracker().SetRunning(first, engine.KindBit("alpha"))
	engine.Tracker().SetRunning(second, engine.KindBit("alpha")|engine.KindBit("beta"))

	running := engine.RunningKinds([]string{first, second, idle})
	if got := running[first]; len(got) != 1 || got[0] != domain.EnrichmentKind("alpha") {
		t.Errorf("first running = %v, want [alpha]", got)
	}
	if got := running[second]; len(got) != 2 {
		t.Errorf("second running = %v, want two kinds", got)
	}
	if _, present := running[idle]; present {
		t.Error("idle asset must be absent (sparse)")
	}

	engine.Tracker().ClearRunning(first, engine.KindBit("alpha"))
	if _, present := engine.RunningKinds([]string{first})[first]; present {
		t.Error("cleared asset must be gone")
	}
}

// TestFailedKinds_ExhaustedOnly: only an attempt-exhausted DLQ row reads as
// failed; a row still under the retry budget stays pending (not an eternal
// spinner, but not yet terminally failed either).
func TestFailedKinds_ExhaustedOnly(t *testing.T) {
	harness := newHarness(t, []enrichment.JobDefinition{fakeDefinition("alpha", nil)}, pausedMachine(), 1, 2)
	repo := &sqlite.EnrichmentRepo{DB: harness.db}
	exhausted, retrying := harness.assets[0].ID, harness.assets[1].ID
	for attempt := 0; attempt < enrichment.MaxAttempts; attempt++ {
		if err := repo.LogFailure(context.Background(), exhausted, "alpha", "decode_failed", "bad"); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.LogFailure(context.Background(), retrying, "alpha", "decode_failed", "bad"); err != nil {
		t.Fatal(err)
	}

	failed, err := harness.engine.FailedKinds(context.Background(), []string{exhausted, retrying})
	if err != nil {
		t.Fatal(err)
	}
	if got := failed[exhausted]; len(got) != 1 || got[0] != domain.EnrichmentKind("alpha") {
		t.Errorf("exhausted failed = %v, want [alpha]", got)
	}
	if _, present := failed[retrying]; present {
		t.Error("a non-exhausted failure must not read as failed")
	}
}

// TestQueueDepths_ReflectsBacklog: paused so nothing drains, the on-start scan
// enqueues every eligible asset, and QueueDepths reports it per kind.
func TestQueueDepths_ReflectsBacklog(t *testing.T) {
	harness := newHarness(t, []enrichment.JobDefinition{fakeDefinition("alpha", nil)}, pausedMachine(), 1, 4)
	harness.start(t)
	waitUntil(t, "backlog enqueued", func() bool {
		return harness.engine.QueueDepths()["alpha"] == 4
	})
}

// TestQueueDepths_EmptyAfterStop: once the dispatcher is gone, the read falls
// through the stopped context to an empty map rather than blocking.
func TestQueueDepths_EmptyAfterStop(t *testing.T) {
	harness := newHarness(t, []enrichment.JobDefinition{fakeDefinition("alpha", nil)}, pausedMachine(), 1, 2)
	harness.engine.Start(context.Background())
	harness.engine.Stop()
	if depths := harness.engine.QueueDepths(); len(depths) != 0 {
		t.Errorf("QueueDepths after Stop = %v, want empty", depths)
	}
}

// TestFailedKinds_NoneWhenNoExhaustion: no DLQ rows → no failed kinds (the
// empty-result branch), and the batch read returns cleanly.
func TestFailedKinds_NoneWhenNoExhaustion(t *testing.T) {
	harness := newHarness(t, []enrichment.JobDefinition{fakeDefinition("alpha", nil)}, pausedMachine(), 1, 1)
	failed, err := harness.engine.FailedKinds(context.Background(), []string{harness.assets[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 0 {
		t.Errorf("no DLQ rows must yield no failed kinds, got %v", failed)
	}
}
