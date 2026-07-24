package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"

	"github.com/akmadian/alexandria/internal/app"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/importer"
	"github.com/akmadian/alexandria/internal/seam"
	"github.com/akmadian/alexandria/internal/settings"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/thumbnailer"
	"github.com/akmadian/alexandria/internal/volume"
)

// host is the app-host composition root: it owns the process-lifetime catalog
// handle and the seam services bound to the webview. It stays deliberately thin
// — resolve the catalog, open it, construct seam services, expose them.
// Everything that only matters once the process stays up (background integrity
// check, backup-before-migration, watcher supervision, live pool resize) belongs
// to impl/12 and grows this same host in place; this is that host seeded minimal
// (seam impl/14 §2, decision 3).
type host struct {
	catalog     *sqlite.Catalog
	settings    *settings.Service
	thumbDir    string
	emitter     *seam.WailsEmitter
	resolver    *volume.Resolver
	volumes     *seam.VolumeService
	assets      *seam.AssetService
	collections *seam.CollectionService
	settingsAPI *seam.SettingsService
	imports     *seam.ImportService
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

	// Settings, keybindings, and machine config live beside the catalog (the same
	// layout the dev harness uses); impl/12 splits the app-scoped files into
	// <app-config-dir> when it resolves one.
	settingsService, err := settings.Open(filepath.Join(dir, "settings"), log.Default())
	if err != nil {
		_ = catalog.Close()
		return nil, err
	}

	assetRepo := &sqlite.AssetRepo{DB: catalog.DB}
	volumeRepo := &sqlite.VolumeRepo{DB: catalog.DB}
	folderRepo := &sqlite.FolderRepo{DB: catalog.DB}
	emitter := seam.NewWailsEmitter()

	newHost := &host{
		catalog:     catalog,
		settings:    settingsService,
		thumbDir:    filepath.Join(dir, "thumbnails"),
		emitter:     emitter,
		resolver:    volume.NewResolver(volumeRepo, volume.NewSystemProber(), log.Default()),
		volumes:     seam.NewVolumeService(volumeRepo),
		assets:      seam.NewAssetService(assetRepo, assetRepo, seam.WithEmitter(emitter)),
		collections: seam.NewCollectionService(&sqlite.CollectionRepo{DB: catalog.DB}, seam.WithEmitter(emitter)),
		settingsAPI: seam.NewSettingsService(settingsService.Settings, settingsService.Keybindings),
	}
	// ImportService needs a way to run an import that reports progress; the host
	// owns the pipeline's dependencies, so it supplies that closure (runImport). The
	// engine never imports Wails (D1) — it just hands over OnProgress and RunJob.
	newHost.imports = seam.NewImportService(folderRepo, volumeRepo, importer.NewJobs(), newHost.runImport, emitter)
	return newHost, nil
}

// runImport builds a wired importer for one run, routes its OnProgress to the
// seam's onProgress callback, and walks the source's filesystem root. It is the
// runImport the ImportService calls under the Jobs registry. The importer is built
// per run so its OnProgress closes over this call's callback with no shared state
// (mirrors cmd/dev's newIngester wiring).
func (h *host) runImport(ctx context.Context, jobID string, folder *domain.Folder, onProgress func(importer.Progress)) (importer.ImportResult, error) {
	// The session mount cache answers "where is this volume mounted right now".
	// ponytail: cold cache (app restarted, folder tracked in a prior session) errors
	// loudly here — the cold-start mount-enumeration layer is task-45 bind territory;
	// no import UI calls this path before then.
	mountPoint, err := h.resolver.MountPoint(ctx, folder.VolumeID)
	if err != nil {
		return importer.ImportResult{}, err
	}
	assetRepo := &sqlite.AssetRepo{DB: h.catalog.DB}
	set := h.settings.Settings.Get()
	thumb := thumbnailer.New(h.thumbDir)
	if set.ThumbnailQuality > 0 {
		thumb.Quality = set.ThumbnailQuality // settings owns JPEG quality
	}
	ingester := &importer.Importer{
		Reader:     assetRepo,
		Obs:        assetRepo,
		Dups:       &sqlite.DuplicateRepo{DB: h.catalog.DB},
		Store:      sqlite.NewStore(h.catalog.DB),
		Imports:    &sqlite.ImportRepo{DB: h.catalog.DB},
		Settings:   set,
		Machine:    h.settings.Machine.Get(),
		Log:        log.Default(),
		OnProgress: onProgress,
	}
	target := importer.Target{VolumeID: folder.VolumeID, WalkRoot: folder.Path, Name: folder.Name}
	return ingester.RunJob(ctx, jobID, target, os.DirFS(mountPoint))
}

// boundServices is the list Wails binds and generates TypeScript for. Each new
// seam service (impl/16 events & jobs) joins this slice — one line, no new seam
// plumbing.
func (h *host) boundServices() []any {
	return []any{h.volumes, h.assets, h.collections, h.settingsAPI, h.imports}
}

// onStartup fires once the webview context exists. It binds that context into the
// event emitter — before this, emits are dropped (no window to receive them). impl/12
// grows the rest of the post-window startup steps here (background integrity check,
// watcher supervision, the app:ready event).
func (h *host) onStartup(ctx context.Context) {
	h.emitter.Bind(ctx)
	log.Info("alexandria ready")
}

// onShutdown releases the settings watches and the catalog (DB handle + instance
// lock) on window close.
func (h *host) onShutdown(_ context.Context) {
	h.settings.Close()
	if err := h.catalog.Close(); err != nil {
		log.Error("closing catalog", "err", err)
	}
}
