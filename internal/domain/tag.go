package domain

import "time"

type Tag struct {
	ID        string
	Name      string
	Slug      string
	ParentID  *string
	Color     *string
	CreatedAt time.Time
}

type AssetTag struct {
	AssetID   string
	TagID     string
	Source    string
	CreatedAt time.Time
}
