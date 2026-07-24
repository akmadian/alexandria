package sqlite

import (
	"context"
	"database/sql"

	"github.com/akmadian/alexandria/internal/domain"
)

// RebuildPathKeys recomputes the derived path_key column (NFC of relative_path)
// for every asset and sidecar, in one transaction. path_key is derived state —
// the "compare keys, open bytes" identity form (D24) — so it carries a
// registered rebuild path like RebuildFTS: run it to repair keys after an
// out-of-band edit to relative_path, or to backfill after the column's
// introduction.
//
// The normalization is Unicode NFC (domain.PathKey), which SQLite cannot do in
// SQL, so the values are recomputed in Go: read the raw bytes, write the key.
func RebuildPathKeys(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := rebuildTablePathKeys(ctx, tx, "assets"); err != nil {
		return err
	}
	if err := rebuildTablePathKeys(ctx, tx, "sidecar_files"); err != nil {
		return err
	}
	return tx.Commit()
}

// rebuildTablePathKeys recomputes path_key from relative_path for one table.
func rebuildTablePathKeys(ctx context.Context, tx *sql.Tx, table string) error {
	//nolint:gosec // table is an internal constant ("assets"/"sidecar_files"), never user input
	rows, err := tx.QueryContext(ctx, "SELECT id, relative_path FROM "+table)
	if err != nil {
		return err
	}
	type entry struct{ id, relativePath string }
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.relativePath); err != nil {
			_ = rows.Close()
			return err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	for _, e := range entries {
		//nolint:gosec // table is an internal constant ("assets"/"sidecar_files"), never user input
		if _, err := tx.ExecContext(ctx,
			"UPDATE "+table+" SET path_key = ? WHERE id = ?",
			domain.PathKey(e.relativePath), e.id); err != nil {
			return err
		}
	}
	return nil
}
