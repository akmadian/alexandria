package domain

// SyncMode is a tracked Folder's per-root sync level (D41; DEFERRED §1's ruling).
// It is edge routing, not a new path through the pipeline stages:
//
//   - manual:    no watcher, no timer. Fidelity is caught only by an explicit
//     "Synchronize Folder" or a launch reconcile; new files are never auto-added.
//   - watched:   the live watcher — auto-add + live fidelity + live XMP.
//   - scheduled: a periodic reconcile (add + fidelity), the poll timer.
//
// The engine already runs all three; sync_mode only decides what happens
// automatically. It rides on tracked ROOTS only (a derived subfolder inherits
// its root's mode); the per-subtree override is deferred (DEFERRED §19).
type SyncMode string

const (
	SyncModeManual    SyncMode = "manual"
	SyncModeWatched   SyncMode = "watched"
	SyncModeScheduled SyncMode = "scheduled"
)
