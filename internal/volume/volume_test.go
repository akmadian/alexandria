package volume_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/charmbracelet/log"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
	"github.com/akmadian/alexandria/internal/volume"
)

// oneFilesystemProber reports a single mount point and identity for every path —
// modeling one physical filesystem, so every path under mount resolves to the
// same volume.
type oneFilesystemProber struct {
	mount string
	uuid  string
}

func (p oneFilesystemProber) Probe(context.Context, string) (volume.Probe, error) {
	return volume.Probe{MountPoint: p.mount, FilesystemUUID: p.uuid}, nil
}

func newResolver(t *testing.T) (*volume.Resolver, *sqlite.VolumeRepo) {
	t.Helper()
	db := testutil.NewTestDB(t)
	volumes := &sqlite.VolumeRepo{DB: db}
	resolver := volume.NewResolver(volumes, oneFilesystemProber{mount: "/mnt/drive", uuid: "drive-uuid"}, log.New(io.Discard))
	return resolver, volumes
}

// TestResolve_FindOrCreate_TwoFoldersOneVolume is the acceptance case: two paths
// on one filesystem yield ONE volume row, and each keeps its own volume-relative
// path.
func TestResolve_FindOrCreate_TwoFoldersOneVolume(t *testing.T) {
	resolver, volumes := newResolver(t)
	ctx := context.Background()

	first, err := resolver.Resolve(ctx, "/mnt/drive/Photos")
	if err != nil {
		t.Fatalf("resolve first: %v", err)
	}
	if first.RelativePath != "Photos" {
		t.Fatalf("relative path = %q, want Photos", first.RelativePath)
	}

	second, err := resolver.Resolve(ctx, "/mnt/drive/Archive/2023")
	if err != nil {
		t.Fatalf("resolve second: %v", err)
	}
	if second.RelativePath != "Archive/2023" {
		t.Fatalf("relative path = %q, want Archive/2023", second.RelativePath)
	}

	if first.VolumeID != second.VolumeID {
		t.Fatalf("two folders on one filesystem got two volumes: %s vs %s", first.VolumeID, second.VolumeID)
	}
	all, err := volumes.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("volume rows = %d, want exactly 1", len(all))
	}

	// The mount point round-trips (Absolute is the inverse of Resolve).
	absolute, err := resolver.Absolute(ctx, first.VolumeID, "Photos/a.jpg")
	if err != nil {
		t.Fatalf("absolute: %v", err)
	}
	if absolute != "/mnt/drive/Photos/a.jpg" {
		t.Fatalf("absolute = %q, want /mnt/drive/Photos/a.jpg", absolute)
	}
}

func newManager(t *testing.T) (*volume.Manager, *sqlite.FolderRepo) {
	t.Helper()
	db := testutil.NewTestDB(t)
	resolver := volume.NewResolver(&sqlite.VolumeRepo{DB: db},
		oneFilesystemProber{mount: "/mnt/drive", uuid: "drive-uuid"}, log.New(io.Discard))
	folders := &sqlite.FolderRepo{DB: db}
	return volume.NewManager(resolver, folders, log.New(io.Discard)), folders
}

// TestCreateFolder_Created covers a brand-new tracked root.
func TestCreateFolder_Created(t *testing.T) {
	manager, folders := newManager(t)
	ctx := context.Background()

	outcome, err := manager.CreateFolder(ctx, "/mnt/drive/Photos", domain.SyncModeManual, false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if outcome.Kind != domain.CreateFolderCreated {
		t.Fatalf("outcome = %q, want created", outcome.Kind)
	}
	all, _ := folders.List(ctx)
	if len(all) != 1 || all[0].Path != "Photos" {
		t.Fatalf("folders = %+v, want one at Photos", all)
	}
}

// TestCreateFolder_AlreadyTrackedWithin covers a subfolder of a tracked root and
// an exact duplicate — both resolve to the existing root, nothing changes.
func TestCreateFolder_AlreadyTrackedWithin(t *testing.T) {
	manager, folders := newManager(t)
	ctx := context.Background()

	root, _ := manager.CreateFolder(ctx, "/mnt/drive/Photos", domain.SyncModeManual, false)

	sub, err := manager.CreateFolder(ctx, "/mnt/drive/Photos/2024", domain.SyncModeManual, false)
	if err != nil {
		t.Fatalf("subfolder: %v", err)
	}
	if sub.Kind != domain.CreateFolderAlreadyTrackedWithin || sub.ExistingFolderID != root.FolderID {
		t.Fatalf("subfolder outcome = %+v, want alreadyTrackedWithin %s", sub, root.FolderID)
	}

	dup, err := manager.CreateFolder(ctx, "/mnt/drive/Photos", domain.SyncModeManual, false)
	if err != nil {
		t.Fatalf("exact dup: %v", err)
	}
	if dup.Kind != domain.CreateFolderAlreadyTrackedWithin || dup.ExistingFolderID != root.FolderID {
		t.Fatalf("exact-dup outcome = %+v, want alreadyTrackedWithin %s", dup, root.FolderID)
	}

	if all, _ := folders.List(ctx); len(all) != 1 {
		t.Fatalf("folders = %d, want still 1 (disjoint invariant)", len(all))
	}
}

// TestCreateFolder_AbsorbedQuietly covers a parent-of-existing add with uniform
// sync behavior: the child roots fold in without confirmation.
func TestCreateFolder_AbsorbedQuietly(t *testing.T) {
	manager, folders := newManager(t)
	ctx := context.Background()

	child, _ := manager.CreateFolder(ctx, "/mnt/drive/Archive/2023", domain.SyncModeManual, false)
	other, _ := manager.CreateFolder(ctx, "/mnt/drive/Archive/2022", domain.SyncModeManual, false)

	outcome, err := manager.CreateFolder(ctx, "/mnt/drive/Archive", domain.SyncModeManual, false)
	if err != nil {
		t.Fatalf("absorb: %v", err)
	}
	if outcome.Kind != domain.CreateFolderAbsorbed {
		t.Fatalf("outcome = %q, want absorbed", outcome.Kind)
	}
	if len(outcome.ReplacedFolderIDs) != 2 {
		t.Fatalf("replaced = %v, want the two children", outcome.ReplacedFolderIDs)
	}
	// Disjointness holds: only the new wider root survives.
	all, _ := folders.List(ctx)
	if len(all) != 1 || all[0].Path != "Archive" || all[0].ID != outcome.FolderID {
		t.Fatalf("after absorb folders = %+v, want only the Archive root", all)
	}
	// The children are gone (absorbed), not lingering.
	for _, gone := range []string{child.FolderID, other.FolderID} {
		if _, err := folders.Get(ctx, gone); err == nil {
			t.Fatalf("absorbed child %s still exists", gone)
		}
	}
}

// TestCreateFolder_NeedsConfirmationThenAbsorbs covers the behavior-change guard:
// a watched child under a manual parent needs confirmation (nothing mutates),
// then absorbs on the confirmed re-call.
func TestCreateFolder_NeedsConfirmationThenAbsorbs(t *testing.T) {
	manager, folders := newManager(t)
	ctx := context.Background()

	watchedChild, _ := manager.CreateFolder(ctx, "/mnt/drive/Watched/sub", domain.SyncModeWatched, false)

	// A manual parent would change the watched child's sync behavior → confirm.
	pending, err := manager.CreateFolder(ctx, "/mnt/drive/Watched", domain.SyncModeManual, false)
	if err != nil {
		t.Fatalf("needs-confirmation: %v", err)
	}
	if pending.Kind != domain.CreateFolderNeedsConfirmation {
		t.Fatalf("outcome = %q, want needsConfirmation", pending.Kind)
	}
	if len(pending.BehaviorChanges) != 1 || pending.BehaviorChanges[0] != watchedChild.FolderID {
		t.Fatalf("behaviorChanges = %v, want the watched child", pending.BehaviorChanges)
	}
	// NOTHING mutated: the child root is untouched, no parent created.
	all, _ := folders.List(ctx)
	if len(all) != 1 || all[0].ID != watchedChild.FolderID {
		t.Fatalf("needs-confirmation mutated the catalog: %+v", all)
	}

	// The confirmed re-call proceeds as absorbed.
	confirmed, err := manager.CreateFolder(ctx, "/mnt/drive/Watched", domain.SyncModeManual, true)
	if err != nil {
		t.Fatalf("confirmed: %v", err)
	}
	if confirmed.Kind != domain.CreateFolderAbsorbed {
		t.Fatalf("confirmed outcome = %q, want absorbed", confirmed.Kind)
	}
	all, _ = folders.List(ctx)
	if len(all) != 1 || all[0].Path != "Watched" {
		t.Fatalf("after confirm folders = %+v, want only the Watched root", all)
	}
}

// racingVolumes simulates the find-or-create race deterministically: the first
// FindByFilesystemUUID misses (both racers passed the lookup), Create fails with
// a unique violation (the other racer won), and the RE-RUN Find returns the
// winner's row. This is the loser's exact view of the interleaving.
type racingVolumes struct {
	winner    *domain.Volume
	findCalls int
}

func (r *racingVolumes) FindByFilesystemUUID(context.Context, string) (*domain.Volume, error) {
	r.findCalls++
	if r.findCalls == 1 {
		return nil, nil // the pre-create miss: the winner hasn't committed yet
	}
	return r.winner, nil // the post-conflict retry sees the winner's row
}

func (r *racingVolumes) Create(context.Context, *domain.Volume) error {
	return errors.New("UNIQUE constraint failed: volumes.filesystem_uuid")
}

func (r *racingVolumes) List(context.Context) ([]*domain.Volume, error)      { panic("unused") }
func (r *racingVolumes) Get(context.Context, string) (*domain.Volume, error) { panic("unused") }
func (r *racingVolumes) Update(context.Context, *domain.Volume) error        { panic("unused") }
func (r *racingVolumes) SetConnectivity(context.Context, string, domain.VolumeConnectivity) error {
	panic("unused")
}

// TestResolve_CreateRaceAdoptsWinner: the loser of a concurrent first-touch
// re-runs the identity lookup and returns the winner's row instead of erroring.
func TestResolve_CreateRaceAdoptsWinner(t *testing.T) {
	winner := &domain.Volume{ID: "vol-winner", Name: "drive"}
	racing := &racingVolumes{winner: winner}
	resolver := volume.NewResolver(racing, oneFilesystemProber{mount: "/mnt/drive", uuid: "drive-uuid"}, log.New(io.Discard))

	resolved, err := resolver.Resolve(context.Background(), "/mnt/drive/Photos")
	if err != nil {
		t.Fatalf("losing the create race must converge, got error: %v", err)
	}
	if resolved.VolumeID != winner.ID {
		t.Fatalf("resolved to %q, want the race winner's row %q", resolved.VolumeID, winner.ID)
	}
}

// TestResolve_ParallelFirstTouch drives real concurrent resolves against the
// real sqlite repo: every goroutine must succeed and exactly one volume row may
// exist afterwards.
func TestResolve_ParallelFirstTouch(t *testing.T) {
	resolver, volumes := newResolver(t)
	ctx := context.Background()

	const racers = 8
	var group sync.WaitGroup
	resolvedIDs := make([]string, racers)
	resolveErrors := make([]error, racers)
	for i := 0; i < racers; i++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			resolved, err := resolver.Resolve(ctx, "/mnt/drive/Photos")
			resolvedIDs[index], resolveErrors[index] = resolved.VolumeID, err
		}(i)
	}
	group.Wait()

	for i := 0; i < racers; i++ {
		if resolveErrors[i] != nil {
			t.Fatalf("racer %d errored: %v", i, resolveErrors[i])
		}
		if resolvedIDs[i] != resolvedIDs[0] {
			t.Fatalf("racers disagree on the volume: %q vs %q", resolvedIDs[i], resolvedIDs[0])
		}
	}
	all, err := volumes.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("parallel first-touch minted %d volume rows, want exactly 1", len(all))
	}
}
