package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed *.sql
var migrationFiles embed.FS

type migration struct {
	Version int
	Name    string
	SQL     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, ".")
	if err != nil {
		return nil, fmt.Errorf("reading migration files: %w", err)
	}

	var migrations []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(e.Name(), "_", 2)
		if len(parts) < 2 {
			continue
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		data, err := migrationFiles.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}
		migrations = append(migrations, migration{
			Version: version,
			Name:    e.Name(),
			SQL:     string(data),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}

func ensureTrackingTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`)
	return err
}

func appliedVersions(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// Migrate runs all pending migrations in order. Each migration runs in its own transaction.
func Migrate(db *sql.DB) error {
	if err := ensureTrackingTable(db); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	all, err := loadMigrations()
	if err != nil {
		return err
	}

	applied, err := appliedVersions(db)
	if err != nil {
		return fmt.Errorf("reading applied versions: %w", err)
	}

	for _, m := range all {
		if applied[m.Version] {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", m.Name, err)
		}

		if _, err := tx.Exec(m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s failed: %w", m.Name, err)
		}

		if _, err := tx.Exec(
			"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
			m.Version, m.Name, time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %s: %w", m.Name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", m.Name, err)
		}
	}

	return nil
}

// LatestVersion returns the highest migration version embedded in the binary.
func LatestVersion() (int, error) {
	all, err := loadMigrations()
	if err != nil {
		return 0, err
	}
	if len(all) == 0 {
		return 0, nil
	}
	return all[len(all)-1].Version, nil
}
