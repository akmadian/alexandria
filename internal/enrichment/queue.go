package enrichment

import (
	"container/heap"
	"time"
)

// One jobQueue per node: the input priority queue holding that node's pending
// jobs. Ordering is a composite key expressed in Less — hinted band first (in
// hint order), then import recency — and this heap is the ONLY place dispatch
// order exists (D28: no priority column in the DB, ever). Hint promotion and
// demotion are heap.Fix calls; the queue never needs bespoke reorder flags.

const (
	priorityHinted = iota // the user is looking at it — drains first
	priorityNormal        // scan- or edge-derived backlog
)

// pendingJob is one queued-or-running job in the dispatcher's ledger. While
// queued it also sits in its node's jobQueue (heapIndex is the heap.Fix
// handle); dequeuing marks it running and removes it from the heap, but the
// ledger keeps it until the writer reports completion — that is the dedup.
type pendingJob struct {
	key        JobKey
	priority   int
	hintRank   int       // position within the hint set; meaningful only when hinted
	ingestedAt time.Time // import recency — newest first within the normal band
	heapIndex  int
	running    bool
}

// jobQueue implements heap.Interface over pendingJob pointers.
type jobQueue struct {
	entries []*pendingJob
}

func (q *jobQueue) Len() int { return len(q.entries) }

func (q *jobQueue) Less(leftIndex, rightIndex int) bool {
	left, right := q.entries[leftIndex], q.entries[rightIndex]
	if left.priority != right.priority {
		return left.priority < right.priority
	}
	if left.priority == priorityHinted && left.hintRank != right.hintRank {
		return left.hintRank < right.hintRank
	}
	if !left.ingestedAt.Equal(right.ingestedAt) {
		return left.ingestedAt.After(right.ingestedAt) // newest ingest first
	}
	return left.key.AssetID < right.key.AssetID // deterministic tiebreak
}

func (q *jobQueue) Swap(leftIndex, rightIndex int) {
	q.entries[leftIndex], q.entries[rightIndex] = q.entries[rightIndex], q.entries[leftIndex]
	q.entries[leftIndex].heapIndex = leftIndex
	q.entries[rightIndex].heapIndex = rightIndex
}

func (q *jobQueue) Push(item any) {
	entry := item.(*pendingJob) //nolint:forcetypeassert // heap.Interface contract; only enqueue calls Push
	entry.heapIndex = len(q.entries)
	q.entries = append(q.entries, entry)
}

func (q *jobQueue) Pop() any {
	last := len(q.entries) - 1
	entry := q.entries[last]
	q.entries[last] = nil // release the reference; the ledger still holds it
	q.entries = q.entries[:last]
	entry.heapIndex = -1
	return entry
}

// enqueue adds a pending job in priority position.
func (q *jobQueue) enqueue(pending *pendingJob) { heap.Push(q, pending) }

// dequeue pops the highest-priority job and marks it running; nil when empty.
func (q *jobQueue) dequeue() *pendingJob {
	if q.Len() == 0 {
		return nil
	}
	pending := heap.Pop(q).(*pendingJob) //nolint:forcetypeassert // heap holds only *pendingJob
	pending.running = true
	return pending
}

// promote lifts a queued job into the hinted band at the given hint rank —
// the user is now looking at its asset.
func (q *jobQueue) promote(pending *pendingJob, hintRank int) {
	pending.priority = priorityHinted
	pending.hintRank = hintRank
	heap.Fix(q, pending.heapIndex)
}

// demote returns a queued job to the normal band — its hint generation was
// replaced. Its import recency still orders it there.
func (q *jobQueue) demote(pending *pendingJob) {
	pending.priority = priorityNormal
	heap.Fix(q, pending.heapIndex)
}
