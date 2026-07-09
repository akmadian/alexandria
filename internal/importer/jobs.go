package importer

import (
	"context"
	"sync"

	"github.com/akmadian/alexandria/internal/domain"
)

// Jobs is the entire v1 job system (D17): a jobID → cancel-func map. No queue,
// no persistence — an import is idempotent, so "durable job state" buys nothing
// (killing a run and re-running is the recovery mechanism). River replaces this
// only when genuinely durable background work arrives, behind the same envelope.
//
// ponytail: map + mutex, no worker pool, no priorities. Add River when durable
// jobs (thumb rebuild at scale, transcription) actually land.
type Jobs struct {
	mu      sync.Mutex
	running map[string]context.CancelFunc
}

func NewJobs() *Jobs { return &Jobs{running: map[string]context.CancelFunc{}} }

// Start launches work in a goroutine under a cancelable context and returns its
// jobID. work receives the jobID so it can stamp progress events with it (a
// small deviation from the design sketch's fn(ctx) — threading the id beats a
// return-value race). The job deregisters itself on return.
func (j *Jobs) Start(kind string, work func(ctx context.Context, jobID string)) string {
	jobID := domain.NewID()
	ctx, cancel := context.WithCancel(context.Background())

	j.mu.Lock()
	if j.running == nil {
		j.running = map[string]context.CancelFunc{}
	}
	j.running[jobID] = cancel
	j.mu.Unlock()

	go func() {
		defer cancel()
		defer j.finish(jobID)
		work(ctx, jobID)
	}()
	return jobID
}

// Cancel cancels a running job (no-op if unknown/already finished).
func (j *Jobs) Cancel(jobID string) {
	j.mu.Lock()
	cancel := j.running[jobID]
	j.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (j *Jobs) finish(jobID string) {
	j.mu.Lock()
	if cancel := j.running[jobID]; cancel != nil {
		cancel() // release the context's resources
		delete(j.running, jobID)
	}
	j.mu.Unlock()
}

// Progress is emitted per batch commit and per walk completion. Total is
// indeterminate until the walk finishes (TotalKnown flips true then), which is
// what upgrades a UI progress bar from spinner to bar without a counting pre-pass.
type Progress struct {
	JobID      string
	Kind       string
	Done       int // assets committed so far
	Total      int // files emitted for processing so far
	TotalKnown bool
	Stage      string
}
