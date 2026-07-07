package sqlite

import (
	"context"
	"database/sql"
)

// RebuildFTS rebuilds the derived assets_fts index from scratch, in one
// transaction. FTS is derived state (rebuildable by definition); this is its
// registered rebuild path — used to repair the index or after a plain VACUUM
// (which can renumber assets.rowid and desync FTS, which keys on rowid).
//
// The asset-resident columns come straight from `assets`; the `tags` column is
// composed from the asset_tags⋈tags join here (it is the one FTS column the
// per-row triggers cannot maintain).
func RebuildFTS(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM assets_fts`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO assets_fts (rowid, asset_id, filename, camera_make, camera_model, lens_model, title, caption, note, tags)
		SELECT a.rowid, a.id, a.filename, a.camera_make, a.camera_model, a.lens_model, a.title, a.caption, a.note,
			COALESCE((
				SELECT group_concat(t.name, ' ')
				FROM asset_tags at JOIN tags t ON t.id = at.tag_id
				WHERE at.asset_id = a.id
			), '')
		FROM assets a`); err != nil {
		return err
	}
	return tx.Commit()
}
