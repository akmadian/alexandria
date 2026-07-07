// Package watcher turns filesystem events into ingest hints. It is a SENSOR, not
// an actor (D14, impl/05 corrected model): it owns the dirty-path set, the
// debounce, and the settle-stat, and on graduation it hands the importer a PATH,
// never a verdict. The importer's single-path entry re-stats that path and owns
// the catalog action (present → ingest; gone → mark missing + delete-side merge),
// so a watcher-fed change heals identically to a walk-detected one. On any failure
// (overflow, kill-9) the watcher schedules a reconcile — the importer's full walk.
// Event loss therefore degrades freshness, never correctness.
//
// The watcher makes NO per-file catalog write. (Its one sanctioned write —
// sources.connectivity — arrives with the 05.3 poll timer, not here.)
package watcher

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/importer"
	"github.com/charmbracelet/log"
)

const (
	// defaultDebounce is the quiet period a path must go without a new event
	// before it graduates. It absorbs creative-app save storms (temp write +
	// rename + re-write) into a single handoff.
	defaultDebounce = 500 * time.Millisecond
	// settleWindow is the gap between the double-stat that confirms a present file
	// has stopped changing before we hand it off.
	settleWindow = 50 * time.Millisecond
	// eventBuffer sizes the event channels. Generous, because graduation briefly
	// blocks the loop (settle sleep + handoff) and we would rather buffer than let
	// notify drop — a full buffer is safe anyway (reconcile is the backstop).
	eventBuffer = 256
)

// Ingester is the slice of the importer the watcher drives. importer.Importer
// satisfies it. It is exactly two seams and nothing more: a full walk (the
// reconcile schedule) and a single-path handoff. There is NO MarkMissing / no
// "fidelity" surface — the corrected model routes a gone path through IngestFile
// like any other path, and the importer decides to mark it missing. Keeping the
// seam this small is what stops the sensor from acting.
type Ingester interface {
	// Run is the reconcile: the importer's full walk (impl/05 "reconcile is a
	// schedule, not a component"). Used at startup and on overflow.
	Run(ctx context.Context, source *domain.Source, fsys fs.FS) (importer.ImportResult, error)
	// IngestFile hands over ONE settled/gone path. The importer stats it and
	// decides the action; present → ingest, gone → mark missing + delete-side merge.
	IngestFile(ctx context.Context, source *domain.Source, fsys fs.FS, name string) error
}

// Watcher watches one source's tree and keeps its catalog rows fresh. One instance
// per source; supervising many is a higher layer's job (the app-host supervisor,
// DEFERRED §2), not the watcher's.
type Watcher struct {
	Ingester Ingester
	Source   *domain.Source
	Root     string // absolute path of the source root (watched, and the DirFS base)
	Log      *log.Logger
	Debounce time.Duration // 0 → defaultDebounce

	// events is the event-source seam. nil → notifyEvents (the real OS backend).
	// Tests set a fake source here.
	events func(ctx context.Context, root string) (<-chan Event, error)
}

// Run does a startup reconcile, then watches until ctx is cancelled. The startup
// reconcile is the kill-9 recovery path: whatever changed while the watcher was
// down is converged by a full walk before live watching begins. Returns
// context.Canceled on a clean shutdown.
func (w *Watcher) Run(ctx context.Context) error {
	// Canonicalize the root: on macOS /var is a symlink to /private/var, and
	// FSEvents reports the resolved path — so a symlinked root would make every
	// event look like it escaped the tree (filepath.Rel → "../…") and get dropped.
	// Resolving once keeps the watch root and event paths in the same namespace.
	root := w.Root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	fsys := os.DirFS(root)

	w.Log.Info("watcher: startup reconcile", "source", w.Source.Name, "root", root)
	if _, err := w.Ingester.Run(ctx, w.Source, fsys); err != nil {
		return fmt.Errorf("startup reconcile: %w", err)
	}

	source := w.events
	if source == nil {
		source = notifyEvents
	}
	events, err := source(ctx, root)
	if err != nil {
		return fmt.Errorf("watching %s: %w", w.Root, err)
	}
	w.Log.Info("watcher: watching", "root", root)
	return w.loop(ctx, fsys, events)
}

// loop is the single goroutine that owns the dirty set. Per-path timers are the
// debounce; a fired timer posts the path to graduated. Owning the map in one
// goroutine keeps it lock-free.
func (w *Watcher) loop(ctx context.Context, fsys fs.FS, events <-chan Event) error {
	delay := w.Debounce
	if delay == 0 {
		delay = defaultDebounce
	}
	timers := map[string]*time.Timer{}
	graduated := make(chan string, eventBuffer)

	// arm (re)starts the debounce timer for a path. A path already in the set just
	// resets its timer — that dedup is what collapses a save storm to one handoff.
	arm := func(relPath string) {
		if timer, ok := timers[relPath]; ok {
			timer.Reset(delay)
			return
		}
		timers[relPath] = time.AfterFunc(delay, func() {
			select {
			case graduated <- relPath:
			case <-ctx.Done():
			}
		})
	}
	stopAll := func() {
		for _, timer := range timers {
			timer.Stop()
		}
		timers = map[string]*time.Timer{}
	}
	defer stopAll()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event, ok := <-events:
			if !ok {
				return nil // source closed (its ctx was cancelled)
			}
			if event.Overflow {
				// One answer to every failure mode: drop the set, re-derive by a
				// full walk. Missed events cannot cause divergence past this point.
				w.Log.Warn("watcher: event overflow — dropping dirty set, reconciling")
				stopAll()
				if _, err := w.Ingester.Run(ctx, w.Source, fsys); err != nil {
					w.Log.Error("watcher: overflow reconcile failed", "err", err)
				}
				continue
			}
			if importer.Ignored(path.Base(event.Path)) {
				continue // ignore-list at intake: a .tmp storm never enters the set
			}
			arm(event.Path)

		case relPath := <-graduated:
			// ponytail: a late event can Reset a timer that already fired, so a path
			// may graduate twice. Harmless — re-feeding an unchanged path is a no-op
			// reimport, and a repeat gone-path is a no-op mark-missing. Not worth a
			// generation counter to prevent.
			delete(timers, relPath)
			if w.graduate(ctx, fsys, relPath) {
				arm(relPath) // still being written — re-debounce
			}
		}
	}
}

// graduate times the handoff for one settled path; it does NOT decide the catalog
// action. The stat here answers only: is this a directory (noise, skip), is a
// present file still changing (re-debounce), or is it ready to hand over? A gone
// path has no settle to do — it's terminal, and falls straight through. Either
// way the PATH goes to the importer's single-path entry, which re-stats it and
// owns the decision (present → ingest, gone → mark missing). The watcher never
// turns its stat result into a verdict. Returns true if the path is still
// changing and should be re-debounced.
func (w *Watcher) graduate(ctx context.Context, fsys fs.FS, relPath string) (requeue bool) {
	if info, err := fs.Stat(fsys, relPath); err == nil {
		if info.IsDir() {
			return false // directory events are noise; its files graduate on their own
		}
		if !settled(fsys, relPath, info) {
			return true // mid-write: come back after it stops changing
		}
	}
	// Settled-present, gone, or a transient stat error: hand the path over. On a
	// transient error the importer re-stats, returns an error we log, and the next
	// reconcile/poll heals — still no action decided here.
	if err := w.Ingester.IngestFile(ctx, w.Source, fsys, relPath); err != nil {
		w.Log.Error("watcher: ingest failed", "path", relPath, "err", err)
	}
	return false
}

// settled confirms a present file has stopped changing: a second stat after a
// short gap must show the same size and mtime as the first. A file still being
// written by a creative app fails this and is re-debounced.
//
// ponytail: the settleWindow sleep runs in the loop goroutine, so a burst of many
// distinct files serializes ~50ms each. Fine for v1; move settle+handoff to a
// worker queue if graduation throughput ever matters.
func settled(fsys fs.FS, relPath string, first fs.FileInfo) bool {
	time.Sleep(settleWindow)
	second, err := fs.Stat(fsys, relPath)
	if err != nil {
		return false
	}
	return first.Size() == second.Size() && first.ModTime().Equal(second.ModTime())
}
