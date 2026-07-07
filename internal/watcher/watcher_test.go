package watcher

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/importer"
	"github.com/charmbracelet/log"
)

// spyIngester records the seam calls the watcher makes. It is the whole point of
// the narrow Ingester seam: we assert the watcher hands over PATHS (and schedules
// reconciles) without needing a real catalog. Present-vs-gone handling of a fed
// path is the importer's job, proven in importer_test.
type spyIngester struct {
	mu      sync.Mutex
	runs    int
	ingests []string
	fired   chan struct{} // pinged after every recorded call
}

func newSpy() *spyIngester { return &spyIngester{fired: make(chan struct{}, 64)} }

func (s *spyIngester) Run(context.Context, *domain.Source, fs.FS) (importer.ImportResult, error) {
	s.mu.Lock()
	s.runs++
	s.mu.Unlock()
	s.ping()
	return importer.ImportResult{}, nil
}

func (s *spyIngester) IngestFile(_ context.Context, _ *domain.Source, _ fs.FS, name string) error {
	s.mu.Lock()
	s.ingests = append(s.ingests, name)
	s.mu.Unlock()
	s.ping()
	return nil
}

func (s *spyIngester) ping() {
	select {
	case s.fired <- struct{}{}:
	default:
	}
}

func (s *spyIngester) snapshot() (int, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs, append([]string(nil), s.ingests...)
}

// waitFor polls until cond() or the deadline; keeps the timing-based tests from
// flaking without a fixed sleep.
func (s *spyIngester) waitFor(t *testing.T, cond func(runs int, ingests []string) bool) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if runs, ingests := s.snapshot(); cond(runs, ingests) {
			return
		}
		select {
		case <-deadline:
			runs, ingests := s.snapshot()
			t.Fatalf("condition not met: runs=%d ingests=%v", runs, ingests)
		case <-s.fired:
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// startWatcher runs a watcher over a real temp dir (graduate stats real files)
// with a caller-driven event source and a fast debounce. Returns the temp dir,
// the event channel to push onto, and the spy.
func startWatcher(t *testing.T) (string, chan Event, *spyIngester) {
	t.Helper()
	root := t.TempDir()
	events := make(chan Event, 64)
	spy := newSpy()
	w := &Watcher{
		Ingester: spy,
		Source:   &domain.Source{ID: "src-1", Name: "test"},
		Root:     root,
		Log:      log.New(io.Discard),
		Debounce: 30 * time.Millisecond,
		events:   func(context.Context, string) (<-chan Event, error) { return events, nil },
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go w.Run(ctx)
	// The startup reconcile is one Run before any event.
	spy.waitFor(t, func(runs int, _ []string) bool { return runs == 1 })
	return root, events, spy
}

func write(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A save-storm (many events for one path inside the debounce window) collapses to
// exactly one handoff.
func TestWatcher_SaveStormCollapsesToOneIngest(t *testing.T) {
	root, events, spy := startWatcher(t)
	write(t, root, "photo.jpg", []byte("bytes"))

	for i := 0; i < 5; i++ {
		events <- Event{Path: "photo.jpg"}
		time.Sleep(5 * time.Millisecond) // resets the timer each time, well under 30ms
	}
	spy.waitFor(t, func(_ int, ingests []string) bool { return len(ingests) == 1 })

	// Give any stray second graduation a chance to show up, then confirm it didn't.
	time.Sleep(80 * time.Millisecond)
	if _, ingests := spy.snapshot(); len(ingests) != 1 || ingests[0] != "photo.jpg" {
		t.Fatalf("save-storm should collapse to one ingest of photo.jpg, got %v", ingests)
	}
}

// A gone path is handed over just like a present one — the watcher does not decide
// mark-missing, it feeds the path and lets the importer stat it.
func TestWatcher_GonePathHandedOver(t *testing.T) {
	_, events, spy := startWatcher(t)
	// No file on disk at this path: graduate stats it (ErrNotExist), skips settle,
	// and hands the path to IngestFile all the same.
	events <- Event{Path: "deleted.jpg"}
	spy.waitFor(t, func(_ int, ingests []string) bool {
		return len(ingests) == 1 && ingests[0] == "deleted.jpg"
	})
}

// Overflow drops the dirty set and schedules a reconcile (a second Run).
func TestWatcher_OverflowReconciles(t *testing.T) {
	root, events, spy := startWatcher(t)
	write(t, root, "photo.jpg", []byte("bytes"))

	events <- Event{Path: "photo.jpg"} // arm a path...
	events <- Event{Overflow: true}    // ...then overflow before it graduates
	spy.waitFor(t, func(runs int, _ []string) bool { return runs == 2 })

	// The armed path was dropped by the overflow, so it must NOT also graduate.
	time.Sleep(80 * time.Millisecond)
	if _, ingests := spy.snapshot(); len(ingests) != 0 {
		t.Fatalf("overflow should drop the dirty set, but paths still graduated: %v", ingests)
	}
}

// Ignore-list is checked at intake: a .tmp storm never enters the debouncer.
func TestWatcher_IgnoreListAtIntake(t *testing.T) {
	root, events, spy := startWatcher(t)
	write(t, root, "scratch.tmp", []byte("in-flight"))
	write(t, root, "photo.jpg", []byte("bytes"))

	events <- Event{Path: "scratch.tmp"} // ignored — never armed
	events <- Event{Path: "photo.jpg"}   // real — graduates
	spy.waitFor(t, func(_ int, ingests []string) bool { return len(ingests) == 1 })

	time.Sleep(80 * time.Millisecond)
	if _, ingests := spy.snapshot(); len(ingests) != 1 || ingests[0] != "photo.jpg" {
		t.Fatalf("the .tmp must be ignored at intake; want only photo.jpg, got %v", ingests)
	}
}
