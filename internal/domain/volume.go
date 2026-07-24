package domain

import (
	"fmt"
	"time"
)

// VolumeKind classifies the storage a volume lives on. It survives the D24 split
// unchanged from the old SourceKind — the identity/portability anchor keeps the
// storage-class vocabulary; the tracked-root (Folder) carries none.
type VolumeKind string

const (
	VolumeKindLocal         VolumeKind = "local"
	VolumeKindExternalDrive VolumeKind = "external_drive"
	VolumeKindSMB           VolumeKind = "smb"
	VolumeKindNFS           VolumeKind = "nfs"
)

// VolumeConnectivity is the observed reachability of a volume's storage. It is an
// observation (the volume monitor / reconciler write it), distinct from a
// Folder's Enabled judgment. A yanked drive flips this to offline while the
// catalog stays fully browsable (D41: offline is a visual state, never a gate).
type VolumeConnectivity string

const (
	VolumeOnline  VolumeConnectivity = "online"
	VolumeOffline VolumeConnectivity = "offline"
)

// Volume is the identity/portability anchor of the D24 split: one physical
// storage location, matched by filesystem UUID rather than mount path (a drive
// can mount at a different path each session). Many Folders may live on one
// Volume; assets and sidecars key on (volume_id, volume-relative path). The
// current mount point is NOT stored — it is resolved live by the volume prober
// (internal/volume) — so a remount rewrites nothing.
//
// The split principle is identity vs. tracking scope, NOT writer class (D41):
// this table carries mixed classes — Name/Host/ShareName are judgments,
// FilesystemUUID/DiskSerial/VolumeLabel/Connectivity are observations. Column
// enforcement stays per-column via the catalog writer interfaces.
type Volume struct {
	ID             string
	Name           string             // [jdg]
	Kind           VolumeKind         // [jdg]
	Host           *string            // [jdg] SMB/NFS host
	ShareName      *string            // [jdg] SMB/NFS share
	FilesystemUUID *string            // [obs] identity key: real fs UUID, "smb://host/share"/"nfs://host/export", or residual "dev:N" (exotic fs only)
	DiskSerial     *string            // [obs]
	VolumeLabel    *string            // [obs]
	Connectivity   VolumeConnectivity // [obs] mount monitor / reconciler
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// VolumeOfflineError signals a volume whose storage is not currently reachable.
type VolumeOfflineError struct {
	VolumeID string
	Path     string
}

func (e *VolumeOfflineError) Error() string {
	return fmt.Sprintf("volume %s is offline (path: %s)", e.VolumeID, e.Path)
}
