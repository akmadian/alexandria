package main

import (
	"context"
	"database/sql"
	"os"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/importer"
	"github.com/akmadian/alexandria/internal/migrations"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/charmbracelet/log"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func main() {
	log.SetDefault(log.NewWithOptions(os.Stderr, log.Options{
		ReportCaller:    true,
		ReportTimestamp: true,
		TimeFormat:      time.Kitchen,
	}))

	ctx := context.Background()

	// In-memory catalog for this smoke run. One connection so the :memory: DB
	// isn't recreated per connection (see testutil).
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		log.Fatal("open catalog", "err", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := migrations.Migrate(db); err != nil {
		log.Fatal("migrate catalog", "err", err)
	}
	log.Info("DB Connection successful")

	sources := &sqlite.SourceRepo{DB: db}
	imp := &importer.Importer{
		Assets: &sqlite.AssetRepo{DB: db},
		Dups:   &sqlite.DuplicateRepo{DB: db},
		Log:    log.Default(),
	}

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
		log.Fatal("register source", "err", err)
	}

	// Run twice to show idempotency: the second pass skips everything.
	for pass := 1; pass <= 2; pass++ {
		res, err := imp.Run(ctx, src, os.DirFS(src.BasePath))
		if err != nil {
			log.Fatal("import failed", "pass", pass, "err", err)
		}
		log.Info("import pass complete", "pass", pass,
			"added", res.Added, "skipped", res.Skipped, "errors", len(res.Errors))
	}
}
