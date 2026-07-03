package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
)

type AssetRepo struct {
	DB *sql.DB
}

const assetColumns = `id, source_id, relative_path, file_status, last_verified_at,
	filename, extension, mime_type, file_type, size_bytes, mtime, partial_hash,
	width, height, duration_secs, color_space, bit_depth,
	captured_at, camera_make, camera_model, lens_model, focal_length_mm,
	aperture, shutter_speed, iso, gps_lat, gps_lon,
	extended_metadata, rating, color_label, flag, note,
	xmp_last_read_at, xmp_last_written_at, xmp_hash,
	thumbnail_path, thumbnail_at, is_deleted, deleted_at, ingested_at, updated_at`

func (r *AssetRepo) Get(ctx context.Context, id string) (*domain.Asset, error) {
	row := r.DB.QueryRowContext(ctx, "SELECT "+assetColumns+" FROM assets WHERE id = ?", id)
	a, err := scanAssetRow(row)
	if err == sql.ErrNoRows {
		return nil, &domain.NotFoundError{Resource: "asset", ID: id}
	}
	return a, err
}

func (r *AssetRepo) Create(ctx context.Context, a *domain.Asset) error {
	extJSON, err := marshalExtended(a.ExtendedMetadata)
	if err != nil {
		return err
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO assets
		(`+assetColumns+`) VALUES
		(?,?,?,?,?, ?,?,?,?,?,?,?, ?,?,?,?,?, ?,?,?,?,?, ?,?,?,?,?, ?,?,?,?,?, ?,?,?, ?,?,?,?,?,?)`,
		a.ID, a.SourceID, a.RelativePath, a.FileStatus, formatTimePtr(a.LastVerifiedAt),
		a.Filename, a.Extension, a.MIMEType, a.FileType, a.SizeBytes, formatTime(a.MTime), a.PartialHash,
		a.Width, a.Height, a.DurationSecs, a.ColorSpace, a.BitDepth,
		formatTimePtr(a.CapturedAt), a.CameraMake, a.CameraModel, a.LensModel, a.FocalLengthMM,
		a.Aperture, a.ShutterSpeed, a.ISO, a.GPSLat, a.GPSLon,
		extJSON, a.Rating, nilColorLabel(a.ColorLabel), nilFlag(a.Flag), a.Note,
		formatTimePtr(a.XMPLastReadAt), formatTimePtr(a.XMPLastWrittenAt), a.XMPHash,
		a.ThumbnailPath, formatTimePtr(a.ThumbnailAt),
		boolToInt(a.IsDeleted), formatTimePtr(a.DeletedAt),
		formatTime(a.IngestedAt), formatTime(a.UpdatedAt))
	return err
}

func (r *AssetRepo) Update(ctx context.Context, a *domain.Asset) error {
	a.UpdatedAt = time.Now().UTC()
	extJSON, err := marshalExtended(a.ExtendedMetadata)
	if err != nil {
		return err
	}
	res, err := r.DB.ExecContext(ctx, `UPDATE assets SET
		source_id=?, relative_path=?, file_status=?, last_verified_at=?,
		filename=?, extension=?, mime_type=?, file_type=?, size_bytes=?, mtime=?, partial_hash=?,
		width=?, height=?, duration_secs=?, color_space=?, bit_depth=?,
		captured_at=?, camera_make=?, camera_model=?, lens_model=?, focal_length_mm=?,
		aperture=?, shutter_speed=?, iso=?, gps_lat=?, gps_lon=?,
		extended_metadata=?, rating=?, color_label=?, flag=?, note=?,
		xmp_last_read_at=?, xmp_last_written_at=?, xmp_hash=?,
		thumbnail_path=?, thumbnail_at=?, is_deleted=?, deleted_at=?, updated_at=?
		WHERE id=?`,
		a.SourceID, a.RelativePath, a.FileStatus, formatTimePtr(a.LastVerifiedAt),
		a.Filename, a.Extension, a.MIMEType, a.FileType, a.SizeBytes, formatTime(a.MTime), a.PartialHash,
		a.Width, a.Height, a.DurationSecs, a.ColorSpace, a.BitDepth,
		formatTimePtr(a.CapturedAt), a.CameraMake, a.CameraModel, a.LensModel, a.FocalLengthMM,
		a.Aperture, a.ShutterSpeed, a.ISO, a.GPSLat, a.GPSLon,
		extJSON, a.Rating, nilColorLabel(a.ColorLabel), nilFlag(a.Flag), a.Note,
		formatTimePtr(a.XMPLastReadAt), formatTimePtr(a.XMPLastWrittenAt), a.XMPHash,
		a.ThumbnailPath, formatTimePtr(a.ThumbnailAt),
		boolToInt(a.IsDeleted), formatTimePtr(a.DeletedAt), formatTime(a.UpdatedAt),
		a.ID)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "asset", a.ID)
}

func (r *AssetRepo) Patch(ctx context.Context, id string, patch catalog.AssetPatch) error {
	setClauses, args := buildPatchSQL(patch)
	if len(setClauses) == 0 {
		return nil
	}
	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, formatTime(time.Now().UTC()))
	args = append(args, id)

	q := "UPDATE assets SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	res, err := r.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "asset", id)
}

func (r *AssetRepo) BulkPatch(ctx context.Context, ids []string, patch catalog.AssetPatch) error {
	if len(ids) == 0 {
		return nil
	}
	setClauses, args := buildPatchSQL(patch)
	if len(setClauses) == 0 {
		return nil
	}
	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, formatTime(time.Now().UTC()))

	placeholders := make([]string, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}

	q := "UPDATE assets SET " + strings.Join(setClauses, ", ") +
		" WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	_, err := r.DB.ExecContext(ctx, q, args...)
	return err
}

func (r *AssetRepo) SoftDelete(ctx context.Context, id string) error {
	now := formatTime(time.Now().UTC())
	res, err := r.DB.ExecContext(ctx,
		"UPDATE assets SET is_deleted = 1, deleted_at = ?, updated_at = ? WHERE id = ?",
		now, now, id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "asset", id)
}

func (r *AssetRepo) FindByHash(ctx context.Context, hash string, sizeBytes int64) (*domain.Asset, error) {
	row := r.DB.QueryRowContext(ctx,
		"SELECT "+assetColumns+" FROM assets WHERE partial_hash = ? AND size_bytes = ? AND is_deleted = 0",
		hash, sizeBytes)
	a, err := scanAssetRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (r *AssetRepo) FindBySourcePath(ctx context.Context, sourceID, relativePath string) (*domain.Asset, error) {
	row := r.DB.QueryRowContext(ctx,
		"SELECT "+assetColumns+" FROM assets WHERE source_id = ? AND relative_path = ?",
		sourceID, relativePath)
	a, err := scanAssetRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (r *AssetRepo) UpdatePath(ctx context.Context, assetID, sourceID, relativePath string) error {
	res, err := r.DB.ExecContext(ctx,
		"UPDATE assets SET source_id = ?, relative_path = ?, updated_at = ? WHERE id = ?",
		sourceID, relativePath, formatTime(time.Now().UTC()), assetID)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "asset", assetID)
}

func (r *AssetRepo) UpdateFileStatus(ctx context.Context, assetID string, status domain.FileStatus) error {
	res, err := r.DB.ExecContext(ctx,
		"UPDATE assets SET file_status = ?, updated_at = ? WHERE id = ?",
		status, formatTime(time.Now().UTC()), assetID)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "asset", assetID)
}

func (r *AssetRepo) MarkOfflineBySource(ctx context.Context, sourceID string) error {
	_, err := r.DB.ExecContext(ctx,
		"UPDATE assets SET file_status = ? WHERE source_id = ? AND file_status = ?",
		domain.FileStatusOffline, sourceID, domain.FileStatusOnline)
	return err
}

func (r *AssetRepo) MarkOnlineBySource(ctx context.Context, sourceID string) error {
	_, err := r.DB.ExecContext(ctx,
		"UPDATE assets SET file_status = ? WHERE source_id = ? AND file_status = ?",
		domain.FileStatusOnline, sourceID, domain.FileStatusOffline)
	return err
}

func (r *AssetRepo) List(ctx context.Context, filter catalog.AssetFilter) ([]*domain.Asset, error) {
	where, args := buildFilterSQL(filter)
	q := "SELECT " + assetColumns + " FROM assets"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}

	sortField := "ingested_at"
	if filter.SortField != "" {
		sortField = filter.SortField
	}
	sortDir := "DESC"
	if filter.SortDir == "asc" {
		sortDir = "ASC"
	}
	q += " ORDER BY " + sortField + " " + sortDir

	if filter.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			q += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []*domain.Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	return assets, rows.Err()
}

// --- filter/patch SQL builders ---

func buildFilterSQL(f catalog.AssetFilter) ([]string, []any) {
	var where []string
	var args []any

	if !f.IncludeDeleted {
		where = append(where, "is_deleted = 0")
	}
	if len(f.FileTypes) > 0 {
		ph := make([]string, len(f.FileTypes))
		for i, ft := range f.FileTypes {
			ph[i] = "?"
			args = append(args, string(ft))
		}
		where = append(where, "file_type IN ("+strings.Join(ph, ",")+")")
	}
	if f.Rating != nil {
		where = append(where, "rating = ?")
		args = append(args, *f.Rating)
	}
	if f.RatingMin != nil {
		where = append(where, "rating >= ?")
		args = append(args, *f.RatingMin)
	}
	if len(f.ColorLabels) > 0 {
		ph := make([]string, len(f.ColorLabels))
		for i, cl := range f.ColorLabels {
			ph[i] = "?"
			args = append(args, string(cl))
		}
		where = append(where, "color_label IN ("+strings.Join(ph, ",")+")")
	}
	if len(f.Flags) > 0 {
		ph := make([]string, len(f.Flags))
		for i, fl := range f.Flags {
			ph[i] = "?"
			args = append(args, string(fl))
		}
		where = append(where, "flag IN ("+strings.Join(ph, ",")+")")
	}
	if len(f.SourceIDs) > 0 {
		ph := make([]string, len(f.SourceIDs))
		for i, sid := range f.SourceIDs {
			ph[i] = "?"
			args = append(args, sid)
		}
		where = append(where, "source_id IN ("+strings.Join(ph, ",")+")")
	}
	if f.CapturedAfter != nil {
		where = append(where, "captured_at >= ?")
		args = append(args, formatTime(*f.CapturedAfter))
	}
	if f.CapturedBefore != nil {
		where = append(where, "captured_at <= ?")
		args = append(args, formatTime(*f.CapturedBefore))
	}
	if len(f.TagIDs) > 0 {
		ph := make([]string, len(f.TagIDs))
		for i, tid := range f.TagIDs {
			ph[i] = "?"
			args = append(args, tid)
		}
		where = append(where, "id IN (SELECT asset_id FROM asset_tags WHERE tag_id IN ("+strings.Join(ph, ",")+"))")
	}
	if f.SearchText != "" {
		where = append(where, "id IN (SELECT asset_id FROM assets_fts WHERE assets_fts MATCH ?)")
		args = append(args, f.SearchText)
	}

	return where, args
}

func buildPatchSQL(p catalog.AssetPatch) ([]string, []any) {
	var clauses []string
	var args []any

	if p.Rating.Set {
		clauses = append(clauses, "rating = ?")
		args = append(args, p.Rating.Value)
	}
	if p.ColorLabel.Set {
		clauses = append(clauses, "color_label = ?")
		if p.ColorLabel.Value != nil {
			args = append(args, string(*p.ColorLabel.Value))
		} else {
			args = append(args, nil)
		}
	}
	if p.Flag.Set {
		clauses = append(clauses, "flag = ?")
		if p.Flag.Value != nil {
			args = append(args, string(*p.Flag.Value))
		} else {
			args = append(args, nil)
		}
	}
	if p.Note.Set {
		clauses = append(clauses, "note = ?")
		args = append(args, p.Note.Value)
	}
	if p.ThumbnailPath.Set {
		clauses = append(clauses, "thumbnail_path = ?")
		args = append(args, p.ThumbnailPath.Value)
	}
	if p.ThumbnailAt.Set {
		clauses = append(clauses, "thumbnail_at = ?")
		if p.ThumbnailAt.Value != nil {
			args = append(args, formatTime(*p.ThumbnailAt.Value))
		} else {
			args = append(args, nil)
		}
	}
	if p.XMPLastReadAt.Set {
		clauses = append(clauses, "xmp_last_read_at = ?")
		if p.XMPLastReadAt.Value != nil {
			args = append(args, formatTime(*p.XMPLastReadAt.Value))
		} else {
			args = append(args, nil)
		}
	}
	if p.XMPLastWrittenAt.Set {
		clauses = append(clauses, "xmp_last_written_at = ?")
		if p.XMPLastWrittenAt.Value != nil {
			args = append(args, formatTime(*p.XMPLastWrittenAt.Value))
		} else {
			args = append(args, nil)
		}
	}
	if p.XMPHash.Set {
		clauses = append(clauses, "xmp_hash = ?")
		args = append(args, p.XMPHash.Value)
	}
	if p.IsDeleted.Set {
		clauses = append(clauses, "is_deleted = ?")
		if p.IsDeleted.Value != nil {
			args = append(args, boolToInt(*p.IsDeleted.Value))
		} else {
			args = append(args, 0)
		}
	}
	if p.DeletedAt.Set {
		clauses = append(clauses, "deleted_at = ?")
		if p.DeletedAt.Value != nil {
			args = append(args, formatTime(*p.DeletedAt.Value))
		} else {
			args = append(args, nil)
		}
	}

	return clauses, args
}

// --- scan helpers ---

type assetScanner interface {
	Scan(dest ...any) error
}

func scanAssetFromRow(sc assetScanner) (*domain.Asset, error) {
	var a domain.Asset
	var isDeleted int
	var lastVerifiedAt, mtime, capturedAt sql.NullString
	var xmpLastReadAt, xmpLastWrittenAt, thumbnailAt, deletedAt sql.NullString
	var ingestedAt, updatedAt string
	var width, height, iso, bitDepth, rating sql.NullInt64
	var durationSecs, focalLengthMM, aperture, gpsLat, gpsLon sql.NullFloat64
	var colorSpace, cameraMake, cameraModel, lensModel, shutterSpeed sql.NullString
	var extMetadata, colorLabel, flag, note sql.NullString
	var xmpHash, thumbnailPath sql.NullString

	err := sc.Scan(
		&a.ID, &a.SourceID, &a.RelativePath, &a.FileStatus, &lastVerifiedAt,
		&a.Filename, &a.Extension, &a.MIMEType, &a.FileType, &a.SizeBytes, &mtime, &a.PartialHash,
		&width, &height, &durationSecs, &colorSpace, &bitDepth,
		&capturedAt, &cameraMake, &cameraModel, &lensModel, &focalLengthMM,
		&aperture, &shutterSpeed, &iso, &gpsLat, &gpsLon,
		&extMetadata, &rating, &colorLabel, &flag, &note,
		&xmpLastReadAt, &xmpLastWrittenAt, &xmpHash,
		&thumbnailPath, &thumbnailAt, &isDeleted, &deletedAt, &ingestedAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	a.LastVerifiedAt = parseNullTime(lastVerifiedAt)
	a.MTime = parseTime(mtime.String)
	a.Width = nullIntPtr(width)
	a.Height = nullIntPtr(height)
	a.DurationSecs = nullFloat64Ptr(durationSecs)
	a.ColorSpace = nullStringPtr(colorSpace)
	a.BitDepth = nullIntPtr(bitDepth)
	a.CapturedAt = parseNullTime(capturedAt)
	a.CameraMake = nullStringPtr(cameraMake)
	a.CameraModel = nullStringPtr(cameraModel)
	a.LensModel = nullStringPtr(lensModel)
	a.FocalLengthMM = nullFloat64Ptr(focalLengthMM)
	a.Aperture = nullFloat64Ptr(aperture)
	a.ShutterSpeed = nullStringPtr(shutterSpeed)
	a.ISO = nullIntPtr(iso)
	a.GPSLat = nullFloat64Ptr(gpsLat)
	a.GPSLon = nullFloat64Ptr(gpsLon)
	if extMetadata.Valid && extMetadata.String != "" {
		a.ExtendedMetadata = make(map[string]any)
		json.Unmarshal([]byte(extMetadata.String), &a.ExtendedMetadata)
	}
	if rating.Valid {
		v := int(rating.Int64)
		a.Rating = &v
	}
	if colorLabel.Valid {
		cl := domain.ColorLabel(colorLabel.String)
		a.ColorLabel = &cl
	}
	if flag.Valid {
		f := domain.Flag(flag.String)
		a.Flag = &f
	}
	a.Note = nullStringPtr(note)
	a.XMPLastReadAt = parseNullTime(xmpLastReadAt)
	a.XMPLastWrittenAt = parseNullTime(xmpLastWrittenAt)
	a.XMPHash = nullStringPtr(xmpHash)
	a.ThumbnailPath = nullStringPtr(thumbnailPath)
	a.ThumbnailAt = parseNullTime(thumbnailAt)
	a.IsDeleted = isDeleted != 0
	a.DeletedAt = parseNullTime(deletedAt)
	a.IngestedAt = parseTime(ingestedAt)
	a.UpdatedAt = parseTime(updatedAt)

	return &a, nil
}

func scanAsset(rows *sql.Rows) (*domain.Asset, error) {
	return scanAssetFromRow(rows)
}

func scanAssetRow(row *sql.Row) (*domain.Asset, error) {
	return scanAssetFromRow(row)
}

func marshalExtended(m map[string]any) (*string, error) {
	if m == nil {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	s := string(b)
	return &s, nil
}

func nilColorLabel(cl *domain.ColorLabel) *string {
	if cl == nil {
		return nil
	}
	s := string(*cl)
	return &s
}

func nilFlag(f *domain.Flag) *string {
	if f == nil {
		return nil
	}
	s := string(*f)
	return &s
}
