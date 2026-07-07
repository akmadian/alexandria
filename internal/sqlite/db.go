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
	Assets  *AssetRepo
	Sources *SourceRepo
	Dups    *DuplicateRepo
}

func reposFor(q DBTX) Repos {
	return Repos{
		Assets:  &AssetRepo{DB: q},
		Sources: &SourceRepo{DB: q},
		Dups:    &DuplicateRepo{DB: q},
	}
}

// Repos returns repositories that autocommit each statement against the DB.
func (s *Store) Repos() Repos { return reposFor(s.DB) }

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
