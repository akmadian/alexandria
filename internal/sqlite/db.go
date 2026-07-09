package sqlite

import (
	"context"
	"database/sql"
)

// DBTX is the subset of *sql.DB / *sql.Tx the repositories use. Holding this
// (rather than a concrete *sql.DB) lets the same repo run either directly or
// inside a transaction — both types satisfy it — which is what makes the
// pipeline's batched, atomic writes possible.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store owns the *sql.DB and hands out repositories — either non-transactional
// (Repos) or bound to a transaction (InTx).
type Store struct {
	DB *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{DB: db} }

// Repos bundles the repositories bound to one DBTX (the DB itself, or a tx).
type Repos struct {
	Assets      *AssetRepo
	Sources     *SourceRepo
	Dups        *DuplicateRepo
	Sidecars    *SidecarRepo
	Imports     *ImportRepo
	Tags        *TagRepo
	Collections *CollectionRepo
}

func reposFor(q DBTX) Repos {
	return Repos{
		Assets:      &AssetRepo{DB: q},
		Sources:     &SourceRepo{DB: q},
		Dups:        &DuplicateRepo{DB: q},
		Sidecars:    &SidecarRepo{DB: q},
		Imports:     &ImportRepo{DB: q},
		Tags:        &TagRepo{DB: q},
		Collections: &CollectionRepo{DB: q},
	}
}

// InTx runs fn with repositories bound to a single transaction: commit on
// success, roll back on any error (or panic). This is the unit of atomicity for
// multi-statement writes (relink = UpdatePath + SetFileStatus; duplicate =
// Create + Dups.Log) and for the pipeline's batched commits.
//
// ponytail: uses the driver's default BEGIN (deferred). Upgrade to BEGIN
// IMMEDIATE via the "_txlock=immediate" DSN param (set in Open) if write-lock
// contention ever surfaces; deferred is correct, just lazier about the lock.
func (s *Store) InTx(ctx context.Context, fn func(Repos) error) (err error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(reposFor(tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ImportKeywords runs a whole keyword import in one transaction so the EnsureTag
// calls building a hierarchy share it (a half-built chain never commits). This is
// the seam the XMP Syncer / LrC migration hold as catalog.TagRepository.
func (s *Store) ImportKeywords(ctx context.Context, assetID string, flat []string, hierarchical [][]string, source string) error {
	return s.InTx(ctx, func(r Repos) error {
		return r.Tags.ImportKeywords(ctx, assetID, flat, hierarchical, source)
	})
}

func (s *Store) AssetTagNames(ctx context.Context, assetID string) ([]string, []string, error) {
	return (&TagRepo{DB: s.DB}).AssetTagNames(ctx, assetID)
}
