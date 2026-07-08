// Command dev is Alexandria's engine harness: it drives the pipeline end-to-end
// and exposes its state, with full access to internal/*. It is the manual rig
// for the impl acceptance criteria (impl/08), not a shipped entrypoint.
//
// ponytail: stdlib flag, one file, subcommands only. The --debug HTTP server
// (pprof/expvar/statsviz/state dashboard) from impl/08 is deferred — the
// pipeline's acceptance is met with `import` + `errors` + `sessions` over a
// fixture tree; add the debug server when profiling a real workload needs it.
package main

import (
	"context"
	"database/sql"
	"errors"
	_ "expvar" // registers /debug/vars on http.DefaultServeMux
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* on http.DefaultServeMux
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/importer"
	"github.com/akmadian/alexandria/internal/migrations"
	"github.com/akmadian/alexandria/internal/settings"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/thumbnailer"
	"github.com/akmadian/alexandria/internal/watcher"
	"github.com/charmbracelet/log"
	_ "modernc.org/sqlite"
)

func main() {
	// The harness exists to see what the engine is doing, so debug logging is on
	// by default (no flag). Setting it as the package default too means leaf
	// packages that log via the global logger inherit the level.
	log.SetDefault(newLogger())

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	command, args := os.Args[1], os.Args[2:]

	pid := os.Getpid()
	fmt.Fprintf(os.Stderr, "alexandria dev harness (pid %d) — %s %v\n", pid, command, args)

	var err error
	switch command {
	case "import":
		err = cmdImport(args)
	case "reconcile":
		err = cmdReconcile(args)
	case "watch":
		err = cmdWatch(args)
	case "errors":
		err = cmdErrors(args)
	case "sessions":
		err = cmdSessions(args)
	case "rebuild":
		err = cmdRebuild(args)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `dev — Alexandria engine harness

usage:
  dev import <path>   [--catalog <dir|:memory:>] [--debug [--debug-addr <addr>]]
                      [--hash-workers N] [--extract-workers N] [--thumb-workers N]
  dev reconcile <path> [--catalog <dir>]           mark missing/restored files
  dev watch     <path> [--catalog <dir>]           live-watch a tree until Ctrl-C
  dev errors          [--catalog <dir>]            dump the latest session's DLQ
  dev sessions        [--catalog <dir>]            list recent import sessions
  dev rebuild fts     [--catalog <dir>]            rebuild the FTS index

worker pools (import):
  EXTRACT and THUMB (CPU-bound) default to ~75% of your cores — fast, but leaves
  headroom so the machine stays responsive (it can't lock up: the OS preempts and
  Go caps parallelism at NumCPU regardless). THUMB (decode + resize) is usually
  the wall for real photos. Override with --thumb-workers/--extract-workers, or
  pass 0 for the conservative engine default (hash=4 extract=2 thumb=2). Each
  in-flight THUMB holds a fully-decoded image, so more workers = more RAM; watch
  it under --debug (/debug/pprof/heap). The effective sizes print at start.

catalog storage (--catalog):
  :memory:   (default)  throwaway in-memory DB — fast, but nothing to browse afterward.
  <dir>                 a real catalog directory. The SQLite file is <dir>/catalog.db —
                        open that path in any DB viewer (sqlite3, Datasette, DB Browser)
                        to inspect assets, sidecars, sessions, and the DLQ. The import
                        prints the exact path on completion.

debug web UI (--debug, import only):
  Serves Go's pprof + expvar at http://localhost:6060/ during (and, so you can browse a
  fast run, after) the import. Override the address with --debug-addr, e.g.
    dev import ~/Photos --catalog ./cat --debug
    dev import ~/Photos --catalog ./cat --debug --debug-addr :6000
  /debug/pprof/goroutine?debug=1 is the live pipeline picture (who's blocked on which
  channel); /debug/vars is runtime + GC stats. Ctrl-C exits once the import is done.
`)
}

// parsePathAndFlags handles "<subcmd> <path> [--flags]": Go's flag package stops
// at the first positional, so we pull the path out and parse the flags that
// follow it. Returns ok=false if no positional path was given.
func parsePathAndFlags(flags *flag.FlagSet, args []string) (string, bool) {
	flags.Parse(args)
	if flags.NArg() < 1 {
		return "", false
	}
	path := flags.Arg(0)
	flags.Parse(flags.Args()[1:]) // flags that appeared after the path
	return path, true
}

// openedCatalog bundles an open catalog for the harness: the Store, a close
// func, a thumbnails dir, and the on-disk DB path (empty for :memory:, so tools
// can tell the user exactly what file to open in a viewer).
type openedCatalog struct {
	store       *sqlite.Store
	thumbDir    string
	dbPath      string
	settings    *settings.Service // nil for :memory: (no dir to colocate config in)
	settingsDir string            // empty for :memory:
	close       func() error
}

// openCatalog opens the catalog named by --catalog. ":memory:" (the default) is
// a throwaway single-connection DB — handy for a one-shot import; the persistent
// path is a real catalog directory (WAL + instance lock, via sqlite.Open) whose
// catalog.db is browsable in any SQLite viewer.
func openCatalog(catalogPath string) (*openedCatalog, error) {
	if catalogPath == ":memory:" {
		database, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
		if err != nil {
			return nil, err
		}
		database.SetMaxOpenConns(1)
		if err := migrations.Migrate(database); err != nil {
			database.Close()
			return nil, err
		}
		return &openedCatalog{
			store:    sqlite.NewStore(database),
			thumbDir: filepath.Join(os.TempDir(), "alexandria-dev-thumbs"),
			close:    database.Close,
		}, nil
	}
	catalog, err := sqlite.Open(catalogPath)
	if err != nil {
		return nil, err
	}
	// Settings live in a "settings" subdir beside catalog.db, created with defaults
	// on open (same as the DB). Uses log.Default() — main set it to the debug logger.
	settingsDir := filepath.Join(catalogPath, "settings")
	settingsService, err := settings.Open(settingsDir, log.Default())
	if err != nil {
		catalog.Close()
		return nil, err
	}
	return &openedCatalog{
		store:       sqlite.NewStore(catalog.DB),
		thumbDir:    filepath.Join(catalogPath, "thumbnails"),
		dbPath:      filepath.Join(catalogPath, sqlite.CatalogDBFile),
		settings:    settingsService,
		settingsDir: settingsDir,
		close: func() error {
			settingsService.Close()
			return catalog.Close()
		},
	}, nil
}

func newIngester(catalog *openedCatalog) *importer.Importer {
	assets := &sqlite.AssetRepo{DB: catalog.store.DB}
	set := catalog.currentSettings()
	thumb := thumbnailer.New(catalog.thumbDir)
	if set.ThumbnailQuality > 0 {
		thumb.Quality = set.ThumbnailQuality // settings owns JPEG quality
	}
	return &importer.Importer{
		Reader:    assets,
		Obs:       assets,
		Derived:   assets,
		Dups:      &sqlite.DuplicateRepo{DB: catalog.store.DB},
		Thumbnail: thumb,
		Store:     catalog.store,
		Imports:   &sqlite.ImportRepo{DB: catalog.store.DB},
		Settings:  set,                      // D18 ignore list + WRITE batch size, owned by settings
		Machine:   catalog.currentMachine(), // worker-pool sizes, owned by settings (machine.json)
		Log:       log.Default(),            // debug-level; configured in main
	}
}

// currentSettings is the settings snapshot the composition root injects into the
// ingester/watcher — the live value when a catalog is open, the built-in defaults
// for :memory: (no settings service) so a throwaway import still skips .DS_Store etc.
func (catalog *openedCatalog) currentSettings() settings.Settings {
	if catalog.settings != nil {
		return catalog.settings.Settings.Get()
	}
	return settings.DefaultSettings()
}

// currentMachine is the machine-config snapshot (worker-pool sizes), same fallback.
func (catalog *openedCatalog) currentMachine() settings.Machine {
	if catalog.settings != nil {
		return catalog.settings.Machine.Get()
	}
	return settings.DefaultMachine()
}

// cpuBoundWorkers is the default pool size for the CPU-bound stages (EXTRACT,
// THUMB): ~75% of the cores, so an import is fast but always leaves headroom for
// the rest of the system (the machine stays responsive, no core fully starved).
// The engine's own default stays the conservative 2 (machine.json owns real
// per-host tuning later); this is a dev-tool convenience. Pass 0 to fall back to
// the engine default, or a specific N to override.
func cpuBoundWorkers() int {
	return max(1, runtime.NumCPU()*3/4)
}

// newLogger builds the harness logger: debug level, with a timestamp and the
// calling site so pipeline log lines are traceable to the stage that emitted
// them. This is a dev tool — verbosity is the point.
func newLogger() *log.Logger {
	logger := log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
		ReportCaller:    true,
		TimeFormat:      time.TimeOnly,
	})
	logger.SetLevel(log.DebugLevel)
	return logger
}

// ensureSource finds the source rooted at absolutePath, creating it if absent.
// The harness works with one source per tree — the same source is reused across
// runs so idempotency and reconcile behave as they would in the real app.
func ensureSource(ctx context.Context, catalog *openedCatalog, absolutePath string) (*domain.Source, error) {
	sources := &sqlite.SourceRepo{DB: catalog.store.DB}
	existing, err := sources.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, source := range existing {
		if source.BasePath == absolutePath {
			return source, nil
		}
	}
	now := time.Now().UTC()
	source := &domain.Source{
		ID:              domain.NewID(),
		Name:            filepath.Base(absolutePath),
		Kind:            domain.SourceKindLocal,
		BasePath:        absolutePath,
		ScanRecursively: true,
		Enabled:         true,
		Connectivity:    domain.SourceOnline,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := sources.Create(ctx, source); err != nil {
		return nil, err
	}
	return source, nil
}

func cmdImport(args []string) error {
	flags := flag.NewFlagSet("import", flag.ExitOnError)
	catalogPath := flags.String("catalog", ":memory:", "catalog dir or :memory:")
	debug := flags.Bool("debug", false, "serve pprof+expvar web UI while importing")
	debugAddr := flags.String("debug-addr", "localhost:6060", "address for the --debug server")
	hashWorkers := flags.Int("hash-workers", 0, "HASH pool size (0 = engine default; I/O-bound)")
	extractWorkers := flags.Int("extract-workers", cpuBoundWorkers(), "EXTRACT pool size (CPU-bound; 0 = conservative engine default)")
	thumbWorkers := flags.Int("thumb-workers", cpuBoundWorkers(), "THUMB pool size (CPU-bound, ~1 image in RAM each; 0 = engine default)")
	path, ok := parsePathAndFlags(flags, args)
	if !ok {
		return fmt.Errorf("usage: dev import <path> [--catalog <dir|:memory:>] [--debug] [--thumb-workers N]")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	catalog, err := openCatalog(*catalogPath)
	if err != nil {
		return err
	}
	defer catalog.close()

	if *debug {
		startDebugServer(*debugAddr)
	}

	ctx := context.Background()
	source, err := ensureSource(ctx, catalog, absolutePath)
	if err != nil {
		return err
	}
	ingester := newIngester(catalog)
	// Worker counts come from machine.json (via newIngester); the dev flags override
	// per stage when >0 (extract/thumb default to cpuBoundWorkers for fast local imports).
	if *hashWorkers > 0 {
		ingester.Machine.Workers.Ingest.Hash = *hashWorkers
	}
	if *extractWorkers > 0 {
		ingester.Machine.Workers.Ingest.Extract = *extractWorkers
	}
	if *thumbWorkers > 0 {
		ingester.Machine.Workers.Ingest.Thumb = *thumbWorkers
	}

	startedAt := time.Now()
	ingester.OnProgress = func(progress importer.Progress) {
		rate := float64(progress.Done) / time.Since(startedAt).Seconds()
		total := "?"
		if progress.TotalKnown {
			total = fmt.Sprintf("%d", progress.Total)
		}
		fmt.Printf("\r  %d/%s committed  %.0f files/s   ", progress.Done, total, rate)
	}

	// Run as a cancelable job so Ctrl-C exercises the mid-run cancel path (the
	// full-processing invariant holds: committed assets are fully processed).
	jobs := importer.NewJobs()
	done := make(chan importer.ImportResult, 1)
	var runErr error
	jobID := jobs.Start("import", func(jobCtx context.Context, id string) {
		result, e := ingester.RunJob(jobCtx, id, source, os.DirFS(absolutePath))
		runErr = e
		done <- result
	})

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	var result importer.ImportResult
	select {
	case result = <-done:
	case <-signals:
		fmt.Fprintln(os.Stderr, "\ncanceling…")
		jobs.Cancel(jobID)
		result = <-done
	}
	fmt.Printf("\r%80s\r", "")

	elapsed := time.Since(startedAt)
	fmt.Printf("session %s (%.1fs, %.0f files/s)\n", result.SessionID, elapsed.Seconds(),
		float64(result.Added+result.Updated+result.Moved+result.Dups)/elapsed.Seconds())
	fmt.Printf("  added=%d updated=%d moved=%d skipped=%d dups=%d missing=%d errors=%d\n",
		result.Added, result.Updated, result.Moved, result.Skipped, result.Dups, result.Missing, len(result.Errors))
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "  (run ended:", runErr, ")")
	}
	if catalog.dbPath != "" {
		fmt.Printf("  catalog db: %s  (open in a SQLite viewer to browse)\n", catalog.dbPath)
		fmt.Printf("  settings:   %s  (settings.json, machine.json, keybindings.json — created with defaults)\n", catalog.settingsDir)
	} else {
		fmt.Println("  catalog db: in-memory (nothing to browse — use --catalog <dir> for a real file)")
	}

	// Hold open so the debug UI is reachable even after a fast run (a sub-second
	// import would otherwise exit before you could open the browser).
	if *debug {
		fmt.Printf("  debug UI: http://%s/  — Ctrl-C to exit\n", *debugAddr)
		hold := make(chan os.Signal, 1)
		signal.Notify(hold, os.Interrupt, syscall.SIGTERM)
		<-hold
	}
	return nil
}

// startDebugServer serves Go's stdlib pprof + expvar on addr in the background.
// net/http/pprof and expvar register their handlers on http.DefaultServeMux via
// their package init (blank imports above), so a nil handler serves them; we add
// a small index at "/" that links to both.
func startDebugServer(addr string) {
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/" {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprint(writer, debugIndexHTML)
	})
	go func() {
		if err := http.ListenAndServe(addr, nil); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "debug server:", err)
		}
	}()
	fmt.Printf("debug UI serving at http://%s/\n", addr)
}

const debugIndexHTML = `<!doctype html><meta charset=utf-8><title>Alexandria dev</title>
<h1>Alexandria engine — debug</h1>
<ul>
  <li><a href="/debug/pprof/">/debug/pprof/</a> — profiles index</li>
  <li><a href="/debug/pprof/goroutine?debug=1">goroutine dump</a> — the live pipeline picture (who's blocked on which channel)</li>
  <li><a href="/debug/pprof/heap?debug=1">heap</a> · <a href="/debug/pprof/profile?seconds=5">30s CPU profile</a></li>
  <li><a href="/debug/vars">/debug/vars</a> — expvar: runtime, GC, memstats</li>
</ul>`

func cmdReconcile(args []string) error {
	flags := flag.NewFlagSet("reconcile", flag.ExitOnError)
	catalogPath := flags.String("catalog", "", "catalog dir")
	path, ok := parsePathAndFlags(flags, args)
	if !ok {
		return fmt.Errorf("usage: dev reconcile <path> --catalog <dir>")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	catalog, err := openCatalog(*catalogPath)
	if err != nil {
		return err
	}
	defer catalog.close()

	ctx := context.Background()
	source, err := ensureSource(ctx, catalog, absolutePath)
	if err != nil {
		return err
	}
	// "Reconcile is a schedule, not a component" (D14): a reconcile is just the
	// pipeline in full-walk mode. The walk-end diff marks vanished files missing and
	// the matrix relinks reappeared ones — the standalone Reconcile retired in 05.3.
	result, err := newIngester(catalog).Run(ctx, source, os.DirFS(absolutePath))
	if err != nil {
		return err
	}
	fmt.Printf("reconcile: added=%d updated=%d moved=%d missing=%d errors=%d\n",
		result.Added, result.Updated, result.Moved, result.Missing, len(result.Errors))
	return nil
}

func cmdWatch(args []string) error {
	flags := flag.NewFlagSet("watch", flag.ExitOnError)
	catalogPath := flags.String("catalog", "", "catalog dir")
	path, ok := parsePathAndFlags(flags, args)
	if !ok {
		return fmt.Errorf("usage: dev watch <path> --catalog <dir>")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	catalog, err := openCatalog(*catalogPath)
	if err != nil {
		return err
	}
	defer catalog.close()

	// Cancel on Ctrl-C: the watcher returns context.Canceled on a clean shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	source, err := ensureSource(ctx, catalog, absolutePath)
	if err != nil {
		return err
	}
	w := &watcher.Watcher{
		Ingester: newIngester(catalog),
		Obs:      &sqlite.AssetRepo{DB: catalog.store.DB}, // the one sanctioned write: connectivity
		Source:   source,
		Root:     absolutePath,
		Settings: catalog.currentSettings(), // D18 intake filter is Settings.Ignored
		Log:      log.Default(),
	}
	fmt.Fprintf(os.Stderr, "watching %s — Ctrl-C to stop\n", absolutePath)
	if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func cmdErrors(args []string) error {
	flags := flag.NewFlagSet("errors", flag.ExitOnError)
	catalogPath := flags.String("catalog", "", "catalog dir")
	flags.Parse(args)
	catalog, err := openCatalog(*catalogPath)
	if err != nil {
		return err
	}
	defer catalog.close()

	imports := &sqlite.ImportRepo{DB: catalog.store.DB}
	sessions, err := imports.ListSessions(context.Background(), 1)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Println("no sessions")
		return nil
	}
	importErrors, err := imports.ListErrors(context.Background(), sessions[0].ID)
	if err != nil {
		return err
	}
	fmt.Printf("session %s: %d error(s)\n", sessions[0].ID, len(importErrors))
	for _, importError := range importErrors {
		fmt.Printf("  [%s/%s x%d] %s: %s\n", importError.Stage, importError.ReasonCode, importError.Attempts, importError.Path, importError.Message)
	}
	return nil
}

func cmdSessions(args []string) error {
	flags := flag.NewFlagSet("sessions", flag.ExitOnError)
	catalogPath := flags.String("catalog", "", "catalog dir")
	flags.Parse(args)
	catalog, err := openCatalog(*catalogPath)
	if err != nil {
		return err
	}
	defer catalog.close()

	sessions, err := (&sqlite.ImportRepo{DB: catalog.store.DB}).ListSessions(context.Background(), 20)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		fmt.Printf("%s  %-9s  added=%d updated=%d moved=%d skipped=%d dups=%d errors=%d\n",
			session.StartedAt.Format(time.RFC3339), session.Kind, session.Added, session.Updated, session.Moved, session.Skipped, session.Dups, session.Errors)
		if len(session.SkippedUnknown) > 0 {
			fmt.Printf("    unknown: %v\n", session.SkippedUnknown)
		}
		if len(session.SkippedIgnored) > 0 {
			fmt.Printf("    ignored: %v\n", session.SkippedIgnored)
		}
	}
	return nil
}

func cmdRebuild(args []string) error {
	if len(args) < 1 || args[0] != "fts" {
		return fmt.Errorf("usage: dev rebuild fts --catalog <dir>")
	}
	flags := flag.NewFlagSet("rebuild", flag.ExitOnError)
	catalogPath := flags.String("catalog", "", "catalog dir")
	flags.Parse(args[1:])
	catalog, err := openCatalog(*catalogPath)
	if err != nil {
		return err
	}
	defer catalog.close()

	if err := sqlite.RebuildFTS(context.Background(), catalog.store.DB); err != nil {
		return err
	}
	fmt.Println("fts rebuilt")
	return nil
}
