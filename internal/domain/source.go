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

type SourceStatus string

const (
	SourceStatusActive  SourceStatus = "active"
	SourceStatusOffline SourceStatus = "offline"
	SourceStatusRemoved SourceStatus = "removed"
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
	Status           SourceStatus
	LastScannedAt    *time.Time
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
