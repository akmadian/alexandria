package watcher

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/settings"
	"github.com/charmbracelet/log"
	"github.com/rjeczalik/notify"
)

// fakeEventInfo is a stand-in for the OS backend's notify.EventInfo, carrying just
// the absolute path normalize() reads.
type fakeEventInfo struct{ path string }

func (f fakeEventInfo) Event() notify.Event { return 0 }
func (f fakeEventInfo) Path() string        { return f.path }
func (f fakeEventInfo) Sys() interface{}    { return nil }

// normalize turns an absolute OS event path into a root-relative slash path, and
// drops anything that escapes the watched tree — the guard that keeps a stray
// path from ever being ingested.
func TestNormalize(t *testing.T) {
	const root = "/data/photos"
	cases := []struct {
		name    string
		abs     string
		wantRel string
		wantOK  bool
	}{
		{"nested", "/data/photos/trip/a.jpg", "trip/a.jpg", true},
		{"root file", "/data/photos/a.jpg", "a.jpg", true},
		{"escapes tree", "/data/other/x.jpg", "", false},
		{"parent", "/data", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event, ok := normalize(root, fakeEventInfo{path: tc.abs})
			if ok != tc.wantOK {
				t.Fatalf("normalize(%q) ok = %v, want %v", tc.abs, ok, tc.wantOK)
			}
			if ok && event.Path != tc.wantRel {
				t.Errorf("normalize(%q) rel = %q, want %q", tc.abs, event.Path, tc.wantRel)
			}
		})
	}
}

func TestIsXMPSidecar(t *testing.T) {
	for _, rel := range []string{"a.xmp", "dir/b.XMP", "deep/nested/c.Xmp"} {
		if !isXMPSidecar(rel) {
			t.Errorf("%q should be recognized as an xmp sidecar", rel)
		}
	}
	for _, rel := range []string{"a.jpg", "b.xmp.jpg", "notxmp"} {
		if isXMPSidecar(rel) {
			t.Errorf("%q should NOT be an xmp sidecar", rel)
		}
	}
}

// A graduating .xmp fires the SidecarChanged callback (with the absolute + relative
// paths) AND falls through to IngestFile so the sidecar_files row stays tracked —
// the D15 "sync the sidecar, but still record it" path.
func TestWatcher_SidecarHintFiresCallbackAndIngests(t *testing.T) {
	root := t.TempDir()
	events := make(chan Event, 8)
	spy := newSpy()

	type call struct{ abs, rel string }
	fired := make(chan call, 1)

	w := &Watcher{
		Ingester: spy,
		Source:   &domain.Source{ID: "src-1", Name: "test"},
		Root:     root,
		Log:      log.New(io.Discard),
		Debounce: 30 * time.Millisecond,
		Settings: settings.DefaultSettings(),
		SidecarChanged: func(_ context.Context, _ *domain.Source, abs, rel string) {
			fired <- call{abs, rel}
		},
		events: func(context.Context, string) (<-chan Event, error) { return events, nil },
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = w.Run(ctx) }()
	spy.waitFor(t, func(runs int, _ []string) bool { return runs == 1 }) // startup reconcile

	write(t, root, "photo.xmp", []byte("<x:xmpmeta/>"))
	events <- Event{Path: "photo.xmp"}

	select {
	case c := <-fired:
		if c.rel != "photo.xmp" {
			t.Errorf("callback rel = %q, want photo.xmp", c.rel)
		}
		if !strings.HasSuffix(c.abs, "photo.xmp") {
			t.Errorf("callback abs = %q, should end with photo.xmp", c.abs)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sidecar callback never fired")
	}

	// It still hands the path to the importer to keep the sidecar row tracked.
	spy.waitFor(t, func(_ int, ingests []string) bool {
		for _, n := range ingests {
			if n == "photo.xmp" {
				return true
			}
		}
		return false
	})
}
