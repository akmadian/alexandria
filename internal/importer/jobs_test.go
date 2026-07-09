package importer_test

import (
	"context"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/importer"
)

// Start runs the work in a goroutine and threads the generated jobID into it, so
// the work can stamp progress events with the same id Start returned.
func TestJobs_StartRunsWorkWithMatchingID(t *testing.T) {
	jobs := importer.NewJobs()
	gotID := make(chan string, 1)

	id := jobs.Start("test", func(_ context.Context, jobID string) { gotID <- jobID })
	if id == "" {
		t.Fatal("Start returned an empty job id")
	}
	select {
	case g := <-gotID:
		if g != id {
			t.Fatalf("work received id %q, want the returned id %q", g, id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("work never ran")
	}
}

// Cancel cancels the running job's context, which is how a long import stops
// mid-flight.
func TestJobs_CancelPropagatesToContext(t *testing.T) {
	jobs := importer.NewJobs()
	started := make(chan struct{})
	canceled := make(chan struct{})

	id := jobs.Start("t", func(ctx context.Context, _ string) {
		close(started)
		<-ctx.Done()
		close(canceled)
	})

	<-started
	jobs.Cancel(id)

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("Cancel did not cancel the job's context")
	}
}

// Cancelling an unknown (or already-finished) job is a no-op, never a panic.
func TestJobs_CancelUnknownIsNoop(t *testing.T) {
	jobs := importer.NewJobs()
	jobs.Cancel("never-started")

	// A job that has already returned deregisters itself; cancelling it afterward
	// must also be safe.
	done := make(chan string, 1)
	id := jobs.Start("t", func(context.Context, string) {})
	// Give it a moment to finish and deregister.
	go func() {
		time.Sleep(50 * time.Millisecond)
		done <- id
	}()
	jobs.Cancel(<-done)
}
