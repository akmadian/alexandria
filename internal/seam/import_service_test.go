package seam_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/fstest"
	"time"

	"github.com/charmbracelet/log"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/importer"
	"github.com/akmadian/alexandria/internal/seam"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
)

// fakeSourceGetter resolves a single source (or an error), standing in for the
// source repo so the import tests need no database.
type fakeSourceGetter struct {
	source *domain.Source
	err    error
}

func (f *fakeSourceGetter) Get(_ context.Context, _ string) (*domain.Source, error) {
	return f.source, f.err
}

func onlineSource() *domain.Source {
	return &domain.Source{ID: "src-1", BasePath: "/tmp/whatever", Connectivity: domain.SourceOnline}
}

// waitFor polls until cond is true or the deadline passes — the events an import
// job emits arrive from a goroutine, so tests synchronize on the captured set
// rather than sleeping a fixed interval.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func lastEvent(events []capturedEvent) capturedEvent {
	return events[len(events)-1]
}

// TestStartImport_EmitsProgressThenDone is the C9 end-to-end acceptance (spec §6):
// a run reports progress ticks and a terminal done carrying the summary. The
// injected run stands in for the real pipeline so no walk or DB is needed.
func TestStartImport_EmitsProgressThenDone(t *testing.T) {
	emitter := &fakeEmitter{}
	run := func(_ context.Context, jobID string, _ *domain.Source, onProgress func(importer.Progress)) (importer.ImportResult, error) {
		onProgress(importer.Progress{JobID: jobID, Kind: "import", Done: 5, Total: 0, TotalKnown: false})
		onProgress(importer.Progress{JobID: jobID, Kind: "import", Done: 10, Total: 10, TotalKnown: true})
		return importer.ImportResult{Added: 10, Skipped: 2}, nil
	}
	service := seam.NewImportService(&fakeSourceGetter{source: onlineSource()}, importer.NewJobs(), run, emitter)

	jobID, err := service.StartImport("src-1")
	if err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	if jobID == "" {
		t.Fatal("StartImport should return a job id")
	}

	waitFor(t, func() bool {
		types := emitter.typesOf()
		return len(types) > 0 && types[len(types)-1] == seam.EventJobDone
	})

	events := emitter.snapshot()
	// Expect: progress, progress, done — in that order.
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3 (2 progress + 1 done): %+v", len(events), emitter.typesOf())
	}
	first, ok := events[0].Payload.(seam.JobProgress)
	if !ok {
		t.Fatalf("first event payload = %T, want JobProgress", events[0].Payload)
	}
	if first.TotalKnown {
		t.Error("first progress should be spinner state (TotalKnown false)")
	}
	if first.State != seam.JobStateRunning || !first.Cancelable || first.Label != "jobs.kind.import" {
		t.Errorf("progress payload wrong: %+v", first)
	}
	done, ok := lastEvent(events).Payload.(seam.JobDone)
	if !ok {
		t.Fatalf("last event payload = %T, want JobDone", lastEvent(events).Payload)
	}
	if done.State != seam.JobStateDone {
		t.Errorf("done state = %q, want done", done.State)
	}
	if done.Summary == nil || done.Summary.Added != 10 || done.Summary.Skipped != 2 {
		t.Errorf("done summary wrong: %+v", done.Summary)
	}
}

// TestStartImport_CancelEmitsCancelled covers the cancel acceptance: CancelJob mid
// run drives a terminal done with state cancelled. The injected run blocks until
// its context is cancelled, so the test controls the timing deterministically.
func TestStartImport_CancelEmitsCancelled(t *testing.T) {
	emitter := &fakeEmitter{}
	started := make(chan struct{})
	run := func(ctx context.Context, _ string, _ *domain.Source, _ func(importer.Progress)) (importer.ImportResult, error) {
		close(started)
		<-ctx.Done() // block until CancelJob fires
		return importer.ImportResult{Added: 3}, ctx.Err()
	}
	service := seam.NewImportService(&fakeSourceGetter{source: onlineSource()}, importer.NewJobs(), run, emitter)

	jobID, err := service.StartImport("src-1")
	if err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	<-started
	if err := service.CancelJob(jobID); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}

	waitFor(t, func() bool {
		types := emitter.typesOf()
		return len(types) > 0 && types[len(types)-1] == seam.EventJobDone
	})
	done := lastEvent(emitter.snapshot()).Payload.(seam.JobDone)
	if done.State != seam.JobStateCancelled {
		t.Errorf("done state = %q, want cancelled", done.State)
	}
	// A cancelled run still reports what it committed.
	if done.Summary == nil || done.Summary.Added != 3 {
		t.Errorf("cancelled summary should carry partial progress: %+v", done.Summary)
	}
}

// TestStartImport_FailureEmitsFailed covers a non-cancel run error → state failed
// with the diagnostic detail attached.
func TestStartImport_FailureEmitsFailed(t *testing.T) {
	emitter := &fakeEmitter{}
	run := func(_ context.Context, _ string, _ *domain.Source, _ func(importer.Progress)) (importer.ImportResult, error) {
		return importer.ImportResult{}, errors.New("catalog exploded")
	}
	service := seam.NewImportService(&fakeSourceGetter{source: onlineSource()}, importer.NewJobs(), run, emitter)

	if _, err := service.StartImport("src-1"); err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	waitFor(t, func() bool {
		types := emitter.typesOf()
		return len(types) > 0 && types[len(types)-1] == seam.EventJobDone
	})
	done := lastEvent(emitter.snapshot()).Payload.(seam.JobDone)
	if done.State != seam.JobStateFailed {
		t.Errorf("done state = %q, want failed", done.State)
	}
	if done.Error == "" {
		t.Error("failed done should carry diagnostic detail")
	}
}

// TestNewImportService_NilEmitterUsesNop confirms a nil emitter is replaced by the
// safe no-op sink — StartImport must run to completion without panicking even when
// no one is listening.
func TestNewImportService_NilEmitterUsesNop(t *testing.T) {
	progressed := make(chan struct{})
	run := func(_ context.Context, jobID string, _ *domain.Source, onProgress func(importer.Progress)) (importer.ImportResult, error) {
		onProgress(importer.Progress{JobID: jobID}) // routed to the nop sink
		close(progressed)
		return importer.ImportResult{}, nil
	}
	service := seam.NewImportService(&fakeSourceGetter{source: onlineSource()}, importer.NewJobs(), run, nil)

	if _, err := service.StartImport("src-1"); err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	<-progressed // the nop emitter handled at least the progress event without panic
}

// TestStartImport_OfflineSourceRejected covers the up-front guard: an offline
// source returns source_offline before any job starts, and emits nothing.
func TestStartImport_OfflineSourceRejected(t *testing.T) {
	emitter := &fakeEmitter{}
	offline := &domain.Source{ID: "src-1", BasePath: "/tmp/x", Connectivity: domain.SourceOffline}
	run := func(context.Context, string, *domain.Source, func(importer.Progress)) (importer.ImportResult, error) {
		t.Fatal("run must not be called for an offline source")
		return importer.ImportResult{}, nil
	}
	service := seam.NewImportService(&fakeSourceGetter{source: offline}, importer.NewJobs(), run, emitter)

	_, err := service.StartImport("src-1")
	assertDomainCode(t, err, "source_offline")
	if len(emitter.snapshot()) != 0 {
		t.Error("a rejected import must emit nothing")
	}
}

// TestStartImport_RealImporterEndToEnd is the spec §6 acceptance with a REAL
// importer (not a fake run): a seeded filesystem is ingested through the actual
// pipeline, progress events arrive, and jobs/done carries a summary whose Added
// count matches the rows committed to the DB. This is what proves the
// Progress→envelope and ImportResult→summary mappings against real engine output,
// where the ImportService unit tests only prove the dispatch logic. The app host's
// os.DirFS + thumbnailer wiring (app.go host.runImport) stays a wails-dev concern;
// here the run closure mirrors it over an in-memory FS.
func TestStartImport_RealImporterEndToEnd(t *testing.T) {
	db := testutil.NewTestDB(t)
	source := testutil.NewTestSource(t, db, "photos")
	assets := &sqlite.AssetRepo{DB: db}
	fsys := fstest.MapFS{
		"a.jpg":     {Data: []byte("jpeg-a")},
		"b.png":     {Data: []byte("png-b")},
		"notes.txt": {Data: []byte("unsupported")}, // skipped by the scanner
	}
	run := func(ctx context.Context, jobID string, src *domain.Source, onProgress func(importer.Progress)) (importer.ImportResult, error) {
		ingester := &importer.Importer{
			Reader:     assets,
			Obs:        assets,
			Derived:    assets,
			Dups:       &sqlite.DuplicateRepo{DB: db},
			Store:      sqlite.NewStore(db),
			Imports:    &sqlite.ImportRepo{DB: db},
			Log:        log.New(io.Discard),
			OnProgress: onProgress,
		}
		return ingester.RunJob(ctx, jobID, src, fsys)
	}
	emitter := &fakeEmitter{}
	service := seam.NewImportService(&fakeSourceGetter{source: source}, importer.NewJobs(), run, emitter)

	if _, err := service.StartImport(source.ID); err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	waitFor(t, func() bool {
		types := emitter.typesOf()
		return len(types) > 0 && types[len(types)-1] == seam.EventJobDone
	})

	events := emitter.snapshot()
	// At least one real progress tick plus the terminal done.
	var sawProgress bool
	for _, e := range events {
		if _, ok := e.Payload.(seam.JobProgress); ok {
			sawProgress = true
		}
	}
	if !sawProgress {
		t.Fatal("real import should emit at least one progress event")
	}
	done, ok := lastEvent(events).Payload.(seam.JobDone)
	if !ok || done.State != seam.JobStateDone {
		t.Fatalf("terminal event = %+v, want a done JobDone", lastEvent(events).Payload)
	}
	if done.Summary == nil || done.Summary.Added != 2 {
		t.Fatalf("summary Added = %v, want 2 (jpg+png, txt skipped)", done.Summary)
	}
	// The summary must match what actually landed in the DB.
	var committed int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM assets WHERE is_deleted=0").Scan(&committed); err != nil {
		t.Fatal(err)
	}
	if committed != done.Summary.Added {
		t.Fatalf("DB has %d assets but summary reported %d added", committed, done.Summary.Added)
	}
}
