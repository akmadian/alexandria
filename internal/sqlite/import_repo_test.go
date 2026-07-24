package sqlite_test

import (
	"context"
	"testing"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
)

// The session lifecycle the dev harness (and, soon, the frontend) reads:
// Start mints an open row, Finish stamps counts + per-extension tallies, and
// ListSessions reads them back — exercising the tallyJSON/parseTally round-trip
// that the DLQ viewer depends on.
func TestImportRepo_SessionRoundTrip(t *testing.T) {
	db := testutil.NewTestDB(t)
	src := testutil.NewTestVolume(t, db, "s")
	repo := &sqlite.ImportRepo{DB: db}
	ctx := context.Background()

	sessionID, err := repo.Start(ctx, src.ID, "import")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if sessionID == "" {
		t.Fatal("Start returned an empty session id")
	}

	// A just-started session is listable and still open (no finished_at).
	open, err := repo.ListSessions(ctx, 10)
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(open) != 1 || open[0].ID != sessionID {
		t.Fatalf("open session not listed: %+v", open)
	}
	if open[0].FinishedAt != nil {
		t.Errorf("fresh session should have no finished_at, got %v", open[0].FinishedAt)
	}

	session := &domain.ImportSession{
		Added: 12, Updated: 3, Moved: 1, Skipped: 4, Dups: 2, Errors: 1,
		SkippedUnknown: map[string]int{"txt": 2, "doc": 1},
		SkippedIgnored: map[string]int{"*.tmp": 4},
	}
	if err := repo.Finish(ctx, sessionID, session); err != nil {
		t.Fatalf("finish: %v", err)
	}

	got, err := repo.ListSessions(ctx, 10)
	if err != nil {
		t.Fatalf("list finished: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d", len(got))
	}
	final := got[0]
	if final.FinishedAt == nil {
		t.Error("finished session must carry finished_at")
	}
	if final.Added != 12 || final.Updated != 3 || final.Moved != 1 ||
		final.Skipped != 4 || final.Dups != 2 || final.Errors != 1 {
		t.Errorf("counts not persisted: %+v", final)
	}
	// parseTally must reconstruct both tally maps from their JSON columns.
	if final.SkippedUnknown["txt"] != 2 || final.SkippedUnknown["doc"] != 1 {
		t.Errorf("SkippedUnknown = %v, want {txt:2 doc:1}", final.SkippedUnknown)
	}
	if final.SkippedIgnored["*.tmp"] != 4 {
		t.Errorf("SkippedIgnored = %v, want {*.tmp:4}", final.SkippedIgnored)
	}
}

// UpdateCounts refreshes the running tallies on an OPEN session (mid-import
// progress): it writes the counts, overwrites (not accumulates) on a later call,
// and must never stamp finished_at — that is Finish's job alone.
func TestImportRepo_UpdateCountsMidFlight(t *testing.T) {
	db := testutil.NewTestDB(t)
	src := testutil.NewTestVolume(t, db, "s")
	repo := &sqlite.ImportRepo{DB: db}
	ctx := context.Background()

	sessionID, err := repo.Start(ctx, src.ID, "import")
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.UpdateCounts(ctx, sessionID, &domain.ImportSession{Added: 5, Dups: 1, Errors: 2}); err != nil {
		t.Fatalf("update counts: %v", err)
	}
	got, _ := repo.ListSessions(ctx, 1)
	if len(got) != 1 || got[0].Added != 5 || got[0].Dups != 1 || got[0].Errors != 2 {
		t.Fatalf("mid-flight counts not persisted: %+v", got)
	}
	if got[0].FinishedAt != nil {
		t.Error("UpdateCounts must not finish the session")
	}

	// A later update overwrites the running counts, it does not add to them.
	if err := repo.UpdateCounts(ctx, sessionID, &domain.ImportSession{Added: 12, Dups: 1, Errors: 2}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.ListSessions(ctx, 1)
	if got[0].Added != 12 {
		t.Errorf("later UpdateCounts should overwrite added, got %d, want 12", got[0].Added)
	}
}

// A finished session with no skips must round-trip nil tally maps (tallyJSON
// writes NULL for an empty map; parseTally reads NULL back as nil), not an empty
// non-nil map or a "null" string.
func TestImportRepo_EmptyTalliesRoundTripNil(t *testing.T) {
	db := testutil.NewTestDB(t)
	src := testutil.NewTestVolume(t, db, "s")
	repo := &sqlite.ImportRepo{DB: db}
	ctx := context.Background()

	sessionID, err := repo.Start(ctx, src.ID, "reconcile")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(ctx, sessionID, &domain.ImportSession{Added: 1}); err != nil {
		t.Fatalf("finish: %v", err)
	}

	got, _ := repo.ListSessions(ctx, 1)
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d", len(got))
	}
	if got[0].SkippedUnknown != nil || got[0].SkippedIgnored != nil {
		t.Errorf("empty tallies should read back nil, got unknown=%v ignored=%v",
			got[0].SkippedUnknown, got[0].SkippedIgnored)
	}
}

// The limit is honored, newest-first: three sessions, limit 2 returns the two
// most recently started.
func TestImportRepo_ListSessionsLimit(t *testing.T) {
	db := testutil.NewTestDB(t)
	src := testutil.NewTestVolume(t, db, "s")
	repo := &sqlite.ImportRepo{DB: db}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := repo.Start(ctx, src.ID, "import"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ListSessions(ctx, 2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("limit 2 returned %d sessions", len(got))
	}
}

// DeleteByID is the hard-delete primitive the review-queue "confirm move"
// resolution will use (asset_repo.go, DEFERRED §5). It must physically remove the
// row AND let the FK cascade clear dependent rows — proven here against asset_tags,
// so the primitive is trusted before the review queue wires it.
func TestAssetRepo_DeleteByID_HardDeleteCascades(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()
	src := testutil.NewTestVolume(t, db, "s")
	asset := testutil.NewTestAsset(t, db, src.ID, "gone.jpg")

	now := "2026-01-01T00:00:00Z"
	if _, err := db.ExecContext(ctx, `INSERT INTO tags (id, name, slug, path, created_at) VALUES ('t1', 'Beach', 'beach', '/t1/', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO asset_tags (asset_id, tag_id, source, created_at) VALUES (?, 't1', 'user', ?)`, asset.ID, now); err != nil {
		t.Fatal(err)
	}

	if err := repo.DeleteByID(ctx, asset.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Unlike SoftDelete, the row is physically gone (Get returns NotFound).
	if _, err := repo.Get(ctx, asset.ID); err == nil {
		t.Fatal("DeleteByID must physically remove the row")
	}
	var tagLinks int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM asset_tags WHERE asset_id = ?`, asset.ID).Scan(&tagLinks); err != nil {
		t.Fatal(err)
	}
	if tagLinks != 0 {
		t.Errorf("FK cascade should have removed asset_tags, found %d", tagLinks)
	}
}
