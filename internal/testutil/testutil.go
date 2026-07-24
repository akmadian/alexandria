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

// NewTestVolume inserts a minimal local volume and returns it (the D24 identity
// anchor). ID is "vol-<name>".
func NewTestVolume(t *testing.T, db *sql.DB, name string) *domain.Volume {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	uuid := "fs-" + name
	volume := &domain.Volume{
		ID:             "vol-" + name,
		Name:           name,
		Kind:           domain.VolumeKindLocal,
		FilesystemUUID: &uuid,
		Connectivity:   domain.VolumeOnline,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err := db.Exec(`INSERT INTO volumes (id, name, kind, filesystem_uuid, connectivity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		volume.ID, volume.Name, volume.Kind, volume.FilesystemUUID, volume.Connectivity,
		volume.CreatedAt.Format(time.RFC3339), volume.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert test volume: %v", err)
	}
	return volume
}

// NewTestFolder inserts a tracked folder (root path) on a volume and returns it.
func NewTestFolder(t *testing.T, db *sql.DB, volumeID, path string) *domain.Folder {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	folder := &domain.Folder{
		ID:              "folder-" + volumeID + "-" + path,
		VolumeID:        volumeID,
		Path:            path,
		Name:            path,
		SyncMode:        domain.SyncModeManual,
		ScanRecursively: true,
		Enabled:         true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err := db.Exec(`INSERT INTO folders (id, volume_id, path, name, sync_mode, scan_recursively, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		folder.ID, folder.VolumeID, folder.Path, folder.Name, folder.SyncMode,
		boolToInt(folder.ScanRecursively), boolToInt(folder.Enabled),
		folder.CreatedAt.Format(time.RFC3339), folder.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert test folder: %v", err)
	}
	return folder
}

// NewTestAsset inserts a minimal asset under the given volume and returns it.
func NewTestAsset(t *testing.T, db *sql.DB, volumeID, filename string) *domain.Asset {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	asset := &domain.Asset{
		ID:           "asset-" + filename,
		VolumeID:     volumeID,
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
		(id, volume_id, relative_path, path_key, file_status, filename, extension, mime_type, file_type,
		 size_bytes, mtime, partial_hash, ingested_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		asset.ID, asset.VolumeID, asset.RelativePath, domain.PathKey(asset.RelativePath), asset.FileStatus,
		asset.Filename, asset.Extension, asset.MIMEType, asset.FileType, asset.SizeBytes, asset.MTime.Format(time.RFC3339),
		asset.PartialHash, asset.IngestedAt.Format(time.RFC3339), asset.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert test asset: %v", err)
	}
	return asset
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
