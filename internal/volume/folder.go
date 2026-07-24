package volume

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/charmbracelet/log"
)

// The four graceful-merge outcome kinds (D41: reject nothing; roots stay
// disjoint) are the shared vocabulary domain.CreateFolderOutcomeKind — declared
// once, generated to the wire (C15) — not an engine-local enum:
//
//   - created: a brand-new tracked root was created.
//   - already_tracked_within: the path is at or under an existing root (exact
//     duplicate resolves to that root itself). Nothing changes.
//   - absorbed: the path is a parent of existing roots; they fold into the new
//     wider root. Performed QUIETLY (no confirmation) when no sync behavior
//     changes. Assets never move — they are already volume-relative — so there
//     is zero identity churn (D20).
//   - needs_confirmation: absorbing would change a watched/scheduled child's
//     sync behavior. NOTHING mutates; the caller re-calls with confirm=true to
//     proceed as absorbed.

// Outcome is the CreateFolder result union. Only the fields relevant to Kind are
// populated (Go has no sum types; Kind is the discriminant).
type Outcome struct {
	Kind domain.CreateFolderOutcomeKind
	// FolderID is the created/absorbing root (created, absorbed).
	FolderID string
	// ExistingFolderID is the tracking root the path already sits within
	// (already_tracked_within).
	ExistingFolderID string
	// ReplacedFolderIDs are the child roots folded into the new root
	// (absorbed, needs_confirmation).
	ReplacedFolderIDs []string
	// BehaviorChanges are the subset of ReplacedFolderIDs whose watched/scheduled
	// sync behavior would change under the new parent (needs_confirmation).
	BehaviorChanges []string
}

// Manager owns folder-add: the disjoint-roots invariant and the four-outcome
// graceful merge (D41). It resolves the path to a volume through the Resolver,
// then reconciles against the volume's existing tracked roots.
type Manager struct {
	Resolver *Resolver
	Folders  catalog.FolderRepository
	Log      *log.Logger
}

// NewManager builds a folder-add manager.
func NewManager(resolver *Resolver, folders catalog.FolderRepository, logger *log.Logger) *Manager {
	if logger == nil {
		logger = log.Default()
	}
	return &Manager{Resolver: resolver, Folders: folders, Log: logger}
}

// CreateFolder adds (or reconciles) a tracked root at absolutePath with the given
// sync mode. It never returns a bare overlap error — it returns one of the four
// D41 outcomes. A parent-of-existing add absorbs quietly unless a watched/
// scheduled child's behavior would change under the new mode, in which case it
// returns needs_confirmation without mutating; a second call with confirm=true
// proceeds as absorbed.
func (m *Manager) CreateFolder(ctx context.Context, absolutePath string, syncMode domain.SyncMode, confirm bool) (Outcome, error) {
	resolved, err := m.Resolver.Resolve(ctx, absolutePath)
	if err != nil {
		return Outcome{}, err
	}
	existing, err := m.Folders.ListByVolume(ctx, resolved.VolumeID)
	if err != nil {
		return Outcome{}, fmt.Errorf("list folders on volume %s: %w", resolved.VolumeID, err)
	}
	newPath := resolved.RelativePath

	// (1) Already tracked: the path sits at or under an existing root.
	for _, folder := range existing {
		if pathContains(folder.Path, newPath) {
			m.Log.Info("folder-add outcome", "outcome", domain.CreateFolderAlreadyTrackedWithin,
				"path", absolutePath, "within", folder.ID)
			return Outcome{Kind: domain.CreateFolderAlreadyTrackedWithin, ExistingFolderID: folder.ID}, nil
		}
	}

	// (2) Children to absorb: existing roots strictly under the new path.
	var children []*domain.Folder
	for _, folder := range existing {
		if folder.Path != newPath && pathContains(newPath, folder.Path) {
			children = append(children, folder)
		}
	}

	if len(children) == 0 {
		folder, err := m.create(ctx, resolved.VolumeID, newPath, absolutePath, syncMode)
		if err != nil {
			return Outcome{}, err
		}
		m.Log.Info("folder-add outcome", "outcome", domain.CreateFolderCreated, "path", absolutePath, "folder", folder.ID)
		return Outcome{Kind: domain.CreateFolderCreated, FolderID: folder.ID}, nil
	}

	// (3) Absorb — quietly unless a watched/scheduled child's behavior changes.
	replaced := make([]string, 0, len(children))
	var behaviorChanges []string
	for _, child := range children {
		replaced = append(replaced, child.ID)
		if child.SyncMode != domain.SyncModeManual && child.SyncMode != syncMode {
			behaviorChanges = append(behaviorChanges, child.ID)
		}
	}
	if len(behaviorChanges) > 0 && !confirm {
		m.Log.Info("folder-add outcome", "outcome", domain.CreateFolderNeedsConfirmation,
			"path", absolutePath, "replaces", replaced, "behaviorChanges", behaviorChanges)
		return Outcome{Kind: domain.CreateFolderNeedsConfirmation, ReplacedFolderIDs: replaced, BehaviorChanges: behaviorChanges}, nil
	}

	// Create the wider root first, then drop the children it now covers — so a
	// crash mid-absorb never leaves the subtree untracked (assets never move, so
	// there is no identity churn either way, D20).
	folder, err := m.create(ctx, resolved.VolumeID, newPath, absolutePath, syncMode)
	if err != nil {
		return Outcome{}, err
	}
	for _, child := range children {
		m.Log.Debug("absorbing child root", "child", child.ID, "childPath", child.Path, "into", folder.ID)
		if err := m.Folders.Delete(ctx, child.ID); err != nil {
			return Outcome{}, fmt.Errorf("absorb child folder %s: %w", child.ID, err)
		}
	}
	m.Log.Info("folder-add outcome", "outcome", domain.CreateFolderAbsorbed, "path", absolutePath,
		"folder", folder.ID, "absorbed", replaced)
	return Outcome{Kind: domain.CreateFolderAbsorbed, FolderID: folder.ID, ReplacedFolderIDs: replaced}, nil
}

func (m *Manager) create(ctx context.Context, volumeID, relativePath, absolutePath string, syncMode domain.SyncMode) (*domain.Folder, error) {
	now := time.Now().UTC()
	folder := &domain.Folder{
		ID:              domain.NewID(),
		VolumeID:        volumeID,
		Path:            relativePath,
		Name:            folderName(absolutePath, relativePath),
		SyncMode:        syncMode,
		ScanRecursively: true,
		Enabled:         true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := m.Folders.Create(ctx, folder); err != nil {
		return nil, fmt.Errorf("create folder: %w", err)
	}
	return folder, nil
}

// pathContains reports whether descendant is at or under ancestor, in volume-
// relative path space. "" is the volume root, which contains everything.
func pathContains(ancestor, descendant string) bool {
	if ancestor == descendant {
		return true
	}
	if ancestor == "" {
		return true
	}
	return strings.HasPrefix(descendant, ancestor+"/")
}

// folderName derives a display name for a new root: the path's basename, or the
// absolute path's basename at the volume root.
func folderName(absolutePath, relativePath string) string {
	if relativePath == "" {
		if base := filepath.Base(absolutePath); base != "" && base != "." {
			return base
		}
		return "Volume Root"
	}
	return filepath.Base(relativePath)
}
