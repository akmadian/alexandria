package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

type SourceRepo struct {
	DB DBTX
}

const sourceColumns = `id, name, kind, base_path, filesystem_uuid, disk_serial, volume_label,
	host, share_name, poll_interval_secs, scan_recursively, enabled, connectivity,
	last_scanned_at, created_at, updated_at`

func (r *SourceRepo) List(ctx context.Context) ([]*domain.Source, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT "+sourceColumns+" FROM sources")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sources []*domain.Source
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		sources = append(sources, s)
	}
	return sources, rows.Err()
}

func (r *SourceRepo) Get(ctx context.Context, id string) (*domain.Source, error) {
	row := r.DB.QueryRowContext(ctx, "SELECT "+sourceColumns+" FROM sources WHERE id = ?", id)
	s, err := scanSourceRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.NotFoundError{Resource: "source", ID: id}
	}
	return s, err
}

func (r *SourceRepo) Create(ctx context.Context, source *domain.Source) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO sources
		(`+sourceColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		source.ID, source.Name, source.Kind, source.BasePath, source.FilesystemUUID, source.DiskSerial, source.VolumeLabel,
		source.Host, source.ShareName, source.PollIntervalSecs, boolToInt(source.ScanRecursively), boolToInt(source.Enabled), source.Connectivity,
		formatTimePtr(source.LastScannedAt), formatTime(source.CreatedAt), formatTime(source.UpdatedAt))
	return err
}

func (r *SourceRepo) Update(ctx context.Context, source *domain.Source) error {
	source.UpdatedAt = time.Now().UTC()
	res, err := r.DB.ExecContext(ctx, `UPDATE sources SET
		name = ?, kind = ?, base_path = ?, filesystem_uuid = ?, disk_serial = ?,
		volume_label = ?, host = ?, share_name = ?, poll_interval_secs = ?,
		scan_recursively = ?, enabled = ?, connectivity = ?, last_scanned_at = ?, updated_at = ?
		WHERE id = ?`,
		source.Name, source.Kind, source.BasePath, source.FilesystemUUID, source.DiskSerial,
		source.VolumeLabel, source.Host, source.ShareName, source.PollIntervalSecs,
		boolToInt(source.ScanRecursively), boolToInt(source.Enabled), source.Connectivity, formatTimePtr(source.LastScannedAt),
		formatTime(source.UpdatedAt), source.ID)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "source", source.ID)
}

// SetConnectivity records observed reachability (online/offline). Observation
// write — the mount monitor and reconciler own it; it never touches Enabled.
func (r *SourceRepo) SetConnectivity(ctx context.Context, id string, c domain.SourceConnectivity) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE sources SET connectivity = ?, updated_at = ? WHERE id = ?`,
		c, formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "source", id)
}

func (r *SourceRepo) FindByFilesystemUUID(ctx context.Context, uuid string) (*domain.Source, error) {
	row := r.DB.QueryRowContext(ctx, "SELECT "+sourceColumns+" FROM sources WHERE filesystem_uuid = ?", uuid)
	s, err := scanSourceRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

func (r *SourceRepo) FindBySharePath(ctx context.Context, host, shareName string) (*domain.Source, error) {
	row := r.DB.QueryRowContext(ctx, "SELECT "+sourceColumns+" FROM sources WHERE host = ? AND share_name = ?", host, shareName)
	s, err := scanSourceRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

type sourceScanner interface {
	Scan(dest ...any) error
}

func scanSourceFromRow(scanner sourceScanner) (*domain.Source, error) {
	var source domain.Source
	var scanRecursively, enabled int
	var lastScannedAt sql.NullString
	var createdAt, updatedAt string
	var filesystemUUID, diskSerial, volumeLabel, host, shareName sql.NullString
	var pollInterval sql.NullInt64

	err := scanner.Scan(&source.ID, &source.Name, &source.Kind, &source.BasePath,
		&filesystemUUID, &diskSerial, &volumeLabel,
		&host, &shareName, &pollInterval, &scanRecursively, &enabled, &source.Connectivity,
		&lastScannedAt, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	source.ScanRecursively = scanRecursively != 0
	source.Enabled = enabled != 0
	source.FilesystemUUID = nullStringPtr(filesystemUUID)
	source.DiskSerial = nullStringPtr(diskSerial)
	source.VolumeLabel = nullStringPtr(volumeLabel)
	source.Host = nullStringPtr(host)
	source.ShareName = nullStringPtr(shareName)
	if pollInterval.Valid {
		v := int(pollInterval.Int64)
		source.PollIntervalSecs = &v
	}
	source.LastScannedAt = parseNullTime(lastScannedAt)
	source.CreatedAt = parseTime(createdAt)
	source.UpdatedAt = parseTime(updatedAt)
	return &source, nil
}

func scanSource(rows *sql.Rows) (*domain.Source, error) {
	return scanSourceFromRow(rows)
}

func scanSourceRow(row *sql.Row) (*domain.Source, error) {
	return scanSourceFromRow(row)
}
