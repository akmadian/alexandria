package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

// EnrichmentRepo owns the enrichment engine's catalog surface: the
// missing-artifact scan queries and the enrichment_errors DLQ (D28). The scan
// SQL here is deliberately engine-internal and NOT routed through internal/ast
// — the AST is the user-predicate contract between frontend and catalog;
// engine plumbing (DLQ anti-joins, artifact-column probes) is a different lane
// and must never leak into that grammar (decided with Ari, enrichment round
// 2026-07-13).
type EnrichmentRepo struct {
	DB DBTX
}

// derivedArtifactColumns is the closed allowlist of enrichment-owned derived
// columns: every column here is nulled by ClearDerived on reimport (the D28
// staleness path), and a registry row's artifact marker MUST be one of them. It
// stands between registry data and SQL — an unknown column is an error, never
// interpolated. The scan keys only on the markers a registry row names (one per
// kind); a kind that writes several columns (clipping → highlights + shadows)
// lists them all here so they clear together, while marking just one.
var derivedArtifactColumns = map[string]bool{
	"thumbnail_at":        true,
	"sharpness":           true,
	"clipping_highlights": true,
	"clipping_shadows":    true,
	"phash":               true,
}

// IsDerivedArtifactColumn reports whether the enrichment scan may key on the
// named assets column. The job-kind registry validates its rows against this
// at boot (C10: fail first boot, never a user session).
func IsDerivedArtifactColumn(name string) bool { return derivedArtifactColumns[name] }

// sortedDerivedArtifactColumns returns the allowlist in deterministic order,
// for SQL builders (AssetRepo.ClearDerived).
func sortedDerivedArtifactColumns() []string {
	columns := make([]string, 0, len(derivedArtifactColumns))
	for column := range derivedArtifactColumns {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

// MissingArtifactScan parameterizes one cold-backlog scan pass for one job kind.
type MissingArtifactScan struct {
	Kind                string   // registry key; scopes the DLQ exhaustion check
	ArtifactColumn      string   // derived column whose NULL marks the artifact missing
	PrerequisiteColumns []string // derived columns that must be present before this kind may run
	Extensions          []string // applicability, precomputed from the assettype registry
	MaxAttempts         int      // DLQ rows at or beyond this are exhausted — skipped
	Limit               int      // page size; a full page means "scan again when drained"
}

// MissingArtifact is one scan hit: an asset whose artifact is missing and
// eligible. Size and type feed the weight and timeout policies at dispatch;
// IngestedAt is the recency half of the queue's composite priority key.
type MissingArtifact struct {
	AssetID    string
	SizeBytes  int64
	FileType   domain.FileType
	IngestedAt time.Time
}

// ListMissingArtifacts returns up to Limit assets missing the scan's artifact,
// newest ingest first (D28: the cold backlog orders by import recency). Only
// online, non-deleted assets qualify — a missing file has no bytes to enrich.
// Attempt-exhausted DLQ rows are skipped so a permanently failing asset never
// spins.
func (r *EnrichmentRepo) ListMissingArtifacts(ctx context.Context, scan *MissingArtifactScan) ([]MissingArtifact, error) {
	if err := validateArtifactColumns(scan.ArtifactColumn, scan.PrerequisiteColumns); err != nil {
		return nil, err
	}
	if len(scan.Extensions) == 0 {
		return nil, fmt.Errorf("enrichment scan for kind %q: empty extension set", scan.Kind)
	}

	var query strings.Builder
	args := make([]any, 0, len(scan.Extensions)+4)
	query.WriteString("SELECT id, size_bytes, file_type, ingested_at FROM assets WHERE ")
	query.WriteString(scan.ArtifactColumn)
	query.WriteString(" IS NULL AND is_deleted = 0 AND file_status = ? AND extension IN (")
	args = append(args, string(domain.FileStatusOnline))
	query.WriteString(placeholders(len(scan.Extensions)))
	query.WriteString(")")
	for _, extension := range scan.Extensions {
		args = append(args, extension)
	}
	for _, prerequisite := range scan.PrerequisiteColumns {
		query.WriteString(" AND ")
		query.WriteString(prerequisite)
		query.WriteString(" IS NOT NULL")
	}
	query.WriteString(" AND id NOT IN (SELECT asset_id FROM enrichment_errors WHERE kind = ? AND attempts >= ?)")
	args = append(args, scan.Kind, scan.MaxAttempts)
	query.WriteString(" ORDER BY ingested_at DESC LIMIT ?")
	args = append(args, scan.Limit)

	rows, err := r.DB.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var missing []MissingArtifact
	for rows.Next() {
		var artifact MissingArtifact
		var ingestedAt string
		if err := rows.Scan(&artifact.AssetID, &artifact.SizeBytes, &artifact.FileType, &ingestedAt); err != nil {
			return nil, err
		}
		artifact.IngestedAt = parseTime(ingestedAt)
		missing = append(missing, artifact)
	}
	return missing, rows.Err()
}

// EligibilityProbe parameterizes one dispatch-time recheck.
type EligibilityProbe struct {
	AssetID             string
	Kind                string   // scopes the DLQ exhaustion check
	ArtifactColumn      string   // must still be NULL (missing)
	PrerequisiteColumns []string // must all be present
	MaxAttempts         int      // an exhausted DLQ row is terminal for hints too
}

// MissingAndEligible is the dispatch-time recheck: true iff the asset is still
// online, not deleted, its artifact is still missing, every prerequisite
// artifact is present, and the (asset, kind) DLQ row — if any — has attempts
// left. Hints arrive unfiltered and scans go stale, so this one cheap probe is
// what keeps a confused queue degrading to wasted ordering, never to
// duplicate, premature, or exhaustion-defying work.
func (r *EnrichmentRepo) MissingAndEligible(ctx context.Context, probe *EligibilityProbe) (bool, error) {
	if err := validateArtifactColumns(probe.ArtifactColumn, probe.PrerequisiteColumns); err != nil {
		return false, err
	}
	var query strings.Builder
	query.WriteString("SELECT (")
	query.WriteString(probe.ArtifactColumn)
	query.WriteString(" IS NULL)")
	for _, prerequisite := range probe.PrerequisiteColumns {
		query.WriteString(" AND (")
		query.WriteString(prerequisite)
		query.WriteString(" IS NOT NULL)")
	}
	query.WriteString(" AND id NOT IN (SELECT asset_id FROM enrichment_errors WHERE kind = ? AND attempts >= ?)")
	query.WriteString(" FROM assets WHERE id = ? AND is_deleted = 0 AND file_status = ?")

	var eligible bool
	err := r.DB.QueryRowContext(ctx, query.String(),
		probe.Kind, probe.MaxAttempts, probe.AssetID, string(domain.FileStatusOnline)).Scan(&eligible)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil // asset gone, offline, or deleted — not eligible, not an error
	}
	if err != nil {
		return false, err
	}
	return eligible, nil
}

// LogFailure upserts the (asset, kind) DLQ row, bumping attempts on a repeat.
// Attempts accumulate across scans by design — the scan is the retry, and this
// counter is what eventually exhausts it.
func (r *EnrichmentRepo) LogFailure(ctx context.Context, assetID, kind, reasonCode, message string) error {
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO enrichment_errors (asset_id, kind, reason_code, message, attempts, last_attempt_at)
			VALUES (?, ?, ?, ?, 1, ?)
			ON CONFLICT(asset_id, kind) DO UPDATE SET
				attempts = attempts + 1, reason_code = excluded.reason_code,
				message = excluded.message, last_attempt_at = excluded.last_attempt_at`,
		assetID, kind, reasonCode, message, formatTime(time.Now().UTC()))
	return err
}

// ClearFailure removes the (asset, kind) DLQ row after a successful production
// — the failed state must not outlive the artifact it described. A no-op when
// no row exists.
func (r *EnrichmentRepo) ClearFailure(ctx context.Context, assetID, kind string) error {
	_, err := r.DB.ExecContext(ctx,
		"DELETE FROM enrichment_errors WHERE asset_id = ? AND kind = ?", assetID, kind)
	return err
}

// ClearFailures removes ALL of an asset's DLQ rows — the reimport half of the
// D28 staleness transition: exhaustion described the OLD bytes, and new bytes
// must get fresh attempts (without this, a repaired file stays terminally
// "failed"). Runs in the same transaction as AssetRepo.ClearDerived.
func (r *EnrichmentRepo) ClearFailures(ctx context.Context, assetID string) error {
	_, err := r.DB.ExecContext(ctx,
		"DELETE FROM enrichment_errors WHERE asset_id = ?", assetID)
	return err
}

// ListFailures returns an asset's DLQ rows (task 21 reads these for the
// per-asset failed state; the dev harness dumps them).
func (r *EnrichmentRepo) ListFailures(ctx context.Context, assetID string) ([]*domain.EnrichmentError, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT asset_id, kind, reason_code, message, attempts, last_attempt_at
			FROM enrichment_errors WHERE asset_id = ? ORDER BY kind`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var failures []*domain.EnrichmentError
	for rows.Next() {
		var failure domain.EnrichmentError
		var lastAttemptAt string
		if err := rows.Scan(&failure.AssetID, &failure.Kind, &failure.ReasonCode,
			&failure.Message, &failure.Attempts, &lastAttemptAt); err != nil {
			return nil, err
		}
		failure.LastAttemptAt = parseTime(lastAttemptAt)
		failures = append(failures, &failure)
	}
	return failures, rows.Err()
}

// ExhaustedKinds returns, per asset, the kinds whose DLQ row is attempt-exhausted
// (attempts >= maxAttempts) — the terminally-failed state the grid renders
// distinctly (D25/task 21). One query over a page of ids; an asset with no
// exhausted failure is absent (sparse, like the in-flight tracker), so a healthy
// page allocates nothing.
func (r *EnrichmentRepo) ExhaustedKinds(ctx context.Context, assetIDs []string, maxAttempts int) (map[string][]string, error) {
	if len(assetIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(assetIDs))
	args := make([]any, 0, len(assetIDs)+1)
	for index, id := range assetIDs {
		placeholders[index] = "?"
		args = append(args, id)
	}
	args = append(args, maxAttempts)
	rows, err := r.DB.QueryContext(ctx,
		"SELECT asset_id, kind FROM enrichment_errors WHERE asset_id IN ("+
			strings.Join(placeholders, ",")+") AND attempts >= ? ORDER BY asset_id, kind", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result map[string][]string
	for rows.Next() {
		var assetID, kind string
		if err := rows.Scan(&assetID, &kind); err != nil {
			return nil, err
		}
		if result == nil {
			result = make(map[string][]string)
		}
		result[assetID] = append(result[assetID], kind)
	}
	return result, rows.Err()
}

// FailureCounts rolls the DLQ up by (kind, reason_code) for the debug snapshot
// (task 22): Count rows per bucket, Exhausted of them attempt-exhausted. One
// grouped query; maxAttempts is passed in (the threshold lives in the enrichment
// package, which this layer must not import). An empty DLQ yields no rows.
func (r *EnrichmentRepo) FailureCounts(ctx context.Context, maxAttempts int) ([]domain.EnrichmentFailureCount, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT kind, reason_code, COUNT(*), COALESCE(SUM(attempts >= ?), 0)
			FROM enrichment_errors GROUP BY kind, reason_code ORDER BY kind, reason_code`, maxAttempts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []domain.EnrichmentFailureCount
	for rows.Next() {
		var count domain.EnrichmentFailureCount
		if err := rows.Scan(&count.Kind, &count.Reason, &count.Count, &count.Exhausted); err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}
	return counts, rows.Err()
}

func validateArtifactColumns(artifactColumn string, prerequisiteColumns []string) error {
	if !derivedArtifactColumns[artifactColumn] {
		return fmt.Errorf("enrichment: %q is not a derived artifact column", artifactColumn)
	}
	for _, prerequisite := range prerequisiteColumns {
		if !derivedArtifactColumns[prerequisite] {
			return fmt.Errorf("enrichment: prerequisite %q is not a derived artifact column", prerequisite)
		}
	}
	return nil
}

// placeholders renders n comma-joined SQL placeholders ("?, ?, ?").
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?, ", n-1) + "?"
}
