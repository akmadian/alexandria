package watcher

import (
	"context"
	"io/fs"
	"time"
)

// defaultPoll is how often a unit stat-probes its root for mount state (and, when
// poll-driven, re-walks to catch changes). Remount is noticed within one interval
// — the deliberate ceiling of the "volume monitor is just the poll timer" collapse
// (impl/05 reconciled plan). ponytail: per-OS mount daemons (DiskArbitration /
// mountinfo-epoll) would make it instant — add only when that latency is measured
// to matter.
const defaultPoll = 30 * time.Second

// probeReachable is the pure root-stat probe: the source is reachable if its root
// still stats. This is the whole "volume monitor" — one stat, every OS, no cgo.
//
// ponytail: a plain stat misses an unmount that leaves an empty mountpoint behind
// (the path still stats, just empty). Distinguishing that needs the filesystem-UUID
// monitor the spec defers; until then a catch-up reconcile heals it on the next
// real event or poll.
func probeReachable(fsys fs.FS) bool {
	_, err := fs.Stat(fsys, ".")
	return err == nil
}

// poll is the connectivity monitor — the watcher's ONE catalog write lives here.
// It stat-probes the root every interval and drives the events⇄polling⇄offline
// state machine:
//
//   - reachable → unreachable: flip the source's assets online→offline (the one
//     sanctioned write; NEVER missing — the files are presumed intact) and quiesce.
//     The graduate gate checks w.offline, so the event loop stops feeding paths
//     while the volume is gone (otherwise every vanished path would be marked
//     missing).
//   - unreachable → reachable: flip back online and schedule a catch-up reconcile
//     (the importer's full walk) so anything that changed while offline converges.
//     From here on the unit is poll-driven — we do NOT re-subscribe live events
//     (deferred); the periodic reconcile below is the change detector.
//   - reachable while poll-driven (subscribe failed at start, or post-remount):
//     re-walk each tick, since there are no live events to lean on.
//
// pollDriven starts true when the event subscribe failed (inotify watch-limit →
// degrade to polling, never crash).
//
// ponytail: the source-level sources.connectivity column (SourceRepo.SetConnectivity)
// is NOT written here — no consumer reads it yet (the P3 health panel is deferred).
// MarkConnectivityBySource (asset file_status) is what keeps assets browsable-but-
// offline, which is the only observable requirement today. Add the source-column
// flip when the status bar consumes it.
func (w *Watcher) poll(ctx context.Context, fsys fs.FS, pollDriven bool) error {
	interval := w.PollInterval
	if interval == 0 {
		interval = defaultPoll
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			reachable := probeReachable(fsys)
			switch {
			case !reachable && !w.offline.Load():
				w.offline.Store(true)
				if err := w.Obs.MarkConnectivityBySource(ctx, w.Source.ID, false); err != nil {
					w.Log.Error("watcher: mark offline failed", "source", w.Source.Name, "err", err)
				}
				w.Log.Warn("watcher: source offline — quiescing", "source", w.Source.Name)

			case reachable && w.offline.Load():
				w.offline.Store(false)
				if err := w.Obs.MarkConnectivityBySource(ctx, w.Source.ID, true); err != nil {
					w.Log.Error("watcher: mark online failed", "source", w.Source.Name, "err", err)
				}
				w.Log.Info("watcher: source online — catch-up reconcile", "source", w.Source.Name)
				pollDriven = true // stay poll-driven; we don't re-subscribe live events
				if _, err := w.Ingester.Run(ctx, w.Source, fsys); err != nil {
					w.Log.Error("watcher: catch-up reconcile failed", "err", err)
				}

			case reachable && pollDriven:
				// No live events (degraded or post-remount): the periodic walk IS the
				// change detector.
				w.Log.Debug("watcher: poll reconcile (no live events)", "source", w.Source.Name)
				if _, err := w.Ingester.Run(ctx, w.Source, fsys); err != nil {
					w.Log.Error("watcher: poll reconcile failed", "err", err)
				}
			}
		}
	}
}
