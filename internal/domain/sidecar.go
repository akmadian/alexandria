package domain

import "time"

// SidecarFile is a companion file (XMP, AAE, THM, …) tracked but never treated
// as an asset. Identity is pure filesystem key — (volume, dir, stem) — not the
// minted UUID of an asset: assets carry identity, sidecars follow. The grouping
// engine (a later milestone) fills AttachedAssetID; ingest writes only the
// observation columns.
type SidecarFile struct {
	ID              string
	VolumeID        string
	Dir             string // volume-relative dir, "" = volume root
	Stem            string // lowercase basename sans final ext
	Ext             string // "xmp", "aae", "thm", …
	RelativePath    string
	SizeBytes       int64
	MTime           time.Time
	PartialHash     string
	AttachedAssetID *string // [der] grouping engine writes this
	FirstSeenAt     time.Time
	UpdatedAt       time.Time
}
