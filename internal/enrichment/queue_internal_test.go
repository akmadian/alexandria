package enrichment

import (
	"testing"
	"time"
)

// Direct unit tests for the jobQueue's composite ordering — the pure core
// (§1) the full-engine harness can't pin: the harness's assets share one
// truncated timestamp and its hints carry one asset, so the hint-rank and
// ingest-recency tie-break tiers of Less would otherwise go unverified.

func pendingFor(assetID string, priority, hintRank int, ingestedAt time.Time) *pendingJob {
	return &pendingJob{
		key:        JobKey{AssetID: assetID, Kind: "fake"},
		priority:   priority,
		hintRank:   hintRank,
		ingestedAt: ingestedAt,
	}
}

func drainOrder(queue *jobQueue) []string {
	var order []string
	for {
		pending := queue.dequeue()
		if pending == nil {
			return order
		}
		order = append(order, pending.key.AssetID)
	}
}

func TestJobQueueOrderingAcrossAllTiers(t *testing.T) {
	now := time.Now().UTC()
	queue := &jobQueue{}
	// Enqueue deliberately scrambled: ties, bands, and ranks interleaved.
	tieOlderB := pendingFor("tie-b", priorityNormal, 0, now.Add(-3*time.Hour))
	hintedSecond := pendingFor("hinted-second", priorityHinted, 1, now.Add(-9*time.Hour)) // ancient ingest must not matter in the hinted band
	normalNewest := pendingFor("normal-newest", priorityNormal, 0, now)
	tieOlderA := pendingFor("tie-a", priorityNormal, 0, now.Add(-3*time.Hour))
	hintedFirst := pendingFor("hinted-first", priorityHinted, 0, time.Time{})
	normalOlder := pendingFor("normal-older", priorityNormal, 0, now.Add(-1*time.Hour))
	for _, pending := range []*pendingJob{tieOlderB, hintedSecond, normalNewest, tieOlderA, hintedFirst, normalOlder} {
		queue.enqueue(pending)
	}

	want := []string{
		"hinted-first", "hinted-second", // hinted band by hint rank, ingest irrelevant
		"normal-newest", "normal-older", // normal band by ingest recency
		"tie-a", "tie-b", // equal timestamps break deterministically by asset ID
	}
	got := drainOrder(queue)
	if len(got) != len(want) {
		t.Fatalf("drained %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("dequeue order = %v, want %v (first mismatch at %d)", got, want, index)
		}
	}
}

func TestJobQueuePromoteAndDemoteReorder(t *testing.T) {
	now := time.Now().UTC()
	queue := &jobQueue{}
	older := pendingFor("older", priorityNormal, 0, now.Add(-time.Hour))
	newer := pendingFor("newer", priorityNormal, 0, now)
	queue.enqueue(older)
	queue.enqueue(newer)

	// Promotion lifts the older entry over the newer one…
	queue.promote(older, 0)
	if first := queue.dequeue(); first.key.AssetID != "older" {
		t.Fatalf("promoted entry did not dequeue first, got %s", first.key.AssetID)
	}
	// …and demotion restores the normal band's recency order.
	queue.enqueue(older) // re-enqueue (fresh state for the demote leg)
	older.running = false
	queue.promote(older, 0)
	queue.demote(older)
	if first := queue.dequeue(); first.key.AssetID != "newer" {
		t.Fatalf("demoted entry outranked a newer normal entry, got %s", first.key.AssetID)
	}
}
