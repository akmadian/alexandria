package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

type SourceRepo struct {
	DB *sql.DB
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
	if err == sql.ErrNoRows {
		return nil, &domain.NotFoundError{Resource: "source", ID: id}
	}
	return s, err
}

func (r *SourceRepo) Create(ctx context.Context, s *domain.Source) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO sources
		(`+sourceColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Kind, s.BasePath, s.FilesystemUUID, s.DiskSerial, s.VolumeLabel,
		s.Host, s.ShareName, s.PollIntervalSecs, boolToInt(s.ScanRecursively), boolToInt(s.Enabled), s.Connectivity,
		formatTimePtr(s.LastScannedAt), formatTime(s.CreatedAt), formatTime(s.UpdatedAt))
	return err
}

func (r *SourceRepo) Update(ctx context.Context, s *domain.Source) error {
	s.UpdatedAt = time.Now().UTC()
	res, err := r.DB.ExecContext(ctx, `UPDATE sources SET
		name = ?, kind = ?, base_path = ?, filesystem_uuid = ?, disk_serial = ?,
		volume_label = ?, host = ?, share_name = ?, poll_interval_secs = ?,
		scan_recursively = ?, enabled = ?, connectivity = ?, last_scanned_at = ?, updated_at = ?
		WHERE id = ?`,
		s.Name, s.Kind, s.BasePath, s.FilesystemUUID, s.DiskSerial,
		s.VolumeLabel, s.Host, s.ShareName, s.PollIntervalSecs,
		boolToInt(s.ScanRecursively), boolToInt(s.Enabled), s.Connectivity, formatTimePtr(s.LastScannedAt),
		formatTime(s.UpdatedAt), s.ID)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "source", s.ID)
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
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (r *SourceRepo) FindBySharePath(ctx context.Context, host, shareName string) (*domain.Source, error) {
	row := r.DB.QueryRowContext(ctx, "SELECT "+sourceColumns+" FROM sources WHERE host = ? AND share_name = ?", host, shareName)
	s, err := scanSourceRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

type sourceScanner interface {
	Scan(dest ...any) error
}

func scanSourceFromRow(sc sourceScanner) (*domain.Source, error) {
	var s domain.Source
	var scanRecursively, enabled int
	var lastScannedAt sql.NullString
	var createdAt, updatedAt string
	var filesystemUUID, diskSerial, volumeLabel, host, shareName sql.NullString
	var pollInterval sql.NullInt64

	err := sc.Scan(&s.ID, &s.Name, &s.Kind, &s.BasePath,
		&filesystemUUID, &diskSerial, &volumeLabel,
		&host, &shareName, &pollInterval, &scanRecursively, &enabled, &s.Connectivity,
		&lastScannedAt, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	s.ScanRecursively = scanRecursively != 0
	s.Enabled = enabled != 0
	s.FilesystemUUID = nullStringPtr(filesystemUUID)
	s.DiskSerial = nullStringPtr(diskSerial)
	s.VolumeLabel = nullStringPtr(volumeLabel)
	s.Host = nullStringPtr(host)
	s.ShareName = nullStringPtr(shareName)
	if pollInterval.Valid {
		v := int(pollInterval.Int64)
		s.PollIntervalSecs = &v
	}
	s.LastScannedAt = parseNullTime(lastScannedAt)
	s.CreatedAt = parseTime(createdAt)
	s.UpdatedAt = parseTime(updatedAt)
	return &s, nil
}

func scanSource(rows *sql.Rows) (*domain.Source, error) {
	return scanSourceFromRow(rows)
}

func scanSourceRow(row *sql.Row) (*domain.Source, error) {
	return scanSourceFromRow(row)
}
