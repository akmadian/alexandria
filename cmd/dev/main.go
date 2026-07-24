// Command dev is Alexandria's engine harness: it drives the pipeline end-to-end
// and exposes its state, with full access to internal/*. It is the manual rig
// for the impl acceptance criteria (impl/08), not a shipped entrypoint.
//
// ponytail: stdlib flag, subcommands only. The --debug HTTP server serves Go's
// pprof/expvar plus the enrichment observability surface (task 22, in
// observability.go): a live asset/kind/artifact/queue page over the engine
// snapshot, and its JSON feed. The remaining impl/08 wants (statsviz, a general
// pipeline /state dashboard, --json) stay deferred — DEFERRED §9.
package main

import (
	"context"
	"database/sql"
	"errors"
	_ "expvar" // registers /debug/vars on http.DefaultServeMux
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // dev-only profiling endpoint
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/akmadian/alexandria/internal/assettype"
	"github.com/akmadian/alexandria/internal/dependency"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/enrichment"
	"github.com/akmadian/alexandria/internal/importer"
	"github.com/akmadian/alexandria/internal/logging"
	"github.com/akmadian/alexandria/internal/migrations"
	"github.com/akmadian/alexandria/internal/settings"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/thumbnailer"
	"github.com/akmadian/alexandria/internal/volume"
	"github.com/akmadian/alexandria/internal/watcher"
	"github.com/akmadian/gospan"
	gospansqlite "github.com/akmadian/gospan/sqlite"
	"github.com/charmbracelet/log"
	_ "modernc.org/sqlite"
)

func main() {
	// The harness exists to see what the engine is doing, so debug logging is on
	// by default (no flag). Setting it as the package default too means leaf
	// packages that log via the global logger inherit the level.
	log.SetDefault(logging.New(os.Stderr))

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
	case "jobs":
		err = cmdJobs(args)
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
                      [--hash-workers N] [--extract-workers N] [--thumbnail-workers N]
                      [--trace=false]
  dev reconcile <path> [--catalog <dir>]           mark missing/restored files
  dev watch     <path> [--catalog <dir>]           live-watch a tree until Ctrl-C
  dev errors          [--catalog <dir>]            dump the latest session's DLQ
  dev sessions        [--catalog <dir>]            list recent import sessions
  dev rebuild fts     [--catalog <dir>]            rebuild the FTS index
  dev rebuild pathkeys [--catalog <dir>]           rebuild the path_key (NFC) column
  dev jobs graph      [--format dot|ascii]         render the enrichment job graph

worker pools (import):
  EXTRACT and the thumbnail enrichment pool (CPU-bound) default to ~75% of your
  cores — fast, but leaves headroom so the machine stays responsive (it can't
  lock up: the OS preempts, Go caps parallelism at NumCPU, and the enrichment
  CPU budget arbitrates the sum regardless). Thumbnailing runs POST-commit on
  the enrichment engine (D25): the import finishes at ingest speed, then the
  harness waits for enrichment to converge. Override with --thumbnail-workers/
  --extract-workers, or pass 0 for the conservative defaults (hash=4 extract=2
  thumbnail=2). Each in-flight thumbnail decode holds a full image in RAM;
  watch it under --debug (/debug/pprof/heap).

catalog storage (--catalog):
  :memory:   (default)  throwaway in-memory DB — fast, but nothing to browse afterward.
  <dir>                 a real catalog directory. The SQLite file is <dir>/catalog.db —
                        open that path in any DB viewer (sqlite3, Datasette, DB Browser)
                        to inspect assets, sidecars, sessions, and the DLQ. The import
                        prints the exact path on completion.

debug web UI (--debug, import only):
  Serves Go's pprof + expvar AND the live enrichment page at http://localhost:6060/
  during (and, so you can browse a fast run, after) the import. Override the address
  with --debug-addr, e.g.
    dev import ~/Photos --catalog ./cat --debug
    dev import ~/Photos --catalog ./cat --debug --debug-addr :6000
  /enrichment is the live asset/kind/artifact/queue view (depths, budget, in-flight,
  DLQ, matrix; /enrichment/snapshot.json is the raw feed) with Pause/Resume controls.
  /debug/pprof/goroutine?debug=1 is the pipeline picture; /debug/vars is runtime + GC
  stats. Ctrl-C exits when done.

  --pause-enrichment starts the engine paused (implies --debug): the import commits and
  the backlog fills the page, but nothing enriches until you click Resume — the
  inspect-then-approve gate.

tracing (import; on by default, --trace=false to disable):
  Every run writes a gospan trace file to <catalog>/traces/ — per-stage spans,
  queue gaps, batch commits — and prints its path at the end. Analyze it with the
  script library in cmd/dev/sql/:
    sqlite3 -box <catalog>/traces/gospan-<run>.sqlite < cmd/dev/sql/trace-report.sql
    sqlite3 -box <catalog>/traces/gospan-<run>.sqlite < cmd/dev/sql/trace-asset.sql
  Catalog scripts live there too (catalog-stats.sql, catalog-wipe.sql for a fresh slate).
`)
}

// parsePathAndFlags handles "<subcmd> <path> [--flags]": Go's flag package stops
// at the first positional, so we pull the path out and parse the flags that
// follow it. Returns ok=false if no positional path was given.
func parsePathAndFlags(flags *flag.FlagSet, args []string) (string, bool) {
	if err := flags.Parse(args); err != nil {
		return "", false
	}
	if flags.NArg() < 1 {
		return "", false
	}
	path := flags.Arg(0)
	if err := flags.Parse(flags.Args()[1:]); err != nil {
		return "", false
	}
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
			_ = database.Close()
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
		_ = catalog.Close()
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
	return &importer.Importer{
		Reader:   assets,
		Obs:      assets,
		Dups:     &sqlite.DuplicateRepo{DB: catalog.store.DB},
		Store:    catalog.store,
		Imports:  &sqlite.ImportRepo{DB: catalog.store.DB},
		Settings: catalog.currentSettings(), // D18 ignore list + WRITE batch size, owned by settings
		Machine:  catalog.currentMachine(),  // worker-pool sizes, owned by settings (machine.json)
		Log:      log.Default(),             // debug-level; configured in main
	}
}

// enrichmentRig is a running enrichment engine plus the runtime dependencies
// the harness composed for it (the first real composition root for the task-18
// engine). Stop shuts the engine down and closes the exiftool daemon.
type enrichmentRig struct {
	engine      *enrichment.Engine
	definitions []enrichment.JobDefinition
	repo        *sqlite.EnrichmentRepo
	daemon      *dependency.ExiftoolDaemon // nil when exiftool is undiscovered
}

// startEnrichment builds and starts the enrichment engine against an open
// catalog: the thumbnailer (with the exiftool daemon when discoverable — RAW
// previews degrade to tool_unavailable DLQ rows without it), the canonical
// job-definition registry, and the engine itself. thumbnailWorkers > 0
// overrides the thumbnail pool size (the harness's fast-local-import knob).
func startEnrichment(ctx context.Context, catalog *openedCatalog, resolver *volume.Resolver, tracer *gospan.Tracer, thumbnailWorkers int) (*enrichmentRig, error) {
	settingsSnapshot := catalog.currentSettings()
	thumbnails := thumbnailer.New(catalog.thumbDir)
	if settingsSnapshot.ThumbnailQuality > 0 {
		thumbnails.Quality = settingsSnapshot.ThumbnailQuality // settings owns JPEG quality
	}
	machine := catalog.currentMachine()
	var daemon *dependency.ExiftoolDaemon
	status := dependency.Discover(dependency.Exiftool, machine.DependencyPaths[string(dependency.Exiftool)])
	if status.Available() {
		started, err := dependency.StartExiftool(status, log.Default())
		if err != nil {
			return nil, err
		}
		daemon = started
		thumbnails.Exiftool = daemon
	} else {
		log.Default().Warn("exiftool not found — RAW thumbnails will fail as tool_unavailable", "state", status.State)
	}

	if thumbnailWorkers > 0 {
		if machine.Workers.Enrichment == nil {
			machine.Workers.Enrichment = map[string]int{}
		}
		machine.Workers.Enrichment["thumbnail"] = thumbnailWorkers
	}
	definitions := enrichment.Definitions(thumbnails, resolver)
	enrichmentRepo := &sqlite.EnrichmentRepo{DB: catalog.store.DB}
	engine, err := enrichment.New(&enrichment.Config{
		Definitions: definitions,
		Reader:      &sqlite.AssetRepo{DB: catalog.store.DB},
		Store:       catalog.store,
		Enrichment:  enrichmentRepo,
		Log:         log.Default(),
		Tracer:      tracer,
		Machine:     machine,
	})
	if err != nil {
		if daemon != nil {
			_ = daemon.Close()
		}
		return nil, err
	}
	engine.Start(ctx)
	return &enrichmentRig{engine: engine, definitions: definitions, repo: enrichmentRepo, daemon: daemon}, nil
}

func (rig *enrichmentRig) stop() {
	rig.engine.Stop()
	if rig.daemon != nil {
		_ = rig.daemon.Close()
	}
}

// missingArtifacts counts the catalog's remaining enrichment work: assets
// eligible for some definition whose artifact is still missing and not
// attempt-exhausted — the same scan the dispatcher runs, reused as the
// harness's convergence probe (0 = every artifact machine reached present or
// failed).
func (rig *enrichmentRig) missingArtifacts(ctx context.Context) (int, error) {
	columnByKind := make(map[string]string, len(rig.definitions))
	for index := range rig.definitions {
		columnByKind[rig.definitions[index].Kind] = rig.definitions[index].ArtifactColumn
	}
	total := 0
	for index := range rig.definitions {
		definition := &rig.definitions[index]
		var extensions []string
		for _, handler := range assettype.All() {
			if definition.Applicable(handler) {
				extensions = append(extensions, handler.Ext)
			}
		}
		prerequisiteColumns := make([]string, 0, len(definition.Prerequisites))
		for _, prerequisite := range definition.Prerequisites {
			prerequisiteColumns = append(prerequisiteColumns, columnByKind[prerequisite])
		}
		missing, err := rig.repo.ListMissingArtifacts(ctx, &sqlite.MissingArtifactScan{
			Kind:                definition.Kind,
			ArtifactColumn:      definition.ArtifactColumn,
			PrerequisiteColumns: prerequisiteColumns,
			Extensions:          extensions,
			MaxAttempts:         enrichment.MaxAttempts,
			Limit:               10000, // a count cap, not a page — dev-harness precision is fine
		})
		if err != nil {
			return 0, err
		}
		total += len(missing)
	}
	return total, nil
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

// newResolver builds the path→volume resolver over the catalog's volume table
// and the OS prober. One resolver per run is shared between folder-add and
// enrichment so its in-session mount cache is warm for both (D24).
func newResolver(catalog *openedCatalog) *volume.Resolver {
	return volume.NewResolver(&sqlite.VolumeRepo{DB: catalog.store.DB}, volume.NewSystemProber(), log.Default())
}

// resolveTarget ensures a tracked folder at absolutePath (find-or-create volume,
// quiet folder-add) and returns the importer target plus the volume's current
// mount point (the DirFS root). The same tree resolves to the same volume across
// runs, so idempotency and reconcile behave as they would in the real app.
func resolveTarget(ctx context.Context, catalog *openedCatalog, resolver *volume.Resolver, absolutePath string) (importer.Target, string, error) {
	manager := volume.NewManager(resolver, &sqlite.FolderRepo{DB: catalog.store.DB}, log.Default())
	// confirm=true: the harness always proceeds (absorbs quietly); the four-outcome
	// union is exercised by the seam/tests, not this convenience path.
	if _, err := manager.CreateFolder(ctx, absolutePath, domain.SyncModeManual, true); err != nil {
		return importer.Target{}, "", err
	}
	resolved, err := resolver.Resolve(ctx, absolutePath)
	if err != nil {
		return importer.Target{}, "", err
	}
	target := importer.Target{
		VolumeID: resolved.VolumeID,
		WalkRoot: resolved.RelativePath,
		Name:     filepath.Base(absolutePath),
	}
	return target, resolved.MountPoint, nil
}

func cmdImport(args []string) error {
	flags := flag.NewFlagSet("import", flag.ExitOnError)
	catalogPath := flags.String("catalog", ":memory:", "catalog dir or :memory:")
	debug := flags.Bool("debug", false, "serve pprof+expvar web UI while importing")
	debugAddr := flags.String("debug-addr", "localhost:6060", "address for the --debug server")
	trace := flags.Bool("trace", true, "write a gospan trace file for this run (--trace=false for A/B overhead runs)")
	hashWorkers := flags.Int("hash-workers", 0, "HASH pool size (0 = engine default; I/O-bound)")
	extractWorkers := flags.Int("extract-workers", cpuBoundWorkers(), "EXTRACT pool size (CPU-bound; 0 = conservative engine default)")
	thumbnailWorkers := flags.Int("thumbnail-workers", cpuBoundWorkers(), "thumbnail enrichment pool size (CPU-bound, ~1 image in RAM each; 0 = registry default)")
	pauseEnrichment := flags.Bool("pause-enrichment", false, "start enrichment paused — inspect the /enrichment page, then click Resume (implies --debug)")
	path, ok := parsePathAndFlags(flags, args)
	if !ok {
		return fmt.Errorf("usage: dev import <path> [--catalog <dir|:memory:>] [--debug] [--thumbnail-workers N]")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if *pauseEnrichment {
		*debug = true // the Resume control lives on the debug page
	}

	catalog, err := openCatalog(*catalogPath)
	if err != nil {
		return err
	}
	defer func() { _ = catalog.close() }()

	ctx := context.Background()
	resolver := newResolver(catalog)
	target, mountPoint, err := resolveTarget(ctx, catalog, resolver, absolutePath)
	if err != nil {
		return err
	}

	// Tracing is on by default in the harness (~360ns/span): one SQLite trace
	// file per run, browsable/queryable like the catalog itself. The complaints
	// channel is the debug logger; span rows never enter the log flow. A nil
	// tracer (--trace=false) turns the whole instrument off.
	var tracer *gospan.Tracer
	var traceSink *gospansqlite.Sink
	if *trace {
		traceDir := filepath.Join(os.TempDir(), "alexandria-dev-traces")
		if *catalogPath != ":memory:" {
			traceDir = filepath.Join(*catalogPath, "traces")
		}
		traceSink, err = gospansqlite.New(traceDir)
		if err != nil {
			return err
		}
		tracer, err = gospan.New(
			traceSink,
			// gospan.MultiSink(
			// 	gospan.SlogSink(slog.Default()),
			// 	traceSink,
			// ),
			gospan.WithLogger(slog.New(log.Default())),
			gospan.WithOverheadSampling(4))
		if err != nil {
			return err
		}
		defer tracer.Close(context.Background()) // safety net for early returns; Close is idempotent — the reporting Close runs below
	}

	// The enrichment engine runs alongside the import (the real model: the CPU
	// budget arbitrates between them); ingest's post-commit hook nudges its
	// dispatcher, and the on-open scan has already queued any backlog from
	// previous runs.
	rig, err := startEnrichment(ctx, catalog, resolver, tracer, *thumbnailWorkers)
	if err != nil {
		return err
	}

	// --pause-enrichment holds the engine before any dispatch: the import still
	// commits assets and the scan still enqueues them (scanning is not
	// dispatching), so the backlog accumulates fully visible on the page, and
	// nothing runs until the user clicks Resume. This is the inspect-then-approve
	// gate — the existing PauseAll (task 21), driven from the debug page.
	if *pauseEnrichment {
		rig.engine.PauseAll()
	}

	// The debug surface mounts now the engine exists: the enrichment page + JSON
	// feed on the same server as pprof/expvar. Started before the import runs so
	// depths can be watched draining from the first commit.
	if *debug {
		mountEnrichmentDebug(rig, catalog)
		startDebugServer(*debugAddr)
	}
	ingester := newIngester(catalog)
	ingester.Tracer = tracer
	ingester.OnAssetCommitted = func(context.Context, string, string, string) {
		rig.engine.RequestScan() // a hint, coalesced; the scan stays the authority
	}
	// Worker counts come from machine.json (via newIngester); the dev flags override
	// per stage when >0 (extract defaults to cpuBoundWorkers for fast local imports).
	if *hashWorkers > 0 {
		ingester.Machine.Workers.Ingest.Hash = *hashWorkers
	}
	if *extractWorkers > 0 {
		ingester.Machine.Workers.Ingest.Extract = *extractWorkers
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
		result, e := ingester.RunJob(jobCtx, id, target, os.DirFS(mountPoint))
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

	// Let enrichment converge before exiting — a one-shot harness process takes
	// its queues with it, so waiting here is what makes `dev import` end with a
	// fully-thumbnailed catalog (a canceled import skips the wait; the next run's
	// on-open scan converges the committed remainder, which is the D25 model).
	// Paused (the inspect gate) has nothing to await — the user drives it from the
	// page; converging here would hang forever on a backlog that can't dispatch.
	switch {
	case *pauseEnrichment:
		fmt.Printf("  enrichment: PAUSED — open http://%s/enrichment to inspect, then click Resume\n", *debugAddr)
	case runErr == nil:
		if err := awaitConvergence(ctx, rig, signals); err != nil {
			fmt.Fprintln(os.Stderr, "  (enrichment convergence:", err, ")")
		}
	}
	// Under --debug keep the engine alive through the hold below, so the live page
	// shows a running (converged, idle) engine rather than an empty stopped one;
	// otherwise stop now. printEnrichmentFailures reads the DB either way.
	if !*debug {
		rig.stop()
	}
	printEnrichmentFailures(ctx, catalog)

	// Close drains the buffer and finishes the trace file; stats read after so
	// Written is final. Dropped > 0 means the event buffer needs WithBufferSize.
	if tracer != nil {
		if err := tracer.Close(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, "  (trace close:", err, ")")
		}
		traceStats := tracer.Stats()
		// Started/Completed count spans; Written/Dropped count queue EVENTS
		// (start + end + attr updates — roughly 2–3 per span). overhead/span is
		// exact, not sampled: WithOverheadSampling(1) times every span. It is a
		// process-lifetime rolling average and lives here, not in the trace file.
		fmt.Printf("  trace db:   %s\n", traceSink.Path())
		fmt.Printf("  tracing:    spans=%d events=%d dropped=%d writeErrors=%d overhead/span=%s (exact)\n",
			traceStats.Completed, traceStats.Written, traceStats.Dropped, traceStats.WriteErrors, traceStats.OverheadPerSpan)
		fmt.Printf("              analyze: sqlite3 -box <trace db> < cmd/dev/sql/trace-report.sql\n")
	}

	// Hold open so the debug UI is reachable even after a fast run (a sub-second
	// import would otherwise exit before you could open the browser).
	if *debug {
		fmt.Printf("  debug UI: http://%s/  — Ctrl-C to exit\n", *debugAddr)
		hold := make(chan os.Signal, 1)
		signal.Notify(hold, os.Interrupt, syscall.SIGTERM)
		<-hold
		rig.stop() // deferred from above so the page stayed live during the hold
	}
	return nil
}

// awaitConvergence polls the missing-artifact count (requesting a scan pass
// each tick so non-exhausted failures keep retrying) until every artifact
// machine reaches present or failed, or the user interrupts.
func awaitConvergence(ctx context.Context, rig *enrichmentRig, interrupt <-chan os.Signal) error {
	remaining, err := rig.missingArtifacts(ctx)
	if err != nil || remaining == 0 {
		return err
	}
	fmt.Printf("  enrichment: %d artifact(s) to converge…\n", remaining)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-interrupt:
			fmt.Printf("\r%60s\rinterrupted — %d artifact(s) left for the next run's scan\n", "", remaining)
			return nil
		case <-ticker.C:
			rig.engine.RequestScan()
			remaining, err = rig.missingArtifacts(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("\r  enrichment: %d remaining   ", remaining)
			if remaining == 0 {
				fmt.Printf("\r%60s\r  enrichment: converged\n", "")
				return nil
			}
		}
	}
}

// printEnrichmentFailures summarizes the enrichment DLQ after a run — absence
// is ambiguous, so the harness says out loud which assets ended failed.
func printEnrichmentFailures(ctx context.Context, catalog *openedCatalog) {
	var failures, exhausted int
	row := catalog.store.DB.QueryRowContext(ctx,
		"SELECT COUNT(*), COALESCE(SUM(attempts >= ?), 0) FROM enrichment_errors", enrichment.MaxAttempts)
	if err := row.Scan(&failures, &exhausted); err != nil {
		fmt.Fprintln(os.Stderr, "  (enrichment DLQ read:", err, ")")
		return
	}
	if failures > 0 {
		fmt.Printf("  enrichment DLQ: %d asset/kind failure(s), %d exhausted (see enrichment_errors)\n", failures, exhausted)
	}
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
	server := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "debug server:", err)
		}
	}()
	fmt.Printf("debug UI serving at http://%s/\n", addr)
}

const debugIndexHTML = `<!doctype html><meta charset=utf-8><title>Alexandria dev</title>
<h1>Alexandria engine — debug</h1>
<ul>
  <li><a href="/enrichment">/enrichment</a> — live enrichment: queues, budget, in-flight, DLQ, asset×kind matrix (<a href="/enrichment/snapshot.json">snapshot.json</a>)</li>
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
	defer func() { _ = catalog.close() }()

	ctx := context.Background()
	resolver := newResolver(catalog)
	target, mountPoint, err := resolveTarget(ctx, catalog, resolver, absolutePath)
	if err != nil {
		return err
	}
	// "Reconcile is a schedule, not a component" (D14): a reconcile is just the
	// pipeline in full-walk mode. The walk-end diff marks vanished files missing and
	// the matrix relinks reappeared ones — the standalone Reconcile retired in 05.3.
	result, err := newIngester(catalog).Run(ctx, target, os.DirFS(mountPoint))
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
	defer func() { _ = catalog.close() }()

	// Cancel on Ctrl-C: the watcher returns context.Canceled on a clean shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	resolver := newResolver(catalog)
	target, mountPoint, err := resolveTarget(ctx, catalog, resolver, absolutePath)
	if err != nil {
		return err
	}
	// The engine converges watcher-ingested files live: every committed asset
	// nudges the dispatcher, and the on-open scan already covered the backlog.
	rig, err := startEnrichment(ctx, catalog, resolver, nil, 0)
	if err != nil {
		return err
	}
	defer rig.stop()
	ingester := newIngester(catalog)
	ingester.OnAssetCommitted = func(context.Context, string, string, string) {
		rig.engine.RequestScan()
	}
	fileWatcher := &watcher.Watcher{
		Ingester:   ingester,
		Obs:        &sqlite.AssetRepo{DB: catalog.store.DB}, // the one sanctioned write: connectivity
		Target:     target,
		MountPoint: mountPoint,
		Settings:   catalog.currentSettings(), // D18 intake filter is Settings.Ignored
		Log:        log.Default(),
	}
	fmt.Fprintf(os.Stderr, "watching %s — Ctrl-C to stop\n", absolutePath)
	if err := fileWatcher.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func cmdErrors(args []string) error {
	flags := flag.NewFlagSet("errors", flag.ExitOnError)
	catalogPath := flags.String("catalog", "", "catalog dir")
	if err := flags.Parse(args); err != nil {
		return err
	}
	catalog, err := openCatalog(*catalogPath)
	if err != nil {
		return err
	}
	defer func() { _ = catalog.close() }()

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
	if err := flags.Parse(args); err != nil {
		return err
	}
	catalog, err := openCatalog(*catalogPath)
	if err != nil {
		return err
	}
	defer func() { _ = catalog.close() }()

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
	if len(args) < 1 || (args[0] != "fts" && args[0] != "pathkeys") {
		return fmt.Errorf("usage: dev rebuild <fts|pathkeys> --catalog <dir>")
	}
	target := args[0]
	flags := flag.NewFlagSet("rebuild", flag.ExitOnError)
	catalogPath := flags.String("catalog", "", "catalog dir")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	catalog, err := openCatalog(*catalogPath)
	if err != nil {
		return err
	}
	defer func() { _ = catalog.close() }()

	switch target {
	case "fts":
		if err := sqlite.RebuildFTS(context.Background(), catalog.store.DB); err != nil {
			return err
		}
		fmt.Println("fts rebuilt")
	case "pathkeys":
		if err := sqlite.RebuildPathKeys(context.Background(), catalog.store.DB); err != nil {
			return err
		}
		fmt.Println("path_key rebuilt")
	}
	return nil
}
