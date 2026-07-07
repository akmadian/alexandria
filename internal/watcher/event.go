package watcher

// Event is a normalized filesystem hint about one path under the watched root.
// It is deliberately thin: the watcher re-derives what actually happened by
// stat-ing the path at graduation (events are hints, the filesystem is truth —
// D14), so a create, write, delete, and rename all arrive the same way — as
// "this path is dirty". Path is a slash path relative to the root (the fs.FS
// convention the importer's single-path entry expects).
//
// Overflow is the one exception that isn't about a single path: the OS told us
// it dropped events for the tree. There is no per-path recovery, so it triggers
// the universal fallback — drop the dirty set and reconcile (a full walk).
type Event struct {
	Path     string
	Overflow bool
}
