package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/importer"
	"github.com/akmadian/alexandria/internal/metadata"
	"github.com/akmadian/alexandria/internal/migrations"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/thumbnailer"
	"github.com/charmbracelet/log"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func main() {
	logger := createLogger()
	ctx := context.Background()

	db, err := openCatalog(logger)
	if err != nil {
		logger.Fatal("catalog init", "err", err)
	}
	defer db.Close()

	// TESTS -- BELOW THIS LINE IS ONLY FOR SMOKE TESTING THE IMPORTER AND THUMBNAILER. NOT A REAL APP.
	sources := &sqlite.SourceRepo{DB: db}
	thumbDir := filepath.Join(os.TempDir(), "alexandria-thumbs")
	os.RemoveAll(thumbDir)
	imp := &importer.Importer{
		Assets:    &sqlite.AssetRepo{DB: db},
		Dups:      &sqlite.DuplicateRepo{DB: db},
		Metadata:  metadata.Default(),
		Thumbnail: thumbnailer.New(thumbDir),
		Log:       logger,
	}
	logger.Debug("thumbnails", "dir", thumbDir)

	now := time.Now().UTC()
	src := &domain.Source{
		ID:              uuid.NewString(),
		Name:            "testdata",
		Kind:            domain.SourceKindLocal,
		BasePath:        "../testdata",
		ScanRecursively: true,
		Status:          domain.SourceStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := sources.Create(ctx, src); err != nil {
		logger.Fatal("register source", "err", err)
	}
	logger.Debug("source registered", "id", src.ID, "kind", src.Kind, "path", src.BasePath)

	res, err := imp.Run(ctx, src, os.DirFS(src.BasePath))
	if err != nil {
		logger.Fatal("import failed", "err", err)
	}
	logger.Info("import pass complete", "added", res.Added, "skipped", res.Skipped, "errors", len(res.Errors))
}

// openCatalog opens the in-memory smoke catalog and migrates it to the latest
// schema. One connection so the :memory: DB isn't recreated per connection
// (see testutil).
func openCatalog(logger *log.Logger) (*sql.DB, error) {
	logger.Debug("opening catalog", "dsn", ":memory:")
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open catalog: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := migrations.Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate catalog: %w", err)
	}
	schema, _ := migrations.LatestVersion()
	logger.Info("catalog ready", "schema", schema)
	return db, nil
}

func createLogger() *log.Logger {
	logger := log.NewWithOptions(os.Stderr, log.Options{
		ReportCaller:    true,
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
	})

	// Deep per-file tracing lives at debug; opt in without recompiling.
	const manualDebugOverride = true
	if os.Getenv("ALEXANDRIA_DEBUG") != "" || manualDebugOverride {
		logger.SetLevel(log.DebugLevel)
	}

	// Also the package default, so leaf packages (metadata, thumbnailer) that log
	// via the global logger inherit this config and level.
	log.SetDefault(logger)
	return logger
}
