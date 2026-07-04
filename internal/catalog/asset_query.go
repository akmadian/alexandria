package catalog

import (
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

// AssetPatch is a sparse update to an asset: only fields with Set=true are
// written. It's a repository query concern, not a global domain noun, so it
// lives with the catalog ports rather than in domain.
type AssetPatch struct {
	Rating           domain.Opt[int]
	ColorLabel       domain.Opt[domain.ColorLabel]
	Flag             domain.Opt[domain.Flag]
	Note             domain.Opt[string]
	ThumbnailAt      domain.Opt[time.Time]
	XMPLastReadAt    domain.Opt[time.Time]
	XMPLastWrittenAt domain.Opt[time.Time]
	XMPHash          domain.Opt[string]
	IsDeleted        domain.Opt[bool]
	DeletedAt        domain.Opt[time.Time]
}

// AssetFilter is the query specification for AssetRepository.List.
type AssetFilter struct {
	FileTypes      []domain.FileType
	Rating         *int
	RatingMin      *int
	ColorLabels    []domain.ColorLabel
	Flags          []domain.Flag
	TagIDs         []string
	SourceIDs      []string
	CapturedAfter  *time.Time
	CapturedBefore *time.Time
	SearchText     string
	IncludeDeleted bool
	SortField      string
	SortDir        string
	Limit          int
	Offset         int
}
