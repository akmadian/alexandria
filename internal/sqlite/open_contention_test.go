package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/sqlite"
)

// A read-then-write transaction (the D28 enrichment staleness guard: SELECT to
// check currency, then UPDATE to apply) must survive a write lock held by the
// other writer goroutine, not fail SQLITE_BUSY instantly. This is the 2026-07-20
// import/enrichment contention defect: with a DEFERRED transaction SQLite refuses
// to wait on the read→write lock upgrade (deadlock avoidance), so busy_timeout is
// never consulted and the second writer dies at once. _txlock=immediate takes the
// write lock at BEGIN, which the busy handler *does* cover — see Open's DSN.
//
// This opens a real on-disk catalog via sqlite.Open so the connection pool hands
// out two connections; the rest of the suite pins MaxOpenConns=1, which serializes
// at the pool and structurally cannot reproduce lock contention. Store.InTx runs
// exactly cat.DB.BeginTx(ctx, nil), so a raw tx here covers the production write
// path.
func TestOpen_ReadThenWriteSurvivesConcurrentWriteLock(t *testing.T) {
	cat, err := sqlite.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer cat.Close()

	ctx := context.Background()
	if _, err := cat.DB.ExecContext(ctx, `CREATE TABLE probe(id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatalf("create probe: %v", err)
	}
	if _, err := cat.DB.ExecContext(ctx, `INSERT INTO probe VALUES (1, 'initial')`); err != nil {
		t.Fatalf("seed probe: %v", err)
	}

	const holdFor = 300 * time.Millisecond
	holder := make(chan error, 1)
	lockTaken := make(chan struct{})
	go func() {
		holder <- func() error {
			tx, err := cat.DB.BeginTx(ctx, nil) // IMMEDIATE: write lock taken here
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE probe SET v = 'holder' WHERE id = 1`); err != nil {
				_ = tx.Rollback()
				return err
			}
			close(lockTaken)
			time.Sleep(holdFor) // hold the write lock while the reader contends
			return tx.Commit()
		}()
	}()
	<-lockTaken

	// The enrichment shape: read first (pins a snapshot), then upgrade to a write.
	start := time.Now()
	tx, err := cat.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("reader begin: %v", err)
	}
	var v string
	if err := tx.QueryRowContext(ctx, `SELECT v FROM probe WHERE id = 1`).Scan(&v); err != nil {
		_ = tx.Rollback()
		t.Fatalf("reader select: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE probe SET v = 'reader' WHERE id = 1`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("read-then-write upgrade failed under contention (the defect): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("reader commit: %v", err)
	}
	waited := time.Since(start)

	if err := <-holder; err != nil {
		t.Fatalf("holder tx: %v", err)
	}
	// The upgrade succeeded only by waiting out the holder — proof the busy handler
	// engaged rather than the holder having already released.
	if waited < holdFor/2 {
		t.Fatalf("reader upgrade returned in %v, expected to wait out the ~%v hold — busy handler did not engage", waited, holdFor)
	}
}
