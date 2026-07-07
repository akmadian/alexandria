package domain

import "github.com/google/uuid"

// NewID returns a fresh UUIDv7 as a string — the primary key for every catalog
// entity. v7 is time-ordered, so freshly minted IDs sort by creation time and
// land adjacent in index b-trees (locality that matters at 500k rows). UUIDs
// (not autoincrement) are load-bearing for multi-catalog and bundle merge-back:
// rows minted in different catalogs must never collide.
//
// NewV7 only fails if the system random source fails, which is unrecoverable —
// panicking is the right call for ID generation.
func NewID() string {
	return uuid.Must(uuid.NewV7()).String()
}
