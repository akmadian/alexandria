//go:build !windows

package sqlite_test

import (
	"errors"
	"testing"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/sqlite"
)

func TestOpen_MigratesAndLocks(t *testing.T) {
	dir := t.TempDir()

	cat, err := sqlite.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer cat.Close()

	// Schema present: querying a known table proves migrations ran.
	if _, err := cat.DB.Exec(`SELECT count(*) FROM assets`); err != nil {
		t.Fatalf("schema not migrated: %v", err)
	}

	// Second open on the same dir must fail — the instance lock is held.
	second, err := sqlite.Open(dir)
	if err == nil {
		second.Close()
		t.Fatal("expected second Open to fail (lock held)")
	}
	var locked *domain.CatalogLockedError
	if !errors.As(err, &locked) {
		t.Fatalf("expected CatalogLockedError, got %v", err)
	}
}
