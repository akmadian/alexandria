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

// ingestSpy implements Ingester and records the calls, so tests assert on what
// the watcher decided without touching a real catalog.
type ingestSpy struct {
	mu       sync.Mutex
	runs     int
	ingested []string
	missing  []string
}

func (s *ingestSpy) Run(context.Context, *domain.Source, fs.FS) (importer.ImportResult, error) {
	s.mu.Lock()
	s.runs++
	s.mu.Unlock()
	return importer.ImportResult{}, nil
}

func (s *ingestSpy) IngestFile(_ context.Context, _ *domain.Source, _ fs.FS, name string) error {
	s.mu.Lock()
	s.ingested = append(s.ingested, name)
	s.mu.Unlock()
	return nil
}

func (s *ingestSpy) MarkMissing(_ context.Context, _ *domain.Source, relPath string) error {
	s.mu.Lock()
	s.missing = append(s.missing, relPath)
	s.mu.Unlock()
	return nil
}

func (s *ingestSpy) snapshot() (runs int, ingested, missing []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs, append([]string(nil), s.ingested...), append([]string(nil), s.missing...)
}

func newTestWatcher(t *testing.T, root string) (*Watcher, *ingestSpy, chan Event) {
	t.Helper()
	events := make(chan Event, 16)
	spy := &ingestSpy{}
	w := &Watcher{
		Ingester: spy,
		Source:   &domain.Source{ID: "s1", Name: "test"},
		Root:     root,
		Log:      log.New(io.Discard),
		Debounce: 40 * time.Millisecond,
		events:   func(context.Context, string) (<-chan Event, error) { return events, nil },
	}
	return w, spy, events
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, why string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", why)
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSaveStorm_CollapsesToOneIngest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "photo.jpg", "stable-bytes")

	w, spy, events := newTestWatcher(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx) //nolint:errcheck // clean shutdown returns context.Canceled

	// Temp write + rename + double-write, all inside the debounce window, all for
	// the same final path — must become exactly one ingest.
	for i := 0; i < 4; i++ {
		events <- Event{Path: "photo.jpg"}
		time.Sleep(5 * time.Millisecond)
	}

	waitFor(t, "one ingest", func() bool {
		_, ingested, _ := spy.snapshot()
		return len(ingested) == 1
	})
	// Give any stray extra graduation time to (wrongly) land.
	time.Sleep(150 * time.Millisecond)
	_, ingested, _ := spy.snapshot()
	if len(ingested) != 1 || ingested[0] != "photo.jpg" {
		t.Fatalf("save storm must collapse to exactly one ingest, got %v", ingested)
	}
}

func TestIgnoreList_NeverEntersDebouncer(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "real.jpg", "bytes")
	writeFile(t, root, "scratch.tmp", "bytes")

	w, spy, events := newTestWatcher(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx) //nolint:errcheck

	events <- Event{Path: "scratch.tmp"} // ignored at intake
	events <- Event{Path: "real.jpg"}    // proves the loop is alive

	waitFor(t, "real file ingested", func() bool {
		_, ingested, _ := spy.snapshot()
		return len(ingested) == 1
	})
	_, ingested, _ := spy.snapshot()
	for _, p := range ingested {
		if p == "scratch.tmp" {
			t.Fatalf(".tmp file must never be ingested, got %v", ingested)
		}
	}
}

func TestDeleteHint_MarksMissing(t *testing.T) {
	root := t.TempDir() // gone.jpg deliberately does NOT exist on disk

	w, spy, events := newTestWatcher(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx) //nolint:errcheck

	events <- Event{Path: "gone.jpg"}

	waitFor(t, "path marked missing", func() bool {
		_, _, missing := spy.snapshot()
		return len(missing) == 1 && missing[0] == "gone.jpg"
	})
	_, ingested, _ := spy.snapshot()
	if len(ingested) != 0 {
		t.Fatalf("a vanished path must not be ingested, got %v", ingested)
	}
}

func TestOverflow_TriggersReconcile(t *testing.T) {
	root := t.TempDir()
	w, spy, events := newTestWatcher(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx) //nolint:errcheck

	// Startup reconcile is the first Run; overflow forces a second.
	waitFor(t, "startup reconcile", func() bool { r, _, _ := spy.snapshot(); return r == 1 })
	events <- Event{Overflow: true}
	waitFor(t, "overflow reconcile", func() bool { r, _, _ := spy.snapshot(); return r == 2 })
}
