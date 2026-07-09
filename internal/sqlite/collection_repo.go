package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/domain"
)

type CollectionRepo struct {
	DB DBTX
}

func (r *CollectionRepo) List(ctx context.Context) ([]*domain.Collection, error) {
	rows, err := r.DB.QueryContext(ctx,
		"SELECT id, name, parent_id, kind, query, cover_asset_id, sort_field, sort_dir, created_at, updated_at FROM collections ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*domain.Collection
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (r *CollectionRepo) Get(ctx context.Context, id string) (*domain.Collection, error) {
	row := r.DB.QueryRowContext(ctx,
		"SELECT id, name, parent_id, kind, query, cover_asset_id, sort_field, sort_dir, created_at, updated_at FROM collections WHERE id = ?", id)
	c, err := scanCollectionRow(row)
	if err == sql.ErrNoRows {
		return nil, &domain.NotFoundError{Resource: "collection", ID: id}
	}
	return c, err
}

func (r *CollectionRepo) Create(ctx context.Context, collection *domain.Collection) error {
	if collection.Kind == domain.CollectionKindSmart && collection.Query != nil {
		if err := validateStoredQuery(*collection.Query); err != nil {
			return fmt.Errorf("collection create: invalid query: %w", err)
		}
	}
	now := formatTime(time.Now().UTC())
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO collections (id, name, parent_id, kind, query, cover_asset_id, sort_field, sort_dir, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		collection.ID, collection.Name, collection.ParentID, collection.Kind,
		collection.Query, collection.CoverAssetID, collection.SortField, collection.SortDir,
		now, now)
	return err
}

func (r *CollectionRepo) Update(ctx context.Context, collection *domain.Collection) error {
	if collection.Kind == domain.CollectionKindSmart && collection.Query != nil {
		if err := validateStoredQuery(*collection.Query); err != nil {
			return fmt.Errorf("collection update: invalid query: %w", err)
		}
	}
	now := formatTime(time.Now().UTC())
	res, err := r.DB.ExecContext(ctx,
		`UPDATE collections SET name = ?, parent_id = ?, kind = ?, query = ?, cover_asset_id = ?,
		 sort_field = ?, sort_dir = ?, updated_at = ? WHERE id = ?`,
		collection.Name, collection.ParentID, collection.Kind, collection.Query,
		collection.CoverAssetID, collection.SortField, collection.SortDir,
		now, collection.ID)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "collection", collection.ID)
}

func (r *CollectionRepo) Delete(ctx context.Context, id string) error {
	res, err := r.DB.ExecContext(ctx, "DELETE FROM collections WHERE id = ?", id)
	if err != nil {
		return err
	}
	return checkRowsAffected(res, "collection", id)
}

func (r *CollectionRepo) AddAsset(ctx context.Context, collectionID, assetID string) error {
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO collection_assets (collection_id, asset_id, position, added_at)
		 VALUES (?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM collection_assets WHERE collection_id = ?), ?)`,
		collectionID, assetID, collectionID, formatTime(time.Now().UTC()))
	return err
}

func (r *CollectionRepo) RemoveAsset(ctx context.Context, collectionID, assetID string) error {
	_, err := r.DB.ExecContext(ctx,
		"DELETE FROM collection_assets WHERE collection_id = ? AND asset_id = ?",
		collectionID, assetID)
	return err
}

func validateStoredQuery(queryJSON string) error {
	var query ast.Query
	if err := json.Unmarshal([]byte(queryJSON), &query); err != nil {
		return err
	}
	return ast.Validate(query)
}

type collectionScanner interface {
	Scan(dest ...any) error
}

func scanCollectionFrom(sc collectionScanner) (*domain.Collection, error) {
	var c domain.Collection
	var parentID, query, coverAssetID, sortField sql.NullString
	var createdAt, updatedAt string
	err := sc.Scan(&c.ID, &c.Name, &parentID, &c.Kind, &query, &coverAssetID,
		&sortField, &c.SortDir, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	c.ParentID = nullStringPtr(parentID)
	c.Query = nullStringPtr(query)
	c.CoverAssetID = nullStringPtr(coverAssetID)
	c.SortField = nullStringPtr(sortField)
	c.CreatedAt = parseTime(createdAt)
	c.UpdatedAt = parseTime(updatedAt)
	return &c, nil
}

func scanCollection(rows *sql.Rows) (*domain.Collection, error) {
	return scanCollectionFrom(rows)
}

func scanCollectionRow(row *sql.Row) (*domain.Collection, error) {
	return scanCollectionFrom(row)
}
