package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

// ImportRepo owns import_sessions (one row per pipeline run) and import_errors
// (the DLQ). Sessions are created up front (the errors FK references them), then
// finalized with counts + per-extension skip tallies when the run drains.
type ImportRepo struct {
	DB DBTX
}

// Start inserts a fresh session row and returns its id. It commits immediately
// (autocommit) so the batched-write transactions that follow can reference it
// via the import_errors foreign key.
func (r *ImportRepo) Start(ctx context.Context, volumeID, kind string) (string, error) {
	sessionID := domain.NewID()
	_, err := r.DB.ExecContext(ctx,
		"INSERT INTO import_sessions (id, volume_id, kind, started_at) VALUES (?, ?, ?, ?)",
		sessionID, volumeID, kind, formatTime(time.Now().UTC()))
	if err != nil {
		return "", err
	}
	return sessionID, nil
}

// UpdateCounts refreshes the running tallies mid-flight (called per batch so a
// live viewer sees progress). Cheap single-row update.
func (r *ImportRepo) UpdateCounts(ctx context.Context, sessionID string, session *domain.ImportSession) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE import_sessions SET added=?, updated=?, moved=?, skipped=?, dups=?, errors=? WHERE id=?`,
		session.Added, session.Updated, session.Moved, session.Skipped, session.Dups, session.Errors, sessionID)
	return err
}

// Finish stamps finished_at, writes the final counts, and persists the
// per-extension skip tallies as JSON.
func (r *ImportRepo) Finish(ctx context.Context, sessionID string, session *domain.ImportSession) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE import_sessions SET finished_at=?, added=?, updated=?, moved=?, skipped=?, dups=?, errors=?,
			skipped_unknown_json=?, skipped_ignored_json=? WHERE id=?`,
		formatTime(time.Now().UTC()), session.Added, session.Updated, session.Moved, session.Skipped, session.Dups, session.Errors,
		tallyJSON(session.SkippedUnknown), tallyJSON(session.SkippedIgnored), sessionID)
	return err
}

// LogError records one DLQ row. It upserts on (session, path, stage): a repeat
// of the same failure in the same session bumps the attempt count rather than
// piling up rows. (Across runs the session_id differs, so re-drives are distinct
// rows by design — the session IS the attempt boundary.)
func (r *ImportRepo) LogError(ctx context.Context, sessionID, path, stage, reasonCode, message string) error {
	result, err := r.DB.ExecContext(ctx,
		`UPDATE import_errors SET attempts=attempts+1, reason_code=?, message=?, occurred_at=?
			WHERE session_id=? AND path=? AND stage=?`,
		reasonCode, message, formatTime(time.Now().UTC()), sessionID, path, stage)
	if err != nil {
		return err
	}
	if updated, _ := result.RowsAffected(); updated > 0 {
		return nil
	}
	_, err = r.DB.ExecContext(ctx,
		`INSERT INTO import_errors (id, session_id, path, stage, reason_code, message, attempts, occurred_at)
			VALUES (?, ?, ?, ?, ?, ?, 1, ?)`,
		domain.NewID(), sessionID, path, stage, reasonCode, message, formatTime(time.Now().UTC()))
	return err
}

// ListErrors returns the DLQ rows for a session (newest first) — the dev harness
// dumps these.
func (r *ImportRepo) ListErrors(ctx context.Context, sessionID string) ([]*domain.ImportError, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, session_id, path, stage, reason_code, message, attempts, occurred_at
			FROM import_errors WHERE session_id = ? ORDER BY occurred_at DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var importErrors []*domain.ImportError
	for rows.Next() {
		var importError domain.ImportError
		var occurredAt string
		if err := rows.Scan(&importError.ID, &importError.SessionID, &importError.Path, &importError.Stage,
			&importError.ReasonCode, &importError.Message, &importError.Attempts, &occurredAt); err != nil {
			return nil, err
		}
		importError.OccurredAt = parseTime(occurredAt)
		importErrors = append(importErrors, &importError)
	}
	return importErrors, rows.Err()
}

// ListSessions returns recent sessions (newest first).
func (r *ImportRepo) ListSessions(ctx context.Context, limit int) ([]*domain.ImportSession, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, volume_id, kind, started_at, finished_at, added, updated, moved, skipped, dups, errors,
			skipped_unknown_json, skipped_ignored_json
			FROM import_sessions ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*domain.ImportSession
	for rows.Next() {
		var session domain.ImportSession
		var startedAt string
		var finishedAt, unknownJSON, ignoredJSON sql.NullString
		if err := rows.Scan(&session.ID, &session.VolumeID, &session.Kind, &startedAt, &finishedAt,
			&session.Added, &session.Updated, &session.Moved, &session.Skipped, &session.Dups, &session.Errors,
			&unknownJSON, &ignoredJSON); err != nil {
			return nil, err
		}
		session.StartedAt = parseTime(startedAt)
		session.FinishedAt = parseNullTime(finishedAt)
		session.SkippedUnknown = parseTally(unknownJSON)
		session.SkippedIgnored = parseTally(ignoredJSON)
		sessions = append(sessions, &session)
	}
	return sessions, rows.Err()
}

func tallyJSON(tallies map[string]int) *string {
	if len(tallies) == 0 {
		return nil
	}
	encoded, err := json.Marshal(tallies)
	if err != nil {
		return nil
	}
	asString := string(encoded)
	return &asString
}

func parseTally(value sql.NullString) map[string]int {
	if !value.Valid || value.String == "" {
		return nil
	}
	tallies := map[string]int{}
	if err := json.Unmarshal([]byte(value.String), &tallies); err != nil {
		return nil
	}
	return tallies
}
