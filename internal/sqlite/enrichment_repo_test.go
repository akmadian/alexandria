package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
)

func TestEnrichmentRepo_DLQLifecycle(t *testing.T) {
	db := testutil.NewTestDB(t)
	source := testutil.NewTestSource(t, db, "dlq")
	asset := testutil.NewTestAsset(t, db, source.ID, "broken.jpg")
	repo := &sqlite.EnrichmentRepo{DB: db}
	ctx := context.Background()

	// Repeat failures upsert onto one (asset, kind) row, bumping attempts and
	// refreshing the reason.
	if err := repo.LogFailure(ctx, asset.ID, "thumbnail", "decode_failed", "boom"); err != nil {
		t.Fatalf("LogFailure: %v", err)
	}
	if err := repo.LogFailure(ctx, asset.ID, "thumbnail", "timeout", "slower boom"); err != nil {
		t.Fatalf("LogFailure repeat: %v", err)
	}
	failures, err := repo.ListFailures(ctx, asset.ID)
	if err != nil {
		t.Fatalf("ListFailures: %v", err)
	}
	if len(failures) != 1 || failures[0].Attempts != 2 || failures[0].ReasonCode != "timeout" {
		t.Fatalf("failures = %+v, want one row with attempts 2, reason timeout", failures)
	}
	if failures[0].LastAttemptAt.IsZero() {
		t.Fatal("LastAttemptAt not recorded")
	}

	// Success clears the row; clearing an absent row is a no-op.
	if err := repo.ClearFailure(ctx, asset.ID, "thumbnail"); err != nil {
		t.Fatalf("ClearFailure: %v", err)
	}
	if err := repo.ClearFailure(ctx, asset.ID, "thumbnail"); err != nil {
		t.Fatalf("ClearFailure (absent): %v", err)
	}
	failures, err = repo.ListFailures(ctx, asset.ID)
	if err != nil {
		t.Fatalf("ListFailures after clear: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("failures after clear = %+v, want none", failures)
	}
}

func TestEnrichmentRepo_ListMissingArtifacts(t *testing.T) {
	db := testutil.NewTestDB(t)
	source := testutil.NewTestSource(t, db, "scan")
	older := testutil.NewTestAsset(t, db, source.ID, "older.jpg")
	newer := testutil.NewTestAsset(t, db, source.ID, "newer.jpg")
	enriched := testutil.NewTestAsset(t, db, source.ID, "done.jpg")
	exhausted := testutil.NewTestAsset(t, db, source.ID, "cursed.jpg")
	offline := testutil.NewTestAsset(t, db, source.ID, "gone.jpg")
	repo := &sqlite.EnrichmentRepo{DB: db}
	ctx := context.Background()

	mustExec(t, db, "UPDATE assets SET ingested_at = ? WHERE id = ?", "2026-01-01T00:00:00Z", older.ID)
	mustExec(t, db, "UPDATE assets SET ingested_at = ? WHERE id = ?", "2026-06-01T00:00:00Z", newer.ID)
	mustExec(t, db, "UPDATE assets SET thumbnail_at = ? WHERE id = ?", time.Now().UTC().Format(time.RFC3339), enriched.ID)
	mustExec(t, db, "UPDATE assets SET file_status = 'missing' WHERE id = ?", offline.ID)
	for range 5 {
		if err := repo.LogFailure(ctx, exhausted.ID, "thumbnail", "decode_failed", "boom"); err != nil {
			t.Fatalf("LogFailure: %v", err)
		}
	}

	scan := &sqlite.MissingArtifactScan{
		Kind:           "thumbnail",
		ArtifactColumn: "thumbnail_at",
		Extensions:     []string{"jpg"},
		MaxAttempts:    5,
		Limit:          10,
	}
	missing, err := repo.ListMissingArtifacts(ctx, scan)
	if err != nil {
		t.Fatalf("ListMissingArtifacts: %v", err)
	}
	// Enriched, exhausted, and offline rows are all excluded; newest ingest first.
	if len(missing) != 2 || missing[0].AssetID != newer.ID || missing[1].AssetID != older.ID {
		t.Fatalf("missing = %+v, want [%s %s] (import recency order)", missing, newer.ID, older.ID)
	}
	if missing[0].SizeBytes != 1024 || missing[0].FileType == "" {
		t.Fatalf("scan hit missing size/type: %+v", missing[0])
	}

	// The prerequisite clause executes and filters: with the artifact itself as
	// its own prerequisite, "missing" and "prerequisite present" contradict, so
	// nothing qualifies.
	scan.PrerequisiteColumns = []string{"thumbnail_at"}
	blocked, err := repo.ListMissingArtifacts(ctx, scan)
	if err != nil {
		t.Fatalf("ListMissingArtifacts with prerequisite: %v", err)
	}
	if len(blocked) != 0 {
		t.Fatalf("prerequisite clause did not filter: %+v", blocked)
	}
	scan.PrerequisiteColumns = nil

	// An artifact column off the allowlist is an error, never interpolated SQL.
	scan.ArtifactColumn = "rating"
	if _, err := repo.ListMissingArtifacts(ctx, scan); err == nil {
		t.Fatal("ListMissingArtifacts accepted a judgment column as artifact")
	}
	scan.ArtifactColumn = "thumbnail_at"
	scan.PrerequisiteColumns = []string{"note"}
	if _, err := repo.ListMissingArtifacts(ctx, scan); err == nil {
		t.Fatal("ListMissingArtifacts accepted a judgment column as prerequisite")
	}
	scan.PrerequisiteColumns = nil
	scan.Extensions = nil
	if _, err := repo.ListMissingArtifacts(ctx, scan); err == nil {
		t.Fatal("ListMissingArtifacts accepted an empty extension set")
	}
}

func TestEnrichmentRepo_MissingAndEligible(t *testing.T) {
	db := testutil.NewTestDB(t)
	source := testutil.NewTestSource(t, db, "eligible")
	pending := testutil.NewTestAsset(t, db, source.ID, "pending.jpg")
	enriched := testutil.NewTestAsset(t, db, source.ID, "done.jpg")
	repo := &sqlite.EnrichmentRepo{DB: db}
	ctx := context.Background()
	mustExec(t, db, "UPDATE assets SET thumbnail_at = ? WHERE id = ?", time.Now().UTC().Format(time.RFC3339), enriched.ID)

	probeFor := func(assetID string) *sqlite.EligibilityProbe {
		return &sqlite.EligibilityProbe{
			AssetID: assetID, Kind: "thumbnail", ArtifactColumn: "thumbnail_at", MaxAttempts: 5,
		}
	}
	assertEligible := func(assetID string, want bool, why string) {
		t.Helper()
		eligible, err := repo.MissingAndEligible(ctx, probeFor(assetID))
		if err != nil {
			t.Fatalf("MissingAndEligible(%s): %v", why, err)
		}
		if eligible != want {
			t.Fatalf("MissingAndEligible(%s) = %v, want %v", why, eligible, want)
		}
	}
	assertEligible(pending.ID, true, "artifact missing")
	assertEligible(enriched.ID, false, "artifact present")
	assertEligible("no-such-asset", false, "asset gone")

	// An exhausted DLQ row is terminal for the recheck too — a hint must not
	// defy the attempt budget the scan respects.
	for range 5 {
		if err := repo.LogFailure(ctx, pending.ID, "thumbnail", "decode_failed", "boom"); err != nil {
			t.Fatalf("LogFailure: %v", err)
		}
	}
	assertEligible(pending.ID, false, "attempt-exhausted")
	if err := repo.ClearFailure(ctx, pending.ID, "thumbnail"); err != nil {
		t.Fatalf("ClearFailure: %v", err)
	}
	assertEligible(pending.ID, true, "cleared DLQ restores eligibility")

	// A prerequisite on itself: missing artifact + missing prerequisite = not yet.
	prerequisiteProbe := probeFor(pending.ID)
	prerequisiteProbe.PrerequisiteColumns = []string{"thumbnail_at"}
	eligible, err := repo.MissingAndEligible(ctx, prerequisiteProbe)
	if err != nil {
		t.Fatalf("MissingAndEligible with prerequisite: %v", err)
	}
	if eligible {
		t.Fatal("eligible despite an absent prerequisite artifact")
	}
	judgmentProbe := probeFor(pending.ID)
	judgmentProbe.ArtifactColumn = "flag"
	if _, err := repo.MissingAndEligible(ctx, judgmentProbe); err == nil {
		t.Fatal("MissingAndEligible accepted a judgment column")
	}
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
