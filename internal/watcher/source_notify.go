package watcher

import (
	"context"
	"path/filepath"

	"github.com/rjeczalik/notify"
)

// notifyEvents subscribes recursively to root and streams normalized Events until
// ctx is cancelled (at which point it stops the OS watch and closes the channel).
//
// It is the ONE file that touches the OS event backend. rjeczalik/notify wraps
// the three platform APIs the design names — recursive FSEvents (macOS, not
// kqueue), inotify (Linux), ReadDirectoryChangesW (Windows) — behind a single
// type, chosen over three hand-rolled cgo adapters (see impl/05 reconciled plan).
// Everything downstream speaks the local Event type, so replacing notify later is
// a change to this file alone.
//
// ponytail: notify does not surface a portable "events dropped" signal, so this
// source never emits Event{Overflow}. The startup reconcile (and the 05.3 poll
// timer) are the real safety net for missed events; the Overflow path exists for
// that timer and for tests to drive.
func notifyEvents(ctx context.Context, root string) (<-chan Event, error) {
	raw := make(chan notify.EventInfo, eventBuffer)
	// The "/..." suffix asks notify for a recursive watch of the whole subtree.
	pattern := filepath.Join(root, "...")
	if err := notify.Watch(pattern, raw, notify.Create, notify.Write, notify.Remove, notify.Rename); err != nil {
		return nil, err
	}
	out := make(chan Event, eventBuffer)
	go func() {
		defer close(out)
		defer notify.Stop(raw)
		for {
			select {
			case <-ctx.Done():
				return
			case info := <-raw:
				event, ok := normalize(root, info)
				if !ok {
					continue
				}
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// normalize turns an absolute notify path into a root-relative slash path. A path
// outside root (filepath.Rel fails or escapes) is dropped — the watch is scoped
// to the tree, so that should not happen, but a bad path is never ingested.
func normalize(root string, info notify.EventInfo) (Event, bool) {
	relative, err := filepath.Rel(root, info.Path())
	if err != nil || relative == ".." || len(relative) >= 2 && relative[:2] == ".." {
		return Event{}, false
	}
	return Event{Path: filepath.ToSlash(relative)}, true
}
