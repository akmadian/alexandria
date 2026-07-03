package domain

import "time"

type CollectionKind string

const (
	CollectionKindManual CollectionKind = "manual"
	CollectionKindSmart  CollectionKind = "smart"
)

type Collection struct {
	ID           string
	Name         string
	ParentID     *string
	Kind         CollectionKind
	Query        *string
	CoverAssetID *string
	SortField    *string
	SortDir      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
