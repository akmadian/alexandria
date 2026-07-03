package catalog

import (
	"context"
	"database/sql"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

type SQLiteSourceRepo struct {
	DB *sql.DB
}

func (r *SQLiteSourceRepo) List(ctx context.Context) ([]*domain.Source, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT
		id, name, kind, base_path, filesystem_uuid, disk_serial, volume_label,
		host, share_name, poll_interval_secs, scan_recursively, status,
		last_scanned_at, created_at, updated_at
		FROM sources`)
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

func (r *SQLiteSourceRepo) Get(ctx context.Context, id string) (*domain.Source, error) {
	row := r.DB.QueryRowContext(ctx, `SELECT
		id, name, kind, base_path, filesystem_uuid, disk_serial, volume_label,
		host, share_name, poll_interval_secs, scan_recursively, status,
		last_scanned_at, created_at, updated_at
		FROM sources WHERE id = ?`, id)

	s, err := scanSourceRow(row)
	if err == sql.ErrNoRows {
		return nil, &domain.NotFoundError{Resource: "source", ID: id}
	}
	return s, err
}

func (r *SQLiteSourceRepo) Create(ctx context.Context, s *domain.Source) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO sources
		(id, name, kind, base_path, filesystem_uuid, disk_serial, volume_label,
		 host, share_name, poll_interval_secs, scan_recursively, status,
		 last_scanned_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Kind, s.BasePath, s.FilesystemUUID, s.DiskSerial, s.VolumeLabel,
		s.Host, s.ShareName, s.PollIntervalSecs, boolToInt(s.ScanRecursively), s.Status,
		formatTimePtr(s.LastScannedAt), formatTime(s.CreatedAt), formatTime(s.UpdatedAt))
	return err
}

func (r *SQLiteSourceRepo) Update(ctx context.Context, s *domain.Source) error {
	s.UpdatedAt = time.Now().UTC()
	res, err := r.DB.ExecContext(ctx, `UPDATE sources SET
		name = ?, kind = ?, base_path = ?, filesystem_uuid = ?, disk_serial = ?,
		volume_label = ?, host = ?, share_name = ?, poll_interval_secs = ?,
		scan_recursively = ?, status = ?, last_scanned_at = ?, updated_at = ?
		WHERE id = ?`,
		s.Name, s.Kind, s.BasePath, s.FilesystemUUID, s.DiskSerial,
		s.VolumeLabel, s.Host, s.ShareName, s.PollIntervalSecs,
		boolToInt(s.ScanRecursively), s.Status, formatTimePtr(s.LastScannedAt),
		formatTime(s.UpdatedAt), s.ID)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "source", s.ID)
}

func (r *SQLiteSourceRepo) UpdateStatus(ctx context.Context, id string, status domain.SourceStatus) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE sources SET status = ?, updated_at = ? WHERE id = ?`,
		status, formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "source", id)
}

func (r *SQLiteSourceRepo) FindByFilesystemUUID(ctx context.Context, uuid string) (*domain.Source, error) {
	row := r.DB.QueryRowContext(ctx, `SELECT
		id, name, kind, base_path, filesystem_uuid, disk_serial, volume_label,
		host, share_name, poll_interval_secs, scan_recursively, status,
		last_scanned_at, created_at, updated_at
		FROM sources WHERE filesystem_uuid = ?`, uuid)
	s, err := scanSourceRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (r *SQLiteSourceRepo) FindBySharePath(ctx context.Context, host, shareName string) (*domain.Source, error) {
	row := r.DB.QueryRowContext(ctx, `SELECT
		id, name, kind, base_path, filesystem_uuid, disk_serial, volume_label,
		host, share_name, poll_interval_secs, scan_recursively, status,
		last_scanned_at, created_at, updated_at
		FROM sources WHERE host = ? AND share_name = ?`, host, shareName)
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
	var scanRecursively int
	var lastScannedAt sql.NullString
	var createdAt, updatedAt string
	var filesystemUUID, diskSerial, volumeLabel, host, shareName sql.NullString
	var pollInterval sql.NullInt64

	err := sc.Scan(&s.ID, &s.Name, &s.Kind, &s.BasePath,
		&filesystemUUID, &diskSerial, &volumeLabel,
		&host, &shareName, &pollInterval, &scanRecursively, &s.Status,
		&lastScannedAt, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	s.ScanRecursively = scanRecursively != 0
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
