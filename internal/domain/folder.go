package domain

import "time"

// Folder is the tracked-root half of the D24 split: a directory the catalog
// walks and watches, plus its sync scope. Path is volume-relative (relative to
// the volume's mount point) so it survives a remount; ("" means the whole
// volume root). Tracked roots on one volume are DISJOINT by invariant (D41:
// adding a subfolder redirects to the existing root; adding a parent absorbs).
//
// Mixed writer classes, like Volume (D41): Name/SyncMode/ScanRecursively/
// Enabled/PollIntervalSecs are judgments; LastScannedAt is sync-state.
type Folder struct {
	ID               string
	VolumeID         string   // FK → Volume
	Path             string   // [jdg] volume-relative; "" = volume root
	Name             string   // [jdg]
	SyncMode         SyncMode // [jdg]
	ScanRecursively  bool     // [jdg]
	Enabled          bool     // [jdg] user activates/deactivates
	PollIntervalSecs *int     // [jdg]
	LastScannedAt    *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
