package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/akmadian/alexandria/internal/migrations"
	_ "modernc.org/sqlite"
)

// CatalogDBFile is the SQLite filename inside a catalog directory. Exported so
// tooling (the dev harness) can point a DB viewer at the exact file.
const CatalogDBFile = "catalog.db"

const catalogLockFile = "catalog.lock"

// Catalog is an open on-disk catalog: the migrated SQLite handle plus the
// single-instance lock. Close releases both.
type Catalog struct {
	DB   *sql.DB
	lock *instanceLock
}

// Open opens (creating if needed) the catalog in dir. It acquires a single-
// instance advisory lock, opens SQLite in WAL mode with the crash-safety
// pragmas, and migrates to the latest schema. The two hard failures — cannot
// open, cannot migrate — return an error and hold no resources. The directory
// is created 0700 per the security requirement.
//
// The per-connection pragmas ride in the DSN so every pooled connection gets
// them; journal_mode(WAL) is persisted in the file header on first apply.
func Open(dir string) (*Catalog, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("catalog dir: %w", err)
	}
	lock, err := acquireLock(filepath.Join(dir, catalogLockFile))
	if err != nil {
		return nil, err
	}
	dsn := filepath.Join(dir, CatalogDBFile) +
		"?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err == nil {
		err = db.Ping() // sql.Open is lazy; surface open failures now
	}
	if err != nil {
		if db != nil {
			_ = db.Close()
		}
		_ = lock.release()
		return nil, fmt.Errorf("open catalog: %w", err)
	}
	if err := migrations.Migrate(db); err != nil {
		_ = db.Close()
		_ = lock.release()
		return nil, fmt.Errorf("migrate catalog: %w", err)
	}
	return &Catalog{DB: db, lock: lock}, nil
}

// Close closes the database and releases the instance lock. The DB error takes
// precedence; the lock is always released.
func (c *Catalog) Close() error {
	err := c.DB.Close()
	if lerr := c.lock.release(); err == nil {
		err = lerr
	}
	return err
}
