package sqlite

import (
	"context"
	"database/sql"

	"github.com/akmadian/alexandria/internal/domain"
)

type DuplicateRepo struct {
	DB *sql.DB
}

// Log records a duplicate pair. INSERT OR IGNORE makes re-detection a no-op (the
// UNIQUE(original, duplicate) constraint), so logging is idempotent across runs.
func (r *DuplicateRepo) Log(ctx context.Context, dup *domain.Duplicate) error {
	_, err := r.DB.ExecContext(ctx, `INSERT OR IGNORE INTO duplicates
		(id, original_asset_id, duplicate_asset_id, partial_hash, detected_at, status, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		dup.ID, dup.OriginalAssetID, dup.DuplicateAssetID, dup.PartialHash,
		formatTime(dup.DetectedAt), dup.Status, formatTimePtr(dup.ResolvedAt))
	return err
}

func (r *DuplicateRepo) ListPending(ctx context.Context) ([]*domain.Duplicate, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT
		id, original_asset_id, duplicate_asset_id, partial_hash, detected_at, status, resolved_at
		FROM duplicates WHERE status = 'pending'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dups []*domain.Duplicate
	for rows.Next() {
		var d domain.Duplicate
		var detectedAt string
		var resolvedAt sql.NullString
		if err := rows.Scan(&d.ID, &d.OriginalAssetID, &d.DuplicateAssetID,
			&d.PartialHash, &detectedAt, &d.Status, &resolvedAt); err != nil {
			return nil, err
		}
		d.DetectedAt = parseTime(detectedAt)
		d.ResolvedAt = parseNullTime(resolvedAt)
		dups = append(dups, &d)
	}
	return dups, rows.Err()
}
