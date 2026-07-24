package seam

import "github.com/akmadian/alexandria/internal/domain"

// The browser-rail wire projections (D41, §12) — the top navigation axis the
// rail renders: the volume/folder forest and the collection list. The json tags
// ARE the wire contract (C13/C15): cmd/generate reflects these structs into the
// generated TS models, so contract.ts never hand-authors a parallel shape. These
// are read-only display projections; the engine's own aggregates (volumes,
// folders, collections tables) stay behind the services.

// VolumeNode is a storage volume (a portability anchor — a physical drive/share)
// with its tracked-root folders. `connectivity` is an OBSERVATION, never a gate:
// an offline volume is dimmed but stays fully browsable (D41), so the rail marks
// it, never disables it. `assetCount` is the volume's total across its folders —
// always available (the deriver sums it in one pass), so a plain count, not the
// nullable form CollectionNode carries.
type VolumeNode struct {
	ID           string                    `json:"id"`
	Name         string                    `json:"name"`
	Kind         domain.VolumeKind         `json:"kind"`
	Connectivity domain.VolumeConnectivity `json:"connectivity"`
	AssetCount   int                       `json:"assetCount"`
	Folders      []FolderNode              `json:"folders"`
}

// FolderNode is one node in a volume's folder tree. `id` is the folder row ID for
// a tracked root; for a derived path node the deriver synthesizes it as
// volumeID + ":" + path (stable per volume, so the rail can key rows without a
// row of its own). `syncMode` rides on tracked ROOTS only (nil on derived nodes,
// which inherit their root's mode) — hence the omitempty pointer. `assetCount`
// is the recursive subtree count (scope defaults recursive, D41): a parent's
// count is its own direct assets plus every descendant's, so counts sum at every
// level. Always available (folders are counted in the deriver's one pass), so a
// plain count.
type FolderNode struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Path       string           `json:"path"`
	SyncMode   *domain.SyncMode `json:"syncMode,omitempty"`
	AssetCount int              `json:"assetCount"`
	Children   []FolderNode     `json:"children"`
}

// CollectionNode is a collection projected for the rail. `kind` is the domain
// enum (C15). `parentId` is nil for a top-level collection (collections nest by
// adjacency, so the rail gets a FLAT list and builds the forest from parentId,
// unlike the volume tree which arrives nested). `assetCount` is a nullable
// pointer with a load-bearing three-state reading (D41): nil = the count is
// UNAVAILABLE (the backend's declared retreat for a pathological smart query it
// declined to compute), 0 = the collection is genuinely EMPTY. A manual
// collection counts its direct membership; a smart one carries a real compiled
// COUNT, with nil reserved as the escape hatch.
type CollectionNode struct {
	ID         string                `json:"id"`
	Name       string                `json:"name"`
	ParentID   *string               `json:"parentId,omitempty"`
	Kind       domain.CollectionKind `json:"kind"`
	AssetCount *int                  `json:"assetCount"`
}

// CreateFolderOutcome is what createFolder returns — the disposition of a
// folder-add attempt plus the folders it touched (D41). The `kind` discriminates
// the four outcomes (domain.CreateFolderOutcomeKind):
//
//   - created: `folderId` is the new root.
//   - already_tracked_within: `folderId` is the existing root redirected to.
//   - absorbed: `folderId` is the new parent root; `absorbedFolderIds` are the
//     roots it took in.
//   - needs_confirmation: NO mutation happened; `absorbedFolderIds` are the roots
//     that WOULD be absorbed and `behaviorChanges` the sync-mode shifts to show
//     the user before they re-issue createFolder with confirm=true.
type CreateFolderOutcome struct {
	Kind              domain.CreateFolderOutcomeKind `json:"kind"`
	FolderID          string                         `json:"folderId,omitempty"`
	AbsorbedFolderIDs []string                       `json:"absorbedFolderIds,omitempty"`
	BehaviorChanges   []FolderBehaviorChange         `json:"behaviorChanges,omitempty"`
}

// FolderBehaviorChange describes one tracked root whose sync policy would change
// under a proposed absorb — the payload the needs_confirmation dialog renders so
// the user sees exactly what changes (D41's "asks first ONLY when the
// combination would change behavior").
type FolderBehaviorChange struct {
	FolderID        string          `json:"folderId"`
	FolderName      string          `json:"folderName"`
	CurrentSyncMode domain.SyncMode `json:"currentSyncMode"`
	NewSyncMode     domain.SyncMode `json:"newSyncMode"`
}

// FolderPatch is the sparse update for a tracked root (updateFolder). Every field
// is an omitempty pointer: an absent field leaves its value untouched
// (read-modify-write), matching CollectionPatch's shape (C7's closed
// optional-field patch). Path is identity and never patched; only the display
// name and the sync policy are mutable.
type FolderPatch struct {
	Name     *string          `json:"name,omitempty"`
	SyncMode *domain.SyncMode `json:"syncMode,omitempty"`
}
