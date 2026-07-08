package settings

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/rjeczalik/notify"
)

// reloadDebounce collapses the burst of fsnotify events editors emit per save
// (write-then-rename, in-place multi-write) into one reload. Same order of
// magnitude as impl/06's outbound-write debounce.
const reloadDebounce = 300 * time.Millisecond

// configFile wraps ONE JSON file end to end: tolerant load, cached in-memory
// value, atomic save, debounced hot-reload watch, and change notification.
// Settings, Machine and Keybindings are all *configFile[T] — one generic, not
// three hand-written services with the same shape (D19).
type configFile[T any] struct {
	path      string
	logger    *log.Logger
	sanitize  func(T, *log.Logger) T // nil = no per-field clamping
	watchStop context.CancelFunc

	mu       sync.RWMutex
	cached   T
	onChange []func(T)
}

// openConfigFile subscribes the watch FIRST, then cold-loads — closing the TOCTOU
// gap where an external edit landing between the initial read and watch startup
// would be missed. A watch-subscribe failure degrades to no hot-reload (logged),
// never a hard error: a missing config watch must not block startup.
func openConfigFile[T any](path string, defaults T, sanitize func(T, *log.Logger) T, logger *log.Logger) (*configFile[T], error) {
	if logger == nil {
		logger = log.Default()
	}
	// A child logger scoped to this file — every line it emits carries the readable
	// filename (config=settings.json), the same pattern the pipeline uses per asset.
	// The path stops being repeated field-by-field.
	logger = logger.With("config", filepath.Base(path))
	c := &configFile[T]{path: path, logger: logger, sanitize: sanitize}

	_, statErr := os.Stat(path)
	firstRun := errors.Is(statErr, os.ErrNotExist)
	c.cached = c.apply(loadJSON(path, defaults, logger))

	// Materialize the file with defaults on first run — same strategy as the
	// catalog DB, which sqlite.Open creates on open. A MALFORMED file is NOT
	// recreated here (loadJSON quarantined it): leaving it absent lets the user
	// inspect the .invalid-* copy, and the next Save writes a fresh one.
	if firstRun {
		if data, err := marshalConfig(c.cached); err != nil {
			logger.Warn("could not encode default config", "err", err)
		} else if err := atomicWrite(path, data); err != nil {
			logger.Warn("could not create default config file", "err", err)
		} else {
			logger.Info("created default config file")
		}
	}

	c.startWatch()
	return c, nil
}

// apply runs the sanitizer if one is set.
func (c *configFile[T]) apply(value T) T {
	if c.sanitize != nil {
		return c.sanitize(value, c.logger)
	}
	return value
}

// Get returns the cached value. No per-lookup file read.
func (c *configFile[T]) Get() T {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cached
}

// OnChange registers a callback fired after any change (UI Save or external edit).
func (c *configFile[T]) OnChange(fn func(T)) {
	c.mu.Lock()
	c.onChange = append(c.onChange, fn)
	c.mu.Unlock()
}

// Save marshals indented (these files are meant to be hand-edited), writes atomically
// (temp file + rename, so the watcher never sees a half-written file), updates the
// cache and fires callbacks. Creates the parent dir on first save.
func (c *configFile[T]) Save(value T) error {
	data, err := marshalConfig(value)
	if err != nil {
		return err
	}
	if err := atomicWrite(c.path, data); err != nil {
		return err
	}
	c.set(value)
	c.logger.Info("config saved")
	return nil
}

// marshalConfig encodes indented — these files are meant to be hand-edited.
func marshalConfig[T any](value T) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}

// atomicWrite writes data to a temp file in the same dir and renames it into
// place, so a reader (or our own watcher) never observes a half-written file.
// Creates the parent dir on first write.
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// set updates the cache and fires callbacks with the callbacks snapshotted outside
// any held lock (a callback must be free to call Get without deadlocking).
func (c *configFile[T]) set(value T) {
	c.mu.Lock()
	c.cached = value
	callbacks := append([]func(T){}, c.onChange...)
	c.mu.Unlock()
	for _, fn := range callbacks {
		fn(value)
	}
}

// Close stops the hot-reload watch. Call for settings.json when its catalog closes;
// machine/keybindings live for the process and need no Close until shutdown.
func (c *configFile[T]) Close() {
	if c.watchStop != nil {
		c.watchStop()
	}
}

// startWatch watches the file's PARENT directory (not the file), so an editor's
// atomic save — which replaces the file via rename and would sever a file-inode
// watch — is still observed. Events for other files in the dir are filtered out.
func (c *configFile[T]) startWatch() {
	events := make(chan notify.EventInfo, 16)
	// No "/..." suffix — a non-recursive watch of just the parent directory.
	if err := notify.Watch(filepath.Dir(c.path), events, notify.Create, notify.Write, notify.Rename); err != nil {
		c.logger.Warn("config hot-reload unavailable (watch failed), edits need a restart", "err", err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.watchStop = cancel
	go c.watchLoop(ctx, events)
}

// watchLoop debounces events for this file and reloads on the trailing edge.
func (c *configFile[T]) watchLoop(ctx context.Context, events chan notify.EventInfo) {
	defer notify.Stop(events)
	base := filepath.Base(c.path)
	var timer *time.Timer
	var fire <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			if filepath.Base(event.Path()) != base {
				continue // some other file in the directory
			}
			if timer == nil {
				timer = time.NewTimer(reloadDebounce)
			} else {
				timer.Reset(reloadDebounce)
			}
			fire = timer.C
		case <-fire:
			fire = nil
			c.reload()
		}
	}
}

// reload is the live path: keep serving last-known-good on any failure, only
// update + fire callbacks when the file actually parsed and changed.
func (c *configFile[T]) reload() {
	current := c.Get()
	value, ok := reloadJSON(c.path, current, c.logger)
	if !ok {
		return
	}
	value = c.apply(value)
	if jsonEqual(value, current) {
		return // e.g. our own Save round-tripping, or a no-op external touch
	}
	c.logger.Info("config reloaded after external edit")
	c.set(value)
}

// loadJSON is the COLD-START path: no prior in-memory value exists, so a malformed
// file must still produce something to boot with. Missing file = defaults (first
// run, no noise). Bad JSON = quarantine the file and revert to defaults.
func loadJSON[T any](path string, defaults T, logger *log.Logger) T {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults
	}
	if err != nil {
		logger.Warn("reading config, using defaults", "err", err)
		return defaults
	}
	value := defaults
	if err := json.Unmarshal(data, &value); err != nil {
		quarantine(path, logger)
		logger.Warn("config file invalid JSON, quarantined and reverted to defaults", "err", err)
		return defaults
	}
	return value
}

// reloadJSON is the LIVE path: a good in-memory value already exists, so failure
// KEEPS serving it (no quarantine, file left untouched — the user may be mid-edit
// and about to save a fixed version). bool = "did it actually parse to a new value".
func reloadJSON[T any](path string, current T, logger *log.Logger) (T, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Warn("re-reading config after external edit, keeping previous values", "err", err)
		return current, false
	}
	value := current
	if err := json.Unmarshal(data, &value); err != nil {
		logger.Warn("external edit produced invalid JSON, keeping previous configuration", "err", err)
		return current, false
	}
	return value, true
}

// quarantine renames a corrupt file to path+".invalid-<UTC timestamp>" — preserved
// for the user, not deleted. A fresh valid file is NOT auto-written; only the next
// explicit Save writes one.
func quarantine(path string, logger *log.Logger) {
	dest := path + ".invalid-" + time.Now().UTC().Format("20060102T150405Z")
	if err := os.Rename(path, dest); err != nil {
		logger.Warn("could not quarantine invalid config", "err", err)
		return
	}
	logger.Warn("quarantined invalid config", "saved_as", filepath.Base(dest))
}

// jsonEqual compares two values by their marshaled form — enough to tell a real
// change from a no-op reload without a per-type equality method.
func jsonEqual[T any](a, b T) bool {
	aJSON, err1 := json.Marshal(a)
	bJSON, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(aJSON) == string(bJSON)
}
