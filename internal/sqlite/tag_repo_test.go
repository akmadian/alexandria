package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
)

// activeTagIDs returns the tag IDs currently attached (removed_at IS NULL) to an
// asset — the membership the hot reverse index serves.
func activeTagIDs(t *testing.T, db *sql.DB, assetID string) map[string]bool {
	t.Helper()
	rows, err := db.Query("SELECT tag_id FROM asset_tags WHERE asset_id = ? AND removed_at IS NULL", assetID)
	if err != nil {
		t.Fatalf("query asset_tags: %v", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[id] = true
	}
	return out
}

func tagPath(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var path string
	if err := db.QueryRow("SELECT path FROM tags WHERE id = ?", id).Scan(&path); err != nil {
		t.Fatalf("query path for %s: %v", id, err)
	}
	return path
}

// TestEnsureTag_FindOrCreate is acceptance "Find-or-create": idempotent per
// (slug, parent); the same name under a different parent is a distinct row; path
// tracks the parent chain.
func TestEnsureTag_FindOrCreate(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.TagRepo{DB: db}
	ctx := context.Background()

	travel, err := repo.EnsureTag(ctx, "Travel", nil)
	if err != nil {
		t.Fatalf("ensure Travel: %v", err)
	}
	japan, err := repo.EnsureTag(ctx, "Japan", &travel)
	if err != nil {
		t.Fatalf("ensure Japan: %v", err)
	}
	tokyo1, err := repo.EnsureTag(ctx, "Tokyo", &japan)
	if err != nil {
		t.Fatalf("ensure Tokyo: %v", err)
	}

	// Idempotent: same (slug, parent) → same ID (case-insensitive via slug).
	tokyo2, err := repo.EnsureTag(ctx, "tokyo", &japan)
	if err != nil {
		t.Fatalf("re-ensure tokyo: %v", err)
	}
	if tokyo2 != tokyo1 {
		t.Errorf("EnsureTag not idempotent: %s != %s", tokyo2, tokyo1)
	}

	// Same name, different parent → distinct row.
	places, err := repo.EnsureTag(ctx, "Places", nil)
	if err != nil {
		t.Fatalf("ensure Places: %v", err)
	}
	tokyoUnderPlaces, err := repo.EnsureTag(ctx, "Tokyo", &places)
	if err != nil {
		t.Fatalf("ensure Tokyo/Places: %v", err)
	}
	if tokyoUnderPlaces == tokyo1 {
		t.Error("Tokyo under a different parent must be a distinct row")
	}

	// Paths are the self-inclusive ancestry chain.
	if got, want := tagPath(t, db, travel), "/"+travel+"/"; got != want {
		t.Errorf("Travel path = %q, want %q", got, want)
	}
	if got, want := tagPath(t, db, tokyo1), "/"+travel+"/"+japan+"/"+tokyo1+"/"; got != want {
		t.Errorf("Tokyo path = %q, want %q", got, want)
	}
}

// TestImportKeywords_DedupeAndUnion is acceptance "Keyword import + dedupe" and
// "Union never deletes": hierarchical is authoritative, flat contributes only the
// genuinely-flat name, and a re-run adds nothing.
func TestImportKeywords_DedupeAndUnion(t *testing.T) {
	db := testutil.NewTestDB(t)
	store := sqlite.NewStore(db)
	source := testutil.NewTestSource(t, db, "s")
	asset := testutil.NewTestAsset(t, db, source.ID, "photo.jpg")
	ctx := context.Background()

	flat := []string{"Travel", "Japan", "Tokyo", "Sunrise"}
	hierarchical := [][]string{{"Travel", "Japan", "Tokyo"}}

	if err := store.ImportKeywords(ctx, asset.ID, flat, hierarchical, "xmp"); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Exactly four tags exist: Travel>Japan>Tokyo + root Sunrise. No duplicate flats.
	var tagCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM tags").Scan(&tagCount); err != nil {
		t.Fatal(err)
	}
	if tagCount != 4 {
		t.Errorf("tag count = %d, want 4 (Travel/Japan/Tokyo/Sunrise)", tagCount)
	}

	// Asset attached to the LEAF Tokyo + root Sunrise only (not Travel/Japan).
	active := activeTagIDs(t, db, asset.ID)
	if len(active) != 2 {
		t.Fatalf("active attachments = %d, want 2 (Tokyo + Sunrise)", len(active))
	}
	tokyo := tagID(t, db, "tokyo", parentOf(t, db, "japan"))
	sunrise := tagID(t, db, "sunrise", "")
	if !active[tokyo] || !active[sunrise] {
		t.Errorf("attached = %v, want Tokyo(%s) + Sunrise(%s)", active, tokyo, sunrise)
	}

	// Re-run: union adds no rows.
	if err := store.ImportKeywords(ctx, asset.ID, flat, hierarchical, "xmp"); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	var attachCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM asset_tags WHERE asset_id = ?", asset.ID).Scan(&attachCount); err != nil {
		t.Fatal(err)
	}
	if attachCount != 2 {
		t.Errorf("attachments after re-import = %d, want 2 (union never duplicates)", attachCount)
	}
}

// TestImportKeywords_TombstoneRespected is acceptance "Tombstone respected": a
// user-suppressed tag stays gone when the sidecar re-asserts it.
func TestImportKeywords_TombstoneRespected(t *testing.T) {
	db := testutil.NewTestDB(t)
	store := sqlite.NewStore(db)
	source := testutil.NewTestSource(t, db, "s")
	asset := testutil.NewTestAsset(t, db, source.ID, "photo.jpg")
	ctx := context.Background()

	if err := store.ImportKeywords(ctx, asset.ID, []string{"Sunset"}, nil, "xmp"); err != nil {
		t.Fatalf("import: %v", err)
	}
	sunset := tagID(t, db, "sunset", "")

	// User suppresses it (the judgment tombstone).
	if _, err := db.Exec("UPDATE asset_tags SET removed_at = '2026-07-07T00:00:00Z' WHERE asset_id = ? AND tag_id = ?", asset.ID, sunset); err != nil {
		t.Fatalf("suppress: %v", err)
	}

	// Sidecar re-asserts the same keyword: DO NOTHING must not clear removed_at.
	if err := store.ImportKeywords(ctx, asset.ID, []string{"Sunset"}, nil, "xmp"); err != nil {
		t.Fatalf("re-import: %v", err)
	}

	var removedAt sql.NullString
	if err := db.QueryRow("SELECT removed_at FROM asset_tags WHERE asset_id = ? AND tag_id = ?", asset.ID, sunset).Scan(&removedAt); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !removedAt.Valid {
		t.Error("removed_at was cleared by the sync path — the suppressed tag resurrected (D8 violation)")
	}
	if active := activeTagIDs(t, db, asset.ID); active[sunset] {
		t.Error("suppressed tag appears in the active membership")
	}
}

// TestSubtreeFilter is acceptance "Subtree filter": path GLOB expands a node to
// its whole subtree, gathering assets across descendants.
func TestSubtreeFilter(t *testing.T) {
	db := testutil.NewTestDB(t)
	store := sqlite.NewStore(db)
	source := testutil.NewTestSource(t, db, "s")
	a1 := testutil.NewTestAsset(t, db, source.ID, "tokyo.jpg")
	a2 := testutil.NewTestAsset(t, db, source.ID, "osaka.jpg")
	ctx := context.Background()

	if err := store.ImportKeywords(ctx, a1.ID, nil, [][]string{{"Travel", "Japan", "Tokyo"}}, "xmp"); err != nil {
		t.Fatalf("import a1: %v", err)
	}
	if err := store.ImportKeywords(ctx, a2.ID, nil, [][]string{{"Travel", "Japan", "Osaka"}}, "xmp"); err != nil {
		t.Fatalf("import a2: %v", err)
	}

	japan := tagID(t, db, "japan", parentOf(t, db, "travel"))
	japanPath := tagPath(t, db, japan)

	// Both assets surface under the Japan subtree via a single path prefix scan.
	rows, err := db.Query(`
		SELECT DISTINCT at.asset_id
		FROM tags t
		JOIN asset_tags at ON at.tag_id = t.id AND at.removed_at IS NULL
		WHERE t.path GLOB ? || '*'`, japanPath)
	if err != nil {
		t.Fatalf("subtree query: %v", err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got[id] = true
	}
	if !got[a1.ID] || !got[a2.ID] {
		t.Errorf("Japan subtree assets = %v, want both tokyo(%s) and osaka(%s)", got, a1.ID, a2.ID)
	}
}

// TestRebuildTagPaths is acceptance "reparent" tail: RebuildTagPaths reproduces
// identical paths from parent_id. Corrupt every path, rebuild, verify recovery.
func TestRebuildTagPaths(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := &sqlite.TagRepo{DB: db}
	ctx := context.Background()

	travel, _ := repo.EnsureTag(ctx, "Travel", nil)
	japan, _ := repo.EnsureTag(ctx, "Japan", &travel)
	tokyo, _ := repo.EnsureTag(ctx, "Tokyo", &japan)
	want := tagPath(t, db, tokyo)

	if _, err := db.Exec("UPDATE tags SET path = 'CORRUPT'"); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if err := repo.RebuildTagPaths(ctx); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if got := tagPath(t, db, tokyo); got != want {
		t.Errorf("rebuilt Tokyo path = %q, want %q", got, want)
	}
}

// --- small query helpers (test-only) ---

func tagID(t *testing.T, db *sql.DB, slug, parentID string) string {
	t.Helper()
	var id string
	if err := db.QueryRow("SELECT id FROM tags WHERE slug = ? AND IFNULL(parent_id,'') = ?", slug, parentID).Scan(&id); err != nil {
		t.Fatalf("tagID(%q, %q): %v", slug, parentID, err)
	}
	return id
}

func parentOf(t *testing.T, db *sql.DB, slug string) string {
	t.Helper()
	var id string
	if err := db.QueryRow("SELECT id FROM tags WHERE slug = ?", slug).Scan(&id); err != nil {
		t.Fatalf("parentOf(%q): %v", slug, err)
	}
	return id
}
