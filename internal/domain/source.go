package domain

import (
	"fmt"
	"time"
)

type SourceKind string

const (
	SourceKindLocal         SourceKind = "local"
	SourceKindExternalDrive SourceKind = "external_drive"
	SourceKindSMB           SourceKind = "smb"
	SourceKindNFS           SourceKind = "nfs"
)

// SourceConnectivity is the observed reachability of a source's storage. It is an
// observation (the volume monitor / reconciler write it), distinct from Enabled,
// which is the user's judgment about whether to watch the source at all. The old
// single `status` enum conflated the two; splitting them lets a user disable a
// source and the watcher report a dead mount without fighting over one column.
type SourceConnectivity string

const (
	SourceOnline  SourceConnectivity = "online"
	SourceOffline SourceConnectivity = "offline"
)

type Source struct {
	ID               string
	Name             string
	Kind             SourceKind
	BasePath         string
	FilesystemUUID   *string
	DiskSerial       *string
	VolumeLabel      *string
	Host             *string
	ShareName        *string
	PollIntervalSecs *int
	ScanRecursively  bool
	Enabled          bool               // judgment: user activates/deactivates
	Connectivity     SourceConnectivity // observation: mount monitor / reconciler
	LastScannedAt    *time.Time         // sync-state
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type SourceOfflineError struct {
	SourceID string
	Path     string
}

func (e *SourceOfflineError) Error() string {
	return fmt.Sprintf("source %s is offline (path: %s)", e.SourceID, e.Path)
}
