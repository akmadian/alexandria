package testutil

import (
	"database/sql"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/migrations"
	_ "modernc.org/sqlite"
)

// NewTestDB returns a migrated in-memory SQLite database. The connection is
// limited to one open conn to avoid the "each connection gets a fresh DB" trap.
func NewTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if err := migrations.Migrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	return db
}

// NewTestSource inserts a minimal local source and returns it.
func NewTestSource(t *testing.T, db *sql.DB, name string) *domain.Source {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	s := &domain.Source{
		ID:              "src-" + name,
		Name:            name,
		Kind:            domain.SourceKindLocal,
		BasePath:        "/tmp/test/" + name,
		ScanRecursively: true,
		Enabled:         true,
		Connectivity:    domain.SourceOnline,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err := db.Exec(`INSERT INTO sources (id, name, kind, base_path, scan_recursively, enabled, connectivity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Kind, s.BasePath, boolToInt(s.ScanRecursively), boolToInt(s.Enabled), s.Connectivity,
		s.CreatedAt.Format(time.RFC3339), s.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert test source: %v", err)
	}
	return s
}

// NewTestAsset inserts a minimal asset under the given source and returns it.
func NewTestAsset(t *testing.T, db *sql.DB, sourceID, filename string) *domain.Asset {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	a := &domain.Asset{
		ID:           "asset-" + filename,
		SourceID:     sourceID,
		RelativePath: filename,
		FileStatus:   domain.FileStatusOnline,
		Filename:     filename,
		Extension:    "jpg",
		MIMEType:     "image/jpeg",
		FileType:     domain.FileTypeImage,
		SizeBytes:    1024,
		MTime:        now,
		PartialHash:  "testhash-" + filename,
		IngestedAt:   now,
		UpdatedAt:    now,
	}
	_, err := db.Exec(`INSERT INTO assets
		(id, source_id, relative_path, file_status, filename, extension, mime_type, file_type,
		 size_bytes, mtime, partial_hash, ingested_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.SourceID, a.RelativePath, a.FileStatus, a.Filename, a.Extension,
		a.MIMEType, a.FileType, a.SizeBytes, a.MTime.Format(time.RFC3339),
		a.PartialHash, a.IngestedAt.Format(time.RFC3339), a.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert test asset: %v", err)
	}
	return a
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
