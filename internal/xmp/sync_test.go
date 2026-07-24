package xmp

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/dependency"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/settings"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
	"github.com/charmbracelet/log"
)

// TestSyncSidecarInbound is impl/06 acceptance #1 (judgment slice) end-to-end
// against a real SQLite catalog and the real exiftool daemon: an LrC sidecar
// applies rating+label, judgment_modified_at is NOT bumped (the oscillator guard,
// #3), and a second pass is a no-op.
func TestSyncSidecarInbound(t *testing.T) {
	status := dependency.Discover(dependency.Exiftool, "")
	if !status.Available() {
		t.Skipf("exiftool not available (%s)", status.State)
	}
	daemon, err := dependency.StartExiftool(status, log.Default())
	if err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer daemon.Close()

	db := testutil.NewTestDB(t)
	source := testutil.NewTestVolume(t, db, "s")
	asset := testutil.NewTestAsset(t, db, source.ID, "sunrise.orf")
	repo := &sqlite.AssetRepo{DB: db}

	syncer := NewSyncer(daemon, repo, repo, nil, settings.DefaultSettings, log.Default())
	ctx := context.Background()

	action, err := syncer.SyncSidecar(ctx, asset, "testdata/lightroom.xmp")
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if action != ActionApplyInbound {
		t.Fatalf("first sync action = %q, want %q", action, ActionApplyInbound)
	}

	applied, err := repo.Get(ctx, asset.ID)
	if err != nil {
		t.Fatalf("reload asset: %v", err)
	}
	if applied.Rating == nil || *applied.Rating != 4 {
		t.Errorf("Rating = %v, want 4", applied.Rating)
	}
	if applied.ColorLabel == nil || *applied.ColorLabel != domain.ColorLabelRed {
		t.Errorf("ColorLabel = %v, want red", applied.ColorLabel)
	}
	if applied.JudgmentModifiedAt != nil {
		t.Errorf("judgment_modified_at was bumped (%v) — inbound sync must not (oscillator guard)", applied.JudgmentModifiedAt)
	}
	if applied.XMPHash == nil {
		t.Error("xmp_hash not recorded; second pass cannot detect a no-op")
	}
	if applied.XMPLastReadAt == nil {
		t.Error("xmp_last_read_at not advanced")
	}

	// Second pass over the unchanged sidecar with the now-synced asset: no-op.
	action, err = syncer.SyncSidecar(ctx, applied, "testdata/lightroom.xmp")
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if action != ActionNoop {
		t.Errorf("second sync action = %q, want %q (sidecar unchanged, catalog unchanged)", action, ActionNoop)
	}
}

// TestSyncSidecarClearsRemovedRating is point-2 end-to-end: once an asset is
// synced from a sidecar, editing the sidecar to REMOVE the rating clears it in the
// catalog (sidecar wins wholesale) — still without bumping judgment_modified_at.
func TestSyncSidecarClearsRemovedRating(t *testing.T) {
	status := dependency.Discover(dependency.Exiftool, "")
	if !status.Available() {
		t.Skipf("exiftool not available (%s)", status.State)
	}
	daemon, err := dependency.StartExiftool(status, log.Default())
	if err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer daemon.Close()

	db := testutil.NewTestDB(t)
	source := testutil.NewTestVolume(t, db, "s")
	asset := testutil.NewTestAsset(t, db, source.ID, "sunrise.orf")
	repo := &sqlite.AssetRepo{DB: db}
	syncer := NewSyncer(daemon, repo, repo, nil, settings.DefaultSettings, log.Default())
	ctx := context.Background()

	// First sync applies rating 4 from the full sidecar.
	if _, err := syncer.SyncSidecar(ctx, asset, "testdata/lightroom.xmp"); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	synced, err := repo.Get(ctx, asset.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if synced.Rating == nil || *synced.Rating != 4 {
		t.Fatalf("precondition: Rating = %v, want 4", synced.Rating)
	}

	// A different sidecar with no rating (a rating removed in LrC) → apply-inbound
	// clears the catalog rating.
	action, err := syncer.SyncSidecar(ctx, synced, "testdata/no-rating.xmp")
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if action != ActionApplyInbound {
		t.Fatalf("action = %q, want %q", action, ActionApplyInbound)
	}
	cleared, err := repo.Get(ctx, asset.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cleared.Rating != nil {
		t.Errorf("Rating = %v, want nil (removed in sidecar → cleared)", *cleared.Rating)
	}
	if cleared.ColorLabel != nil {
		t.Errorf("ColorLabel = %v, want nil (absent in sidecar → cleared)", *cleared.ColorLabel)
	}
	if cleared.JudgmentModifiedAt != nil {
		t.Errorf("judgment_modified_at bumped on a clear — must not (oscillator guard)")
	}
}

// TestSyncSidecarUnionsKeywords wires a real tag importer (impl/10) into the
// Syncer and verifies the impl/06 keyword-union path end-to-end: the LrC fixture
// carries flat [Travel,Japan,Tokyo] + hierarchical Travel|Japan|Tokyo, so exactly
// the leaf Tokyo attaches (dedupe drops the flat ancestors) with source='xmp'.
func TestSyncSidecarUnionsKeywords(t *testing.T) {
	status := dependency.Discover(dependency.Exiftool, "")
	if !status.Available() {
		t.Skipf("exiftool not available (%s)", status.State)
	}
	daemon, err := dependency.StartExiftool(status, log.Default())
	if err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer daemon.Close()

	db := testutil.NewTestDB(t)
	source := testutil.NewTestVolume(t, db, "s")
	asset := testutil.NewTestAsset(t, db, source.ID, "sunrise.orf")
	store := sqlite.NewStore(db)
	repo := &sqlite.AssetRepo{DB: db}

	syncer := NewSyncer(daemon, repo, repo, store, settings.DefaultSettings, log.Default())
	ctx := context.Background()

	if _, err := syncer.SyncSidecar(ctx, asset, "testdata/lightroom.xmp"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Tree Travel>Japan>Tokyo exists; only the leaf is attached.
	var attached []string
	rows, err := db.Query(`SELECT t.slug FROM asset_tags at JOIN tags t ON t.id = at.tag_id
		WHERE at.asset_id = ? AND at.removed_at IS NULL`, asset.ID)
	if err != nil {
		t.Fatalf("query attachments: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			t.Fatal(err)
		}
		attached = append(attached, slug)
	}
	if len(attached) != 1 || attached[0] != "tokyo" {
		t.Errorf("attached slugs = %v, want [tokyo] (leaf-only, flat ancestors deduped)", attached)
	}

	var source1 string
	if err := db.QueryRow(`SELECT source FROM asset_tags WHERE asset_id = ?`, asset.ID).Scan(&source1); err != nil {
		t.Fatal(err)
	}
	if source1 != "xmp" {
		t.Errorf("source = %q, want xmp", source1)
	}
}

// TestToTriagePatch checks the wholesale judgment mapping: present fields set,
// ABSENT fields clear (sidecar wins, upholding the conflict policy), out-of-range
// rating clears, unknown/empty label clears.
func TestToTriagePatch(t *testing.T) {
	syncer := NewSyncer(nil, nil, nil, nil, settings.DefaultSettings, log.Default())
	rating := func(n int) *int { return &n }

	cases := []struct {
		name       string
		fields     Fields
		wantRating domain.Opt[int]
		wantLabel  domain.Opt[domain.ColorLabel]
	}{
		{"rating+german-label", Fields{Rating: rating(4), Label: "Rot"}, domain.SetOpt(4), domain.SetOpt(domain.ColorLabelRed)},
		{"rating-only-clears-label", Fields{Rating: rating(2)}, domain.SetOpt(2), domain.ClearOpt[domain.ColorLabel]()},
		{"out-of-range-clears-rating", Fields{Rating: rating(-1)}, domain.ClearOpt[int](), domain.ClearOpt[domain.ColorLabel]()},
		{"unknown-label-clears-both", Fields{Label: "Krypton"}, domain.ClearOpt[int](), domain.ClearOpt[domain.ColorLabel]()},
		{"empty-clears-both", Fields{}, domain.ClearOpt[int](), domain.ClearOpt[domain.ColorLabel]()},
	}
	for _, tc := range cases {
		patch := syncer.toTriagePatch(&tc.fields, "asset-x")
		if !reflect.DeepEqual(patch.Rating, tc.wantRating) {
			t.Errorf("%s: Rating = %+v, want %+v", tc.name, patch.Rating, tc.wantRating)
		}
		if !reflect.DeepEqual(patch.ColorLabel, tc.wantLabel) {
			t.Errorf("%s: ColorLabel = %+v, want %+v", tc.name, patch.ColorLabel, tc.wantLabel)
		}
	}
}

// TestCatalogChanged covers the "catalog changed?" reduction.
func TestCatalogChanged(t *testing.T) {
	early := mustTime("2026-01-01T00:00:00Z")
	late := mustTime("2026-06-01T00:00:00Z")

	cases := []struct {
		name  string
		asset domain.Asset
		want  bool
	}{
		{"never-judged", domain.Asset{}, false},
		{"judged-never-synced", domain.Asset{JudgmentModifiedAt: &late}, true},
		{"judged-after-read", domain.Asset{JudgmentModifiedAt: &late, XMPLastReadAt: &early}, true},
		{"judged-before-read", domain.Asset{JudgmentModifiedAt: &early, XMPLastReadAt: &late}, false},
		{"judged-before-write", domain.Asset{JudgmentModifiedAt: &early, XMPLastWrittenAt: &late}, false},
	}
	for _, tc := range cases {
		if got := catalogChanged(&tc.asset); got != tc.want {
			t.Errorf("%s: catalogChanged = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestOscillatorGuard is acceptance #3: apply inbound with write-back enabled →
// the inbound apply must NOT trigger an outbound write. This is structural:
// ApplyXMPInbound never bumps judgment_modified_at, so catalogChanged stays false
// after an inbound apply, and Decide returns noop on the second pass.
func TestOscillatorGuard(t *testing.T) {
	status := dependency.Discover(dependency.Exiftool, "")
	if !status.Available() {
		t.Skipf("exiftool not available (%s)", status.State)
	}
	daemon, err := dependency.StartExiftool(status, log.Default())
	if err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer daemon.Close()

	db := testutil.NewTestDB(t)
	source := testutil.NewTestVolume(t, db, "s")
	asset := testutil.NewTestAsset(t, db, source.ID, "sunrise.orf")
	repo := &sqlite.AssetRepo{DB: db}

	writeBackSettings := func() settings.Settings {
		s := settings.DefaultSettings()
		s.XMPWriteBack = true
		return s
	}
	syncer := NewSyncer(daemon, repo, repo, nil, writeBackSettings, log.Default())
	ctx := context.Background()

	// Inbound apply.
	action, err := syncer.SyncSidecar(ctx, asset, "testdata/lightroom.xmp")
	if err != nil {
		t.Fatalf("inbound sync: %v", err)
	}
	if action != ActionApplyInbound {
		t.Fatalf("action = %q, want %q", action, ActionApplyInbound)
	}

	// Reload and re-sync: must be noop, NOT write-outbound. If judgment_modified_at
	// were bumped, catalogChanged would be true → outbound → oscillator.
	applied, err := repo.Get(ctx, asset.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	action, err = syncer.SyncSidecar(ctx, applied, "testdata/lightroom.xmp")
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if action != ActionNoop {
		t.Errorf("second pass action = %q, want noop (oscillator guard)", action)
	}
}

// TestEchoCheck is acceptance #2: after an outbound write, re-syncing the same
// sidecar must be a noop (the hash matches → sidecar NOT changed → no re-read).
func TestEchoCheck(t *testing.T) {
	status := dependency.Discover(dependency.Exiftool, "")
	if !status.Available() {
		t.Skipf("exiftool not available (%s)", status.State)
	}
	daemon, err := dependency.StartExiftool(status, log.Default())
	if err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer daemon.Close()

	db := testutil.NewTestDB(t)
	source := testutil.NewTestVolume(t, db, "s")
	asset := testutil.NewTestAsset(t, db, source.ID, "sunrise.orf")
	repo := &sqlite.AssetRepo{DB: db}
	ctx := context.Background()

	// Give the asset a rating via judgment writer (bumps judgment_modified_at).
	judgmentPatch := catalog.TriagePatch{Rating: domain.SetOpt(3)}
	if err := repo.ApplyTriagePatch(ctx, []string{asset.ID}, judgmentPatch); err != nil {
		t.Fatalf("apply judgment: %v", err)
	}

	syncer := NewSyncer(daemon, repo, repo, nil, settings.DefaultSettings, log.Default())

	// Write outbound directly (the debounce path).
	sidecarPath := t.TempDir() + "/sunrise.xmp"
	judged, err := repo.Get(ctx, asset.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if err := syncer.writeOutbound(ctx, judged, sidecarPath); err != nil {
		t.Fatalf("writeOutbound: %v", err)
	}

	// Reload asset (now has xmp_hash + xmp_last_written_at from writeOutbound).
	written, err := repo.Get(ctx, asset.ID)
	if err != nil {
		t.Fatalf("reload after write: %v", err)
	}
	if written.XMPHash == nil {
		t.Fatal("xmp_hash not stored after outbound write")
	}

	// Simulate the watcher hint: SyncSidecar on the file we just wrote. The
	// hash matches xmp_hash → sidecar NOT changed → noop (echo dropped).
	action, err := syncer.SyncSidecar(ctx, written, sidecarPath)
	if err != nil {
		t.Fatalf("echo sync: %v", err)
	}
	if action != ActionNoop {
		t.Errorf("echo action = %q, want noop (our own write should be dropped)", action)
	}
}

// TestOutboundMergePreservation is acceptance #6: writing our fields into an
// LrC sidecar with develop settings must leave the crs: namespace intact.
func TestOutboundMergePreservation(t *testing.T) {
	status := dependency.Discover(dependency.Exiftool, "")
	if !status.Available() {
		t.Skipf("exiftool not available (%s)", status.State)
	}
	daemon, err := dependency.StartExiftool(status, log.Default())
	if err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer daemon.Close()

	// Copy the LrC fixture to a temp dir so we can write to it.
	original, err := os.ReadFile("testdata/lightroom.xmp")
	if err != nil {
		t.Fatal(err)
	}
	sidecarPath := filepath.Join(t.TempDir(), "lightroom.xmp")
	if err := os.WriteFile(sidecarPath, original, 0o600); err != nil { //nolint:gosec // test file in t.TempDir
		t.Fatal(err)
	}

	rating := 2
	label := domain.ColorLabelBlue
	err = Write(context.Background(), daemon, sidecarPath, &WriteFields{
		Rating:     &rating,
		ColorLabel: &label,
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read back and verify our fields changed.
	fields, err := Read(context.Background(), daemon, sidecarPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if fields.Rating == nil || *fields.Rating != 2 {
		t.Errorf("rating = %v, want 2", fields.Rating)
	}
	if fields.Label != "Blue" {
		t.Errorf("label = %q, want Blue", fields.Label)
	}

	// Verify the crs: develop settings survived the merge.
	after, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	afterStr := string(after)
	for _, marker := range []string{"crs:Exposure2012", "crs:Temperature", "crs:ProcessVersion"} {
		if !strings.Contains(afterStr, marker) {
			t.Errorf("merge lost %s — foreign namespace not preserved", marker)
		}
	}
}

// TestConflictResolution is acceptance #4: both sides changed → policy applied;
// tags unioned regardless. Two sub-tests: xmp_wins (inbound) and catalog_wins
// (outbound).
func TestConflictResolution(t *testing.T) {
	status := dependency.Discover(dependency.Exiftool, "")
	if !status.Available() {
		t.Skipf("exiftool not available (%s)", status.State)
	}
	daemon, err := dependency.StartExiftool(status, log.Default())
	if err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer daemon.Close()

	setup := func(t *testing.T) (*sqlite.AssetRepo, *sqlite.Store, *domain.Asset, string) {
		t.Helper()
		db := testutil.NewTestDB(t)
		source := testutil.NewTestVolume(t, db, "s")
		asset := testutil.NewTestAsset(t, db, source.ID, "sunrise.orf")
		repo := &sqlite.AssetRepo{DB: db}
		store := sqlite.NewStore(db)
		ctx := context.Background()

		// Step 1: inbound sync to establish the baseline (sets xmp_hash + xmp_last_read_at).
		baseline := NewSyncer(daemon, repo, repo, store, settings.DefaultSettings, log.Default())
		if _, err := baseline.SyncSidecar(ctx, asset, "testdata/lightroom.xmp"); err != nil {
			t.Fatalf("baseline sync: %v", err)
		}

		// Step 2: user edits the catalog (bumps judgment_modified_at → catalog changed).
		// ponytail: timestamps are RFC3339 (second precision) so we need a full second
		// gap for judgment_modified_at to be strictly After xmp_last_read_at.
		time.Sleep(time.Second)
		if err := repo.ApplyTriagePatch(ctx, []string{asset.ID}, catalog.TriagePatch{
			Rating: domain.SetOpt(1),
		}); err != nil {
			t.Fatalf("user judgment: %v", err)
		}

		// Step 3: prepare a DIFFERENT sidecar (sidecar changed).
		sidecarPath := filepath.Join(t.TempDir(), "sunrise.xmp")
		original, _ := os.ReadFile("testdata/no-rating.xmp")
		if err := os.WriteFile(sidecarPath, original, 0o600); err != nil { //nolint:gosec // test file in t.TempDir
			t.Fatal(err)
		}

		synced, _ := repo.Get(ctx, asset.ID)
		return repo, store, synced, sidecarPath
	}

	t.Run("xmp_wins", func(t *testing.T) {
		repo, store, asset, sidecarPath := setup(t)
		ctx := context.Background()

		syncer := NewSyncer(daemon, repo, repo, store, settings.DefaultSettings, log.Default())
		action, err := syncer.SyncSidecar(ctx, asset, sidecarPath)
		if err != nil {
			t.Fatalf("conflict sync: %v", err)
		}
		if action != ActionConflict {
			t.Fatalf("action = %q, want conflict", action)
		}

		// xmp_wins: sidecar's values applied inbound (no-rating.xmp has no rating → cleared).
		resolved, _ := repo.Get(ctx, asset.ID)
		if resolved.Rating != nil {
			t.Errorf("Rating = %v, want nil (xmp_wins cleared it)", *resolved.Rating)
		}
		// judgment_modified_at must NOT be bumped by the inbound apply.
		if resolved.JudgmentModifiedAt == nil || !resolved.JudgmentModifiedAt.Equal(*asset.JudgmentModifiedAt) {
			t.Errorf("judgment_modified_at changed during conflict resolution — oscillator risk")
		}
	})

	t.Run("catalog_wins", func(t *testing.T) {
		repo, store, asset, sidecarPath := setup(t)
		ctx := context.Background()

		catalogWins := func() settings.Settings {
			s := settings.DefaultSettings()
			s.XMPWriteBack = true
			s.XMPConflictResolution = string(PolicyCatalogWins)
			return s
		}
		syncer := NewSyncer(daemon, repo, repo, store, catalogWins, log.Default())
		action, err := syncer.SyncSidecar(ctx, asset, sidecarPath)
		if err != nil {
			t.Fatalf("conflict sync: %v", err)
		}
		if action != ActionConflict {
			t.Fatalf("action = %q, want conflict", action)
		}

		// catalog_wins: catalog's rating=1 pushed outbound; catalog value preserved.
		resolved, _ := repo.Get(ctx, asset.ID)
		if resolved.Rating == nil || *resolved.Rating != 1 {
			t.Errorf("Rating = %v, want 1 (catalog_wins preserves user judgment)", resolved.Rating)
		}

		// The sidecar should now carry the catalog's rating.
		fields, err := Read(ctx, daemon, sidecarPath)
		if err != nil {
			t.Fatalf("read back sidecar: %v", err)
		}
		if fields.Rating == nil || *fields.Rating != 1 {
			t.Errorf("sidecar rating = %v, want 1 (catalog_wins wrote outbound)", fields.Rating)
		}
	})

	t.Run("tags_union_regardless", func(t *testing.T) {
		repo, store, asset, sidecarPath := setup(t)
		ctx := context.Background()

		// Use catalog_wins so the JUDGMENT verdict is outbound, but tags should
		// still union inbound (the documented exception).
		catalogWins := func() settings.Settings {
			s := settings.DefaultSettings()
			s.XMPWriteBack = true
			s.XMPConflictResolution = string(PolicyCatalogWins)
			return s
		}

		// Write a sidecar that has keywords (copy the full LrC fixture instead).
		lrcFixture, _ := os.ReadFile("testdata/lightroom.xmp")
		if err := os.WriteFile(sidecarPath, lrcFixture, 0o600); err != nil { //nolint:gosec // test file in t.TempDir
			t.Fatal(err)
		}

		syncer := NewSyncer(daemon, repo, repo, store, catalogWins, log.Default())
		if _, err := syncer.SyncSidecar(ctx, asset, sidecarPath); err != nil {
			t.Fatalf("conflict sync: %v", err)
		}

		// Tags should have been unioned even though the judgment policy is catalog_wins.
		var count int
		if err := repo.DB.QueryRowContext(ctx,
			"SELECT count(*) FROM asset_tags WHERE asset_id = ? AND removed_at IS NULL",
			asset.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Error("no tags attached — tags should union regardless of conflict policy")
		}
	})
}

// TestBuildWriteArgs covers the pure argument builder.
func TestBuildWriteArgs(t *testing.T) {
	rating := 3
	label := domain.ColorLabelGreen
	args := buildWriteArgs(&WriteFields{
		Rating:       &rating,
		ColorLabel:   &label,
		Tags:         []string{"travel", "japan"},
		Hierarchical: []string{"Travel|Japan|Tokyo"},
		Caption:      "a caption",
		Title:        "a title",
	}, "/tmp/test.xmp", "/tmp/test.xmp.alxtmp")

	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-Rating=3",
		"-Label=Green",
		"-Subject=travel", "-Subject+=japan",
		"-HierarchicalSubject=Travel|Japan|Tokyo",
		"-Description=a caption",
		"-Title=a title",
		"-overwrite_original",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q in: %s", want, joined)
		}
	}

	// Empty fields → clear args.
	clearArgs := buildWriteArgs(&WriteFields{}, "/tmp/test.xmp", "/tmp/test.xmp.alxtmp")
	clearJoined := strings.Join(clearArgs, " ")
	for _, want := range []string{"-Rating=", "-Label=", "-Subject=", "-Description=", "-Title="} {
		// Must have the clear form (tag=) without a value after it.
		if !strings.Contains(clearJoined, want) {
			t.Errorf("clear args missing %q", want)
		}
	}
}

// TestAssetTagNames verifies the tag read-back for outbound writes.
func TestAssetTagNames(t *testing.T) {
	db := testutil.NewTestDB(t)
	source := testutil.NewTestVolume(t, db, "s")
	asset := testutil.NewTestAsset(t, db, source.ID, "test.jpg")
	store := sqlite.NewStore(db)
	ctx := context.Background()

	// Import a hierarchy: Travel>Japan>Tokyo (leaf=Tokyo attached).
	if err := store.ImportKeywords(ctx, asset.ID, []string{"standalone"}, [][]string{{"Travel", "Japan", "Tokyo"}}, "xmp"); err != nil {
		t.Fatalf("import: %v", err)
	}

	tagRepo := &sqlite.TagRepo{DB: db}
	flat, hierarchical, err := tagRepo.AssetTagNames(ctx, asset.ID)
	if err != nil {
		t.Fatalf("AssetTagNames: %v", err)
	}

	// Flat should include all attached tag names.
	if len(flat) != 2 {
		t.Errorf("flat = %v, want 2 names (Tokyo + standalone)", flat)
	}

	// Hierarchical should include the path for Tokyo (which has a parent).
	if len(hierarchical) != 1 {
		t.Fatalf("hierarchical = %v, want 1 path", hierarchical)
	}
	if hierarchical[0] != "Travel|Japan|Tokyo" {
		t.Errorf("hierarchical[0] = %q, want Travel|Japan|Tokyo", hierarchical[0])
	}
}

func mustTime(s string) time.Time {
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return parsed
}
