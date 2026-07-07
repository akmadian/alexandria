package xmp

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/dependency"
	"github.com/akmadian/alexandria/internal/domain"
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
	source := testutil.NewTestSource(t, db, "s")
	asset := testutil.NewTestAsset(t, db, source.ID, "sunrise.orf")
	repo := &sqlite.AssetRepo{DB: db}

	syncer := NewSyncer(daemon, repo, PolicyXMPWins, log.Default())
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
	source := testutil.NewTestSource(t, db, "s")
	asset := testutil.NewTestAsset(t, db, source.ID, "sunrise.orf")
	repo := &sqlite.AssetRepo{DB: db}
	syncer := NewSyncer(daemon, repo, PolicyXMPWins, log.Default())
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

// TestToTriagePatch checks the wholesale judgment mapping: present fields set,
// ABSENT fields clear (sidecar wins, upholding the conflict policy), out-of-range
// rating clears, unknown/empty label clears.
func TestToTriagePatch(t *testing.T) {
	syncer := NewSyncer(nil, nil, PolicyXMPWins, log.Default())
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
		patch := syncer.toTriagePatch(tc.fields, "asset-x")
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

func mustTime(s string) time.Time {
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return parsed
}
