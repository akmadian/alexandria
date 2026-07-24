package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

// FolderRepo is the tracked-root adapter (D24 split): the directories the catalog
// walks/watches. Roots on one volume are disjoint by invariant (D41) — the
// folder-add engine (internal/volume) enforces disjointness; this repo is plain
// CRUD.
type FolderRepo struct {
	DB DBTX
}

const folderColumns = `id, volume_id, path, name, sync_mode, scan_recursively, enabled,
	poll_interval_secs, last_scanned_at, created_at, updated_at`

func (r *FolderRepo) List(ctx context.Context) ([]*domain.Folder, error) {
	return r.query(ctx, "SELECT "+folderColumns+" FROM folders")
}

func (r *FolderRepo) ListByVolume(ctx context.Context, volumeID string) ([]*domain.Folder, error) {
	return r.query(ctx, "SELECT "+folderColumns+" FROM folders WHERE volume_id = ?", volumeID)
}

func (r *FolderRepo) query(ctx context.Context, sql string, args ...any) ([]*domain.Folder, error) {
	rows, err := r.DB.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []*domain.Folder
	for rows.Next() {
		folder, err := scanFolder(rows)
		if err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}
	return folders, rows.Err()
}

func (r *FolderRepo) Get(ctx context.Context, id string) (*domain.Folder, error) {
	row := r.DB.QueryRowContext(ctx, "SELECT "+folderColumns+" FROM folders WHERE id = ?", id)
	folder, err := scanFolderRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.NotFoundError{Resource: "folder", ID: id}
	}
	return folder, err
}

func (r *FolderRepo) Create(ctx context.Context, folder *domain.Folder) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO folders
		(`+folderColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		folder.ID, folder.VolumeID, folder.Path, folder.Name, folder.SyncMode,
		boolToInt(folder.ScanRecursively), boolToInt(folder.Enabled), folder.PollIntervalSecs,
		formatTimePtr(folder.LastScannedAt), formatTime(folder.CreatedAt), formatTime(folder.UpdatedAt))
	return err
}

// Update is the user-action whole-row write. It deliberately does NOT touch
// last_scanned_at — that is a [syn] cursor owned by SetLastScannedAt (the
// scanner's narrow writer), and a user edit racing a scan must not clobber it.
func (r *FolderRepo) Update(ctx context.Context, folder *domain.Folder) error {
	folder.UpdatedAt = time.Now().UTC()
	res, err := r.DB.ExecContext(ctx, `UPDATE folders SET
		volume_id = ?, path = ?, name = ?, sync_mode = ?, scan_recursively = ?, enabled = ?,
		poll_interval_secs = ?, updated_at = ?
		WHERE id = ?`,
		folder.VolumeID, folder.Path, folder.Name, folder.SyncMode,
		boolToInt(folder.ScanRecursively), boolToInt(folder.Enabled), folder.PollIntervalSecs,
		formatTime(folder.UpdatedAt), folder.ID)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "folder", folder.ID)
}

// SetLastScannedAt records a scan completion — the folder table's one [syn]
// column. This is the ONLY writer of last_scanned_at (catalog.FolderScanRecorder);
// the scanner holds this narrow slice, never the fat Update.
func (r *FolderRepo) SetLastScannedAt(ctx context.Context, id string, scannedAt time.Time) error {
	res, err := r.DB.ExecContext(ctx,
		"UPDATE folders SET last_scanned_at = ?, updated_at = ? WHERE id = ?",
		formatTime(scannedAt), formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "folder", id)
}

func (r *FolderRepo) Delete(ctx context.Context, id string) error {
	_, err := r.DB.ExecContext(ctx, "DELETE FROM folders WHERE id = ?", id)
	return err
}

func scanFolderFromRow(scanner rowScanner) (*domain.Folder, error) {
	var folder domain.Folder
	var scanRecursively, enabled int
	var pollInterval sql.NullInt64
	var lastScannedAt sql.NullString
	var createdAt, updatedAt string

	err := scanner.Scan(&folder.ID, &folder.VolumeID, &folder.Path, &folder.Name, &folder.SyncMode,
		&scanRecursively, &enabled, &pollInterval, &lastScannedAt, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	folder.ScanRecursively = scanRecursively != 0
	folder.Enabled = enabled != 0
	if pollInterval.Valid {
		v := int(pollInterval.Int64)
		folder.PollIntervalSecs = &v
	}
	folder.LastScannedAt = parseNullTime(lastScannedAt)
	folder.CreatedAt = parseTime(createdAt)
	folder.UpdatedAt = parseTime(updatedAt)
	return &folder, nil
}

func scanFolder(rows *sql.Rows) (*domain.Folder, error)  { return scanFolderFromRow(rows) }
func scanFolderRow(row *sql.Row) (*domain.Folder, error) { return scanFolderFromRow(row) }
