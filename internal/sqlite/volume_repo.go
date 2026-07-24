package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

// VolumeRepo is the identity/portability-anchor adapter (D24 split). A volume is
// found-or-created by filesystem UUID (the path resolver's job); the mount point
// is never stored — it is resolved live by the volume prober.
type VolumeRepo struct {
	DB DBTX
}

const volumeColumns = `id, name, kind, host, share_name, filesystem_uuid, disk_serial,
	volume_label, connectivity, created_at, updated_at`

func (r *VolumeRepo) List(ctx context.Context) ([]*domain.Volume, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT "+volumeColumns+" FROM volumes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var volumes []*domain.Volume
	for rows.Next() {
		volume, err := scanVolume(rows)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, volume)
	}
	return volumes, rows.Err()
}

func (r *VolumeRepo) Get(ctx context.Context, id string) (*domain.Volume, error) {
	row := r.DB.QueryRowContext(ctx, "SELECT "+volumeColumns+" FROM volumes WHERE id = ?", id)
	volume, err := scanVolumeRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.NotFoundError{Resource: "volume", ID: id}
	}
	return volume, err
}

func (r *VolumeRepo) Create(ctx context.Context, volume *domain.Volume) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO volumes
		(`+volumeColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		volume.ID, volume.Name, volume.Kind, volume.Host, volume.ShareName, volume.FilesystemUUID,
		volume.DiskSerial, volume.VolumeLabel, volume.Connectivity,
		formatTime(volume.CreatedAt), formatTime(volume.UpdatedAt))
	return err
}

func (r *VolumeRepo) Update(ctx context.Context, volume *domain.Volume) error {
	volume.UpdatedAt = time.Now().UTC()
	res, err := r.DB.ExecContext(ctx, `UPDATE volumes SET
		name = ?, kind = ?, host = ?, share_name = ?, filesystem_uuid = ?, disk_serial = ?,
		volume_label = ?, connectivity = ?, updated_at = ?
		WHERE id = ?`,
		volume.Name, volume.Kind, volume.Host, volume.ShareName, volume.FilesystemUUID, volume.DiskSerial,
		volume.VolumeLabel, volume.Connectivity, formatTime(volume.UpdatedAt), volume.ID)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "volume", volume.ID)
}

// SetConnectivity records observed reachability (online/offline). Observation
// write — the mount monitor and reconciler own it.
func (r *VolumeRepo) SetConnectivity(ctx context.Context, id string, c domain.VolumeConnectivity) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE volumes SET connectivity = ?, updated_at = ? WHERE id = ?`,
		c, formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "volume", id)
}

// FindByFilesystemUUID is the identity lookup behind the path resolver's
// find-or-create. A nil volume (no error) means no volume carries this UUID yet.
func (r *VolumeRepo) FindByFilesystemUUID(ctx context.Context, uuid string) (*domain.Volume, error) {
	row := r.DB.QueryRowContext(ctx, "SELECT "+volumeColumns+" FROM volumes WHERE filesystem_uuid = ?", uuid)
	volume, err := scanVolumeRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return volume, err
}

func scanVolumeFromRow(scanner rowScanner) (*domain.Volume, error) {
	var volume domain.Volume
	var createdAt, updatedAt string
	var host, shareName, filesystemUUID, diskSerial, volumeLabel sql.NullString

	err := scanner.Scan(&volume.ID, &volume.Name, &volume.Kind, &host, &shareName,
		&filesystemUUID, &diskSerial, &volumeLabel, &volume.Connectivity, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	volume.Host = nullStringPtr(host)
	volume.ShareName = nullStringPtr(shareName)
	volume.FilesystemUUID = nullStringPtr(filesystemUUID)
	volume.DiskSerial = nullStringPtr(diskSerial)
	volume.VolumeLabel = nullStringPtr(volumeLabel)
	volume.CreatedAt = parseTime(createdAt)
	volume.UpdatedAt = parseTime(updatedAt)
	return &volume, nil
}

func scanVolume(rows *sql.Rows) (*domain.Volume, error)  { return scanVolumeFromRow(rows) }
func scanVolumeRow(row *sql.Row) (*domain.Volume, error) { return scanVolumeFromRow(row) }
