package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/akmadian/alexandria/internal/ast"
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

// --- Reader ---

func (r *AssetRepo) Get(ctx context.Context, id string) (*domain.Asset, error) {
	row := r.DB.QueryRowContext(ctx, "SELECT "+assetColumns+" FROM assets WHERE id = ?", id)
	a, err := scanAssetRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.NotFoundError{Resource: "asset", ID: id}
	}
	return a, err
}

func (r *AssetRepo) FindByHash(ctx context.Context, hash string, sizeBytes int64) (*domain.Asset, error) {
	row := r.DB.QueryRowContext(ctx,
		"SELECT "+assetColumns+" FROM assets WHERE partial_hash = ? AND size_bytes = ? AND is_deleted = 0",
		hash, sizeBytes)
	a, err := scanAssetRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (r *AssetRepo) FindBySourcePath(ctx context.Context, sourceID, relativePath string) (*domain.Asset, error) {
	row := r.DB.QueryRowContext(ctx,
		"SELECT "+assetColumns+" FROM assets WHERE source_id = ? AND relative_path = ?",
		sourceID, relativePath)
	a, err := scanAssetRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// DeleteByID hard-deletes an asset row. Unlike SoftDelete (a user judgment), this
// physically removes an identity. FK cascades clean its duplicates rows; the FTS
// delete trigger clears its index entry.
//
// Currently uncalled by ingest (D20 removed the delete-side merge that used it);
// retained as the primitive the review queue's user-triggered "confirm move"
// resolution needs (DEFERRED §5): confirming a move deletes the throwaway new
// identity and adopts its path onto the original.
// ponytail: leaves the deleted row's thumbnail file orphaned on disk (DEFERRED §4);
// a thumbnail GC sweeps it if orphans ever accumulate.
func (r *AssetRepo) DeleteByID(ctx context.Context, id string) error {
	_, err := r.DB.ExecContext(ctx, "DELETE FROM assets WHERE id = ?", id)
	return err
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

// --- Query-layer reader (impl/13) ---

func (r *AssetRepo) QueryAssets(ctx context.Context, query ast.Query, arrangement ast.Arrangement, page ast.Page) ([]catalog.AssetRow, int, error) {
	now := time.Now()
	start := time.Now()

	selectStmt, err := ast.CompileSelect(query, arrangement, page, now)
	if err != nil {
		return nil, 0, fmt.Errorf("query assets: compile select: %w", err)
	}
	countStmt, err := ast.CompileCount(query, now)
	if err != nil {
		return nil, 0, fmt.Errorf("query assets: compile count: %w", err)
	}

	log.Printf("query assets: compiled select (%d args), compiled count (%d args)",
		len(selectStmt.Args), len(countStmt.Args))

	rows, err := r.DB.QueryContext(ctx, selectStmt.SQL, selectStmt.Args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query assets: select: %w", err)
	}
	defer rows.Close()

	var result []catalog.AssetRow
	for rows.Next() {
		row, err := scanAssetRowSlim(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("query assets: scan: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int
	if err := r.DB.QueryRowContext(ctx, countStmt.SQL, countStmt.Args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("query assets: count: %w", err)
	}

	log.Printf("query assets: returned %d rows, total %d, took %s",
		len(result), total, time.Since(start).Round(time.Millisecond))

	return result, total, nil
}

func (r *AssetRepo) AssetIDSlice(ctx context.Context, query ast.Query, arrangement ast.Arrangement, fromIndex, toIndex int) ([]string, error) {
	stmt, err := ast.CompileIDSlice(query, arrangement, fromIndex, toIndex, time.Now())
	if err != nil {
		return nil, fmt.Errorf("asset id slice: compile: %w", err)
	}

	rows, err := r.DB.QueryContext(ctx, stmt.SQL, stmt.Args...)
	if err != nil {
		return nil, fmt.Errorf("asset id slice: query: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *AssetRepo) IndexOfAsset(ctx context.Context, query ast.Query, arrangement ast.Arrangement, id string) (*int, error) {
	stmt, err := ast.CompileIndexOf(query, arrangement, id, time.Now())
	if err != nil {
		return nil, fmt.Errorf("index of asset: compile: %w", err)
	}

	var position int
	err = r.DB.QueryRowContext(ctx, stmt.SQL, stmt.Args...).Scan(&position)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("index of asset: query: %w", err)
	}
	return &position, nil
}

func (r *AssetRepo) DistinctValues(ctx context.Context, field ast.Field) ([]string, error) {
	stmt, err := ast.CompileDistinctValues(field)
	if err != nil {
		return nil, fmt.Errorf("distinct values: compile: %w", err)
	}

	rows, err := r.DB.QueryContext(ctx, stmt.SQL, stmt.Args...)
	if err != nil {
		return nil, fmt.Errorf("distinct values: query: %w", err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r *AssetRepo) ReadTriageStates(ctx context.Context, ids []string) ([]catalog.TriageState, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := "SELECT id, rating, color_label, flag, note FROM assets WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read triage states: %w", err)
	}
	defer rows.Close()

	var states []catalog.TriageState
	for rows.Next() {
		var state catalog.TriageState
		var rating sql.NullInt64
		var colorLabel, flag, note sql.NullString
		if err := rows.Scan(&state.ID, &rating, &colorLabel, &flag, &note); err != nil {
			return nil, err
		}
		if rating.Valid {
			v := int(rating.Int64)
			state.Rating = &v
		}
		if colorLabel.Valid {
			cl := domain.ColorLabel(colorLabel.String)
			state.ColorLabel = &cl
		}
		if flag.Valid {
			f := domain.Flag(flag.String)
			state.Flag = &f
		}
		state.Note = nullStringPtr(note)
		states = append(states, state)
	}
	return states, rows.Err()
}

// ApplyTriagePatchByQuery applies a triage patch via a single UPDATE … WHERE
// <compiled query>. Returns the affected IDs for undo's before-image capture.
func (r *AssetRepo) ApplyTriagePatchByQuery(ctx context.Context, query ast.Query, exceptIDs []string, p catalog.TriagePatch) ([]string, error) {
	clauses, args := buildTriageSQL(p)
	if len(clauses) == 0 {
		return nil, nil
	}
	now := formatTime(time.Now().UTC())
	clauses = append(clauses, "judgment_modified_at = ?", "updated_at = ?")
	args = append(args, now, now)

	whereStmt, err := ast.CompileWhere(query, exceptIDs, time.Now())
	if err != nil {
		return nil, fmt.Errorf("apply triage by query: compile: %w", err)
	}
	args = append(args, whereStmt.Args...)

	updateSQL := "UPDATE assets SET " + strings.Join(clauses, ", ") + " WHERE " + whereStmt.SQL +
		" RETURNING id"

	rows, err := r.DB.QueryContext(ctx, updateSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("apply triage by query: update: %w", err)
	}
	defer rows.Close()

	var affectedIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		affectedIDs = append(affectedIDs, id)
	}

	log.Printf("apply triage by query: affected %d assets", len(affectedIDs))
	return affectedIDs, rows.Err()
}

func scanAssetRowSlim(rows *sql.Rows) (catalog.AssetRow, error) {
	var row catalog.AssetRow
	var rating sql.NullInt64
	var colorLabel, flag sql.NullString
	var width, height sql.NullInt64
	var capturedAt, thumbnailAt, ingestedAt sql.NullString
	var sizeBytes int64

	if err := rows.Scan(
		&row.ID, &row.SourceID, &row.Filename, &row.FileType, &row.FileStatus,
		&rating, &colorLabel, &flag,
		&width, &height, &capturedAt, &ingestedAt,
		&thumbnailAt, &row.RelativePath, &sizeBytes,
	); err != nil {
		return catalog.AssetRow{}, err
	}

	row.SizeBytes = sizeBytes
	if rating.Valid {
		v := int(rating.Int64)
		row.Rating = &v
	}
	if colorLabel.Valid {
		cl := domain.ColorLabel(colorLabel.String)
		row.ColorLabel = &cl
	}
	if flag.Valid {
		f := domain.Flag(flag.String)
		row.Flag = &f
	}
	if width.Valid {
		v := int(width.Int64)
		row.Width = &v
	}
	if height.Valid {
		v := int(height.Int64)
		row.Height = &v
	}
	row.CapturedAt = parseNullTime(capturedAt)
	row.ThumbnailAt = parseNullTime(thumbnailAt)
	if ingestedAt.Valid {
		row.IngestedAt = parseTime(ingestedAt.String)
	}

	return row, nil
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
func (r *AssetRepo) ApplyFilePatch(ctx context.Context, id string, p *catalog.FilePatch) error {
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
		if err := json.Unmarshal([]byte(extMetadata.String), &a.ExtendedMetadata); err != nil {
			return nil, fmt.Errorf("unmarshal extended metadata: %w", err)
		}
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
