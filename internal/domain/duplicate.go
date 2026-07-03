package domain

import "time"

type Duplicate struct {
	ID               string
	OriginalAssetID  string
	DuplicateAssetID string
	PartialHash      string
	DetectedAt       time.Time
	Status           string
	ResolvedAt       *time.Time
}
