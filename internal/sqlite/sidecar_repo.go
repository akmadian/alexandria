package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

type SidecarRepo struct {
	DB DBTX
}

// UpsertObservation records (or refreshes) a sidecar's observation columns. It
// deliberately never touches attached_asset_id — that column is derived state
// owned by the grouping engine. Keyed on UNIQUE(volume_id, path_key), so a
// re-scan of an unchanged sidecar just bumps size/mtime/hash/updated_at.
func (r *SidecarRepo) UpsertObservation(ctx context.Context, sidecar *domain.SidecarFile) error {
	now := formatTime(time.Now().UTC())
	_, err := r.DB.ExecContext(ctx, `INSERT INTO sidecar_files
		(id, volume_id, dir, stem, ext, relative_path, path_key, size_bytes, mtime, partial_hash, first_seen_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(volume_id, path_key) DO UPDATE SET
			dir = excluded.dir, stem = excluded.stem, ext = excluded.ext,
			relative_path = excluded.relative_path,
			size_bytes = excluded.size_bytes, mtime = excluded.mtime,
			partial_hash = excluded.partial_hash, updated_at = excluded.updated_at`,
		sidecar.ID, sidecar.VolumeID, sidecar.Dir, sidecar.Stem, sidecar.Ext, sidecar.RelativePath,
		domain.PathKey(sidecar.RelativePath), sidecar.SizeBytes, formatTime(sidecar.MTime), sidecar.PartialHash, now, now)
	return err
}

// DeleteByPath removes a sidecar row (the file vanished from disk). Keyed on
// path_key (compare keys, open bytes).
func (r *SidecarRepo) DeleteByPath(ctx context.Context, volumeID, relativePath string) error {
	_, err := r.DB.ExecContext(ctx,
		"DELETE FROM sidecar_files WHERE volume_id = ? AND path_key = ?",
		volumeID, domain.PathKey(relativePath))
	return err
}

// ListByKey returns every sidecar sharing a (volume, dir, stem) key — the join
// the grouping engine walks to attach sidecars to their asset.
func (r *SidecarRepo) ListByKey(ctx context.Context, volumeID, directory, stem string) ([]*domain.SidecarFile, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT
		id, volume_id, dir, stem, ext, relative_path, size_bytes, mtime, partial_hash,
		attached_asset_id, first_seen_at, updated_at
		FROM sidecar_files WHERE volume_id = ? AND dir = ? AND stem = ?`,
		volumeID, directory, stem)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sidecars []*domain.SidecarFile
	for rows.Next() {
		sidecar, err := scanSidecar(rows)
		if err != nil {
			return nil, err
		}
		sidecars = append(sidecars, sidecar)
	}
	return sidecars, rows.Err()
}

func scanSidecar(scanner rowScanner) (*domain.SidecarFile, error) {
	var sidecar domain.SidecarFile
	var mtime, firstSeenAt, updatedAt string
	var attachedAssetID sql.NullString
	if err := scanner.Scan(&sidecar.ID, &sidecar.VolumeID, &sidecar.Dir, &sidecar.Stem, &sidecar.Ext, &sidecar.RelativePath,
		&sidecar.SizeBytes, &mtime, &sidecar.PartialHash, &attachedAssetID, &firstSeenAt, &updatedAt); err != nil {
		return nil, err
	}
	sidecar.MTime = parseTime(mtime)
	sidecar.AttachedAssetID = nullStringPtr(attachedAssetID)
	sidecar.FirstSeenAt = parseTime(firstSeenAt)
	sidecar.UpdatedAt = parseTime(updatedAt)
	return &sidecar, nil
}
