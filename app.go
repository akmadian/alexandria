package main

import (
	"context"

	"github.com/charmbracelet/log"

	"github.com/akmadian/alexandria/internal/app"
	"github.com/akmadian/alexandria/internal/seam"
	"github.com/akmadian/alexandria/internal/sqlite"
)

// host is the app-host composition root: it owns the process-lifetime catalog
// handle and the seam services bound to the webview. It stays deliberately thin
// — resolve the catalog, open it, construct seam services, expose them.
// Everything that only matters once the process stays up (background integrity
// check, backup-before-migration, watcher supervision, live pool resize) belongs
// to impl/12 and grows this same host in place; this is that host seeded minimal
// (seam impl/14 §2, decision 3).
type host struct {
	catalog *sqlite.Catalog
	sources *seam.SourceService
}

// newHost runs the minimal startup sequence: resolve the catalog directory, then
// open it. sqlite.Open acquires the single-instance lock, opens SQLite in WAL
// mode with the crash-safety pragmas, and migrates to the latest schema — so the
// two hard exits of the startup sequence (cannot open, cannot migrate) surface
// here as an error, before the window opens. Wiring the rest of the sequence
// (dir-resolution UI, integrity check, backup, watcher supervision, app:ready)
// is impl/12's work, grown on this same host.
func newHost() (*host, error) {
	dir, err := app.CatalogDir()
	if err != nil {
		return nil, err
	}
	log.Info("opening catalog", "dir", dir)
	catalog, err := sqlite.Open(dir)
	if err != nil {
		return nil, err
	}
	return &host{
		catalog: catalog,
		sources: seam.NewSourceService(&sqlite.SourceRepo{DB: catalog.DB}),
	}, nil
}

// boundServices is the list Wails binds and generates TypeScript for. Each new
// seam service (impl/15 method surface, impl/16 events & jobs) joins this slice
// — one line, no new seam plumbing.
func (h *host) boundServices() []any {
	return []any{h.sources}
}

// onStartup fires once the webview context exists. impl/12 grows the post-window
// startup steps here (background integrity check, watcher supervision, the
// app:ready event); for now it only logs readiness.
func (h *host) onStartup(_ context.Context) {
	log.Info("alexandria ready")
}

// onShutdown releases the catalog (DB handle + instance lock) on window close.
func (h *host) onShutdown(_ context.Context) {
	if err := h.catalog.Close(); err != nil {
		log.Error("closing catalog", "err", err)
	}
}
