package domain

import (
	"strings"
	"time"
)

// ColorMode is the tri-state a bare nullable color cannot express: whether a tag
// inherits its color from an ancestor, sets its own, or explicitly has none (which
// also breaks the inheritance chain for its subtree).
type ColorMode string

const (
	ColorInherit ColorMode = "inherit"
	ColorCustom  ColorMode = "custom"
	ColorNone    ColorMode = "none"
)

// Slugify is the normalized match key: lowercase, trim, collapse internal
// whitespace runs to a single '-'. It deliberately KEEPS non-ASCII — CJK and
// accented keywords (赤, café) must survive, so this normalizes case/whitespace
// only, never ASCII-strips.
func Slugify(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), "-")
}

type Tag struct {
	ID        string
	Name      string
	Slug      string
	ParentID  *string
	Color     *string   // hex "#RRGGBB"; used only when ColorMode == ColorCustom
	ColorMode ColorMode
	Path      string    // derived materialized ancestry '/rootId/…/selfId/'
	CreatedAt time.Time
}

type AssetTag struct {
	AssetID   string
	TagID     string
	Source    string
	RemovedAt *time.Time // nil = active; non-nil = user-suppressed (judgment tombstone)
	CreatedAt time.Time
}
