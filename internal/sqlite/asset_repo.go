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
	DB DBTX
}

const assetColumns = `id, source_id, relative_path, file_status, last_verified_at,
	filename, extension, mime_type, file_type, size_bytes, mtime, partial_hash,
	width, height, duration_secs, color_space, bit_depth,
	captured_at, camera_make, camera_model, lens_model, focal_length_mm,
	aperture, shutter_speed, iso, gps_lat, gps_lon, creator, copyright,
	extended_metadata, rating, color_label, flag, note,
	xmp_last_read_at, xmp_last_written_at, xmp_hash,
	thumbnail_at, is_deleted, deleted_at, ingested_at, updated_at,
	title, caption, judgment_modified_at`

// sortColumns whitelists the logical sort names the API exposes, mapping each to
// a real column. Anything not in the map is rejected — sort fields are never
// interpolated raw (that was an injection hole).
var sortColumns = map[string]string{
	"captured": "captured_at",
	"added":    "ingested_at",
	"rating":   "rating",
	"filename": "filename",
	"size":     "size_bytes",
}

// --- Reader ---

func (r *AssetRepo) Get(ctx context.Context, id string) (*domain.Asset, error) {
	row := r.DB.QueryRowContext(ctx, "SELECT "+assetColumns+" FROM assets WHERE id = ?", id)
	a, err := scanAssetRow(row)
	if err == sql.ErrNoRows {
		return nil, &domain.NotFoundError{Resource: "asset", ID: id}
	}
	return a, err
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

// ListKnownFiles returns relative_path → (mtime, size, hash) for every ONLINE
// asset in the source, in one query. The importer loads this once per scan to
// skip unchanged files without a per-file lookup.
//
// Only online assets are included on purpose: a missing/offline asset whose file
// reappears unchanged must NOT be skipped — it has to flow through the matrix to
// be restored (relink/reimport → online). Skipping it would leave it missing
// forever.
func (r *AssetRepo) ListKnownFiles(ctx context.Context, sourceID string) (map[string]domain.FileStat, error) {
	rows, err := r.DB.QueryContext(ctx,
		"SELECT relative_path, mtime, size_bytes, partial_hash FROM assets WHERE source_id = ? AND is_deleted = 0 AND file_status = 'online'",
		sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	known := make(map[string]domain.FileStat)
	for rows.Next() {
		var relPath, mtime string
		var size int64
		var hash sql.NullString
		if err := rows.Scan(&relPath, &mtime, &size, &hash); err != nil {
			return nil, err
		}
		known[relPath] = domain.FileStat{
			MTime:       parseTime(mtime),
			SizeBytes:   size,
			PartialHash: hash.String,
		}
	}
	return known, rows.Err()
}

// ListPathsStatus is the slim reconciliation projection: id, path, status for
// every live asset in the source — no 40-column scan per row.
func (r *AssetRepo) ListPathsStatus(ctx context.Context, sourceID string) ([]catalog.PathStatus, error) {
	rows, err := r.DB.QueryContext(ctx,
		"SELECT id, relative_path, file_status FROM assets WHERE source_id = ? AND is_deleted = 0",
		sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []catalog.PathStatus
	for rows.Next() {
		var p catalog.PathStatus
		if err := rows.Scan(&p.ID, &p.RelativePath, &p.FileStatus); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *AssetRepo) List(ctx context.Context, filter catalog.AssetFilter) ([]*domain.Asset, error) {
	where, args := buildFilterSQL(filter)
	q := "SELECT " + assetColumns + " FROM assets"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}

	sortField := "ingested_at"
	if filter.SortField != "" {
		col, ok := sortColumns[filter.SortField]
		if !ok {
			return nil, fmt.Errorf("invalid sort field %q", filter.SortField)
		}
		sortField = col
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

// --- Observation writer (ingest / watcher / reconciler) ---

func (r *AssetRepo) Create(ctx context.Context, a *domain.Asset) error {
	// Minting is observation-only: judgment fields must be zero. Defense in depth
	// — the interface split already keeps ingest away from the judgment writer.
	if a.Rating != nil || a.ColorLabel != nil || a.Flag != nil || a.Note != nil || a.IsDeleted || a.JudgmentModifiedAt != nil {
		return fmt.Errorf("Create: minting is observation-only, judgment fields must be zero (asset %s)", a.ID)
	}
	extJSON, err := marshalExtended(a.ExtendedMetadata)
	if err != nil {
		return err
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO assets
		(`+assetColumns+`) VALUES
		(?,?,?,?,?, ?,?,?,?,?,?,?, ?,?,?,?,?, ?,?,?,?,?, ?,?,?,?,?,?,?, ?,?,?,?,?, ?,?,?, ?,?,?,?,?, ?,?,?)`,
		a.ID, a.SourceID, a.RelativePath, a.FileStatus, formatTimePtr(a.LastVerifiedAt),
		a.Filename, a.Extension, a.MIMEType, a.FileType, a.SizeBytes, formatTime(a.MTime), a.PartialHash,
		a.Width, a.Height, a.DurationSecs, a.ColorSpace, a.BitDepth,
		formatTimePtr(a.CapturedAt), a.CameraMake, a.CameraModel, a.LensModel, a.FocalLengthMM,
		a.Aperture, a.ShutterSpeed, a.ISO, a.GPSLat, a.GPSLon,
		a.Creator, a.Copyright,
		extJSON, a.Rating, nilColorLabel(a.ColorLabel), nilFlag(a.Flag), a.Note,
		formatTimePtr(a.XMPLastReadAt), formatTimePtr(a.XMPLastWrittenAt), a.XMPHash,
		formatTimePtr(a.ThumbnailAt),
		boolToInt(a.IsDeleted), formatTimePtr(a.DeletedAt),
		formatTime(a.IngestedAt), formatTime(a.UpdatedAt),
		a.Title, a.Caption, formatTimePtr(a.JudgmentModifiedAt))
	return err
}

// ApplyFilePatch writes observation columns only (file facts always; metadata
// overlaid non-nil). It cannot touch rating/flag/note/is_deleted/xmp_*/
// thumbnail_at/judgment_modified_at — the reimport-clobber bug is structurally
// impossible here.
func (r *AssetRepo) ApplyFilePatch(ctx context.Context, id string, p catalog.FilePatch) error {
	clauses := []string{
		"filename = ?", "extension = ?", "mime_type = ?", "file_type = ?",
		"size_bytes = ?", "mtime = ?", "partial_hash = ?", "file_status = ?",
	}
	args := []any{
		p.Filename, p.Extension, p.MIMEType, p.FileType,
		p.SizeBytes, formatTime(p.MTime), p.PartialHash, p.FileStatus,
	}

	set := func(col string, ok bool, val any) {
		if ok {
			clauses = append(clauses, col+" = ?")
			args = append(args, val)
		}
	}
	set("width", p.Width != nil, ptrInt(p.Width))
	set("height", p.Height != nil, ptrInt(p.Height))
	set("duration_secs", p.DurationSecs != nil, ptrFloat(p.DurationSecs))
	set("captured_at", p.CapturedAt != nil, formatTimePtr(p.CapturedAt))
	set("camera_make", p.CameraMake != nil, ptrStr(p.CameraMake))
	set("camera_model", p.CameraModel != nil, ptrStr(p.CameraModel))
	set("lens_model", p.LensModel != nil, ptrStr(p.LensModel))
	set("focal_length_mm", p.FocalLengthMM != nil, ptrFloat(p.FocalLengthMM))
	set("aperture", p.Aperture != nil, ptrFloat(p.Aperture))
	set("shutter_speed", p.ShutterSpeed != nil, ptrStr(p.ShutterSpeed))
	set("iso", p.ISO != nil, ptrInt(p.ISO))
	set("gps_lat", p.GPSLat != nil, ptrFloat(p.GPSLat))
	set("gps_lon", p.GPSLon != nil, ptrFloat(p.GPSLon))
	set("color_space", p.ColorSpace != nil, ptrStr(p.ColorSpace))
	set("bit_depth", p.BitDepth != nil, ptrInt(p.BitDepth))
	set("creator", p.Creator != nil, ptrStr(p.Creator))
	set("copyright", p.Copyright != nil, ptrStr(p.Copyright))
	set("title", p.Title != nil, ptrStr(p.Title))
	set("caption", p.Caption != nil, ptrStr(p.Caption))
	if p.Extended != nil {
		extJSON, err := marshalExtended(p.Extended)
		if err != nil {
			return err
		}
		clauses = append(clauses, "extended_metadata = ?")
		args = append(args, extJSON)
	}

	clauses = append(clauses, "updated_at = ?")
	args = append(args, formatTime(time.Now().UTC()), id)

	q := "UPDATE assets SET " + strings.Join(clauses, ", ") + " WHERE id = ?"
	res, err := r.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "asset", id)
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

func (r *AssetRepo) SetFileStatus(ctx context.Context, assetID string, status domain.FileStatus) error {
	res, err := r.DB.ExecContext(ctx,
		"UPDATE assets SET file_status = ?, updated_at = ? WHERE id = ?",
		status, formatTime(time.Now().UTC()), assetID)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "asset", assetID)
}

// MarkConnectivityBySource flips every asset of a source between online and
// offline (a whole-volume mount/unmount). Files are presumed intact when a
// source goes offline — this never marks them missing.
func (r *AssetRepo) MarkConnectivityBySource(ctx context.Context, sourceID string, online bool) error {
	from, to := domain.FileStatusOnline, domain.FileStatusOffline
	if online {
		from, to = domain.FileStatusOffline, domain.FileStatusOnline
	}
	_, err := r.DB.ExecContext(ctx,
		"UPDATE assets SET file_status = ? WHERE source_id = ? AND file_status = ?",
		to, sourceID, from)
	return err
}

// --- Judgment writer (user-action service) ---

// ApplyTriagePatch is the ONE place judgment_modified_at is bumped. An empty
// patch is a no-op (no timestamp churn).
func (r *AssetRepo) ApplyTriagePatch(ctx context.Context, ids []string, p catalog.TriagePatch) error {
	if len(ids) == 0 {
		return nil
	}
	clauses, args := buildTriageSQL(p)
	if len(clauses) == 0 {
		return nil
	}
	now := formatTime(time.Now().UTC())
	clauses = append(clauses, "judgment_modified_at = ?", "updated_at = ?")
	args = append(args, now, now)
	return r.execUpdateIn(ctx, clauses, args, ids)
}

func (r *AssetRepo) SoftDelete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	now := formatTime(time.Now().UTC())
	// is_deleted is a judgment (removal is a user decision) → bump judgment_modified_at.
	clauses := []string{"is_deleted = 1", "deleted_at = ?", "judgment_modified_at = ?", "updated_at = ?"}
	args := []any{now, now, now}
	return r.execUpdateIn(ctx, clauses, args, ids)
}

// --- Sync writer (XMP) ---

// ApplyXMPInbound applies inbound judgment VALUES and advances the xmp read
// cursor, but deliberately does NOT bump judgment_modified_at — otherwise every
// inbound sync would look like a user edit and drive an outbound write, an
// endless oscillation (D15 loop level 2).
func (r *AssetRepo) ApplyXMPInbound(ctx context.Context, id string, p catalog.TriagePatch, readAt time.Time, xmpHash string) error {
	clauses, args := buildTriageSQL(p)
	clauses = append(clauses, "xmp_last_read_at = ?", "xmp_hash = ?", "updated_at = ?")
	args = append(args, formatTime(readAt), xmpHash, formatTime(time.Now().UTC()), id)
	res, err := r.DB.ExecContext(ctx, "UPDATE assets SET "+strings.Join(clauses, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "asset", id)
}

func (r *AssetRepo) RecordXMPWritten(ctx context.Context, id string, writtenAt time.Time, xmpHash string) error {
	res, err := r.DB.ExecContext(ctx,
		"UPDATE assets SET xmp_last_written_at = ?, xmp_hash = ?, updated_at = ? WHERE id = ?",
		formatTime(writtenAt), xmpHash, formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "asset", id)
}

// --- Derived writer (jobs / thumbnail stage) ---

func (r *AssetRepo) SetThumbnailAt(ctx context.Context, id string, t time.Time) error {
	res, err := r.DB.ExecContext(ctx,
		"UPDATE assets SET thumbnail_at = ?, updated_at = ? WHERE id = ?",
		formatTime(t), formatTime(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "asset", id)
}

// execUpdateIn runs "UPDATE assets SET <clauses> WHERE id IN (<ids>)". args holds
// the SET args; the id placeholders are appended here.
func (r *AssetRepo) execUpdateIn(ctx context.Context, clauses []string, args []any, ids []string) error {
	ph := make([]string, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args = append(args, id)
	}
	q := "UPDATE assets SET " + strings.Join(clauses, ", ") + " WHERE id IN (" + strings.Join(ph, ",") + ")"
	_, err := r.DB.ExecContext(ctx, q, args...)
	return err
}

// --- SQL builders ---

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

// buildTriageSQL returns the sparse SET clauses for the four judgment columns.
// Shared by the judgment writer and the XMP sync writer (which appends its own
// cursor columns and skips the judgment_modified_at bump).
func buildTriageSQL(p catalog.TriagePatch) ([]string, []any) {
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
	var xmpHash sql.NullString
	var creator, copyright, title, caption sql.NullString
	var judgmentModifiedAt sql.NullString

	err := sc.Scan(
		&a.ID, &a.SourceID, &a.RelativePath, &a.FileStatus, &lastVerifiedAt,
		&a.Filename, &a.Extension, &a.MIMEType, &a.FileType, &a.SizeBytes, &mtime, &a.PartialHash,
		&width, &height, &durationSecs, &colorSpace, &bitDepth,
		&capturedAt, &cameraMake, &cameraModel, &lensModel, &focalLengthMM,
		&aperture, &shutterSpeed, &iso, &gpsLat, &gpsLon, &creator, &copyright,
		&extMetadata, &rating, &colorLabel, &flag, &note,
		&xmpLastReadAt, &xmpLastWrittenAt, &xmpHash,
		&thumbnailAt, &isDeleted, &deletedAt, &ingestedAt, &updatedAt,
		&title, &caption, &judgmentModifiedAt)
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
	a.Creator = nullStringPtr(creator)
	a.Copyright = nullStringPtr(copyright)
	a.Title = nullStringPtr(title)
	a.Caption = nullStringPtr(caption)
	a.JudgmentModifiedAt = parseNullTime(judgmentModifiedAt)
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

// ptr* deref helpers for the sparse ApplyFilePatch args (guarded non-nil at the
// call site; they exist so the arg is a value, not a typed pointer, for clarity).
func ptrInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
func ptrFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
func ptrStr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
