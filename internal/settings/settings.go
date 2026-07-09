// Package settings owns the three plain-JSON config files that back user
// settings, machine config, and keybinding overrides (impl/11). It is a LEAF:
// it imports no other internal package, no DB, no catalog. Consumers read a
// cached struct via Get and react to external/UI edits via OnChange; the
// composition root (cmd/dev today, the app host later) is the only caller.
//
//   - <catalog-dir>/settings.json    — catalog-scoped, opened alongside the catalog
//   - <app-config-dir>/machine.json  — machine-scoped, process lifetime
//   - <app-config-dir>/keybindings.json — user-scoped, process lifetime
//
// Storage is files, not a DB table, precisely so a user can hand-edit them while
// the app runs; every open file is watched and hot-reloaded (see configfile.go).
package settings

import (
	"encoding/json"
	"path/filepath"

	"github.com/charmbracelet/log"
)

// Settings is catalog-scoped, at <catalog-dir>/settings.json.
//
// Only fields with a live consumer live here — this is a YAGNI checkpoint (Ari,
// 2026-07-07). Removed until something reads them: catalogBackupCount, undoStackSize,
// updateCheckEnabled, defaultSortField/Dir (no query layer to inject them yet).
type Settings struct {
	ThumbnailQuality      int             `json:"thumbnailQuality"`      // JPEG quality 1..100 (thumbnailer)
	ImportBatchSize       int             `json:"importBatchSize"`       // rows per WRITE transaction
	IgnorePatterns        []string        `json:"ignorePatterns"`        // D18 — plain, user-editable array
	XMPWriteBack          bool            `json:"xmpWriteBack"`          // write catalog judgments → sidecars (impl/06)
	XMPConflictResolution string          `json:"xmpConflictResolution"` // "xmp_wins" (default) | "catalog_wins"
	UI                    json.RawMessage `json:"ui,omitempty"`          // frontend-owned, opaque to Go
}

// Machine is machine-scoped, at <app-config-dir>/machine.json.
type Machine struct {
	Workers         WorkerCounts      `json:"workers"`
	DependencyPaths map[string]string `json:"dependencyPaths,omitempty"`
	OpenInApps      map[string]string `json:"openInApps,omitempty"`
}

// WorkerCounts nests per pipeline — the nesting IS the "Ingest" prefix, for free.
type WorkerCounts struct {
	Ingest IngestWorkers `json:"ingest"`
}

type IngestWorkers struct {
	Hash    int `json:"hash"`
	Extract int `json:"extract"`
	Thumb   int `json:"thumb"`
}

// Keybindings is user-scoped, at <app-config-dir>/keybindings.json. Overrides
// only — the backend never interprets a commandID or chord, that vocabulary is
// the frontend command registry's.
type Keybindings map[string]string // commandID -> chord

// DefaultSettings — used on first run and as the per-field fallback for bad values.
func DefaultSettings() Settings {
	return Settings{
		ThumbnailQuality: 80, // matches thumbnailer.DefaultQuality — keep in sync
		ImportBatchSize:  50, // matches importer.defaultBatchSize — keep in sync
		IgnorePatterns: []string{
			".DS_Store", "._*", ".Spotlight-V100", ".Trashes", ".fseventsd", // macOS
			"Thumbs.db", "desktop.ini", // Windows
			"@eaDir", ".AppleDouble", // NAS
			"*.tmp", "*.temp", "*.part", "*.crdownload", "*.download", // in-flight writes
		},
	}
}

// DefaultMachine — worker defaults mirror importer.defaultPools (hash=4/extract=2/thumb=2).
func DefaultMachine() Machine {
	return Machine{Workers: WorkerCounts{Ingest: IngestWorkers{Hash: 4, Extract: 2, Thumb: 2}}}
}

// Service is the composition root's single handle onto all three files.
type Service struct {
	Settings    *configFile[Settings]    // per-catalog: opened alongside the catalog, closed with it
	Machine     *configFile[Machine]     // process lifetime: opened once at startup, before any catalog
	Keybindings *configFile[Keybindings] // process lifetime
}

// Open opens all three config files in dir, each created with defaults on first
// run (the same strategy sqlite.Open uses for the catalog DB). Returns the
// composition root's handle; Close it at shutdown.
//
// dir today is <catalog-dir>/settings — all three files colocated with the
// catalog, which is what the dev harness wires. machine.json and keybindings.json
// are app-scoped by design (they belong in an OS app-config dir and should outlive
// any single catalog); they sit here provisionally until the app-host milestone
// resolves <app-config-dir> and splits them out.
func Open(dir string, logger *log.Logger) (*Service, error) {
	settingsFile, err := OpenSettings(dir, logger)
	if err != nil {
		return nil, err
	}
	machineFile, err := OpenMachine(dir, logger)
	if err != nil {
		settingsFile.Close()
		return nil, err
	}
	keybindingsFile, err := OpenKeybindings(dir, logger)
	if err != nil {
		settingsFile.Close()
		machineFile.Close()
		return nil, err
	}
	return &Service{Settings: settingsFile, Machine: machineFile, Keybindings: keybindingsFile}, nil
}

// Close stops every file's hot-reload watch.
func (s *Service) Close() {
	s.Settings.Close()
	s.Machine.Close()
	s.Keybindings.Close()
}

// OpenSettings opens (or first-runs) <dir>/settings.json, the catalog-scoped file.
func OpenSettings(dir string, logger *log.Logger) (*configFile[Settings], error) {
	return openConfigFile(filepath.Join(dir, "settings.json"), DefaultSettings(), sanitizeSettings, logger)
}

// OpenMachine opens (or first-runs) <dir>/machine.json.
func OpenMachine(dir string, logger *log.Logger) (*configFile[Machine], error) {
	return openConfigFile(filepath.Join(dir, "machine.json"), DefaultMachine(), sanitizeMachine, logger)
}

// OpenKeybindings opens (or first-runs) <dir>/keybindings.json. No sanitizer — a
// bare override map has no numeric fields to clamp.
func OpenKeybindings(dir string, logger *log.Logger) (*configFile[Keybindings], error) {
	return openConfigFile(filepath.Join(dir, "keybindings.json"), Keybindings{}, nil, logger)
}

// sanitizeSettings clamps present-but-bad fields back to defaults, so one bad
// value never nukes the file's other good fields. Same "non-positive means unset,
// use default" convention importer.resolvePools already applies to worker counts.
func sanitizeSettings(settings Settings, logger *log.Logger) Settings {
	defaults := DefaultSettings()
	clamp := func(name string, field *int, fallback int) {
		if *field <= 0 {
			logger.Debug("settings: non-positive field, using default", "field", name, "got", *field)
			*field = fallback
		}
	}
	clamp("thumbnailQuality", &settings.ThumbnailQuality, defaults.ThumbnailQuality)
	clamp("importBatchSize", &settings.ImportBatchSize, defaults.ImportBatchSize)
	return settings
}

// sanitizeMachine clamps non-positive worker counts back to defaults.
func sanitizeMachine(machine Machine, logger *log.Logger) Machine {
	defaults := DefaultMachine().Workers.Ingest
	ingest := &machine.Workers.Ingest
	clamp := func(name string, field *int, fallback int) {
		if *field <= 0 {
			logger.Debug("machine: non-positive worker count, using default", "field", name, "got", *field)
			*field = fallback
		}
	}
	clamp("workers.ingest.hash", &ingest.Hash, defaults.Hash)
	clamp("workers.ingest.extract", &ingest.Extract, defaults.Extract)
	clamp("workers.ingest.thumb", &ingest.Thumb, defaults.Thumb)
	return machine
}
