package seam

import (
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

// AssetDetail is the full-asset wire projection GetAsset returns — the detail
// counterpart to catalog.AssetRow's slim grid projection. The json tags ARE the
// wire contract (C13/C15): cmd/generate reflects this struct into the generated
// TS model. It is a read-only view for the inspector; judgment bookkeeping
// (judgment_modified_at), sync-state cursors, and derived internals stay off
// the wire — they are engine concerns, not display rows.
//
// This struct is also the decoration point DEFERRED §13 predicts: per-asset
// transient enrichment state (running/failed/blocked kinds, DLQ reasons)
// attaches HERE when the inspector renders it — never as a separate
// enrichment-service lookup keyed by asset.
type AssetDetail struct {
	ID           string            `json:"id"`
	SourceID     string            `json:"sourceId"`
	Filename     string            `json:"filename"`
	Extension    string            `json:"extension"`
	MIMEType     string            `json:"mimeType"`
	FileType     domain.FileType   `json:"fileType"`
	FileStatus   domain.FileStatus `json:"fileStatus"`
	RelativePath string            `json:"relativePath"`
	SizeBytes    int64             `json:"sizeBytes"`
	MTime        time.Time         `json:"mtime"`
	IngestedAt   time.Time         `json:"ingestedAt"`

	Width         *int       `json:"width"`
	Height        *int       `json:"height"`
	DurationSecs  *float64   `json:"durationSecs"`
	CapturedAt    *time.Time `json:"capturedAt"`
	CameraMake    *string    `json:"cameraMake"`
	CameraModel   *string    `json:"cameraModel"`
	LensModel     *string    `json:"lensModel"`
	FocalLengthMM *float64   `json:"focalLengthMm"`
	Aperture      *float64   `json:"aperture"`
	ShutterSpeed  *string    `json:"shutterSpeed"`
	ISO           *int       `json:"iso"`
	GPSLat        *float64   `json:"gpsLat"`
	GPSLon        *float64   `json:"gpsLon"`
	ColorSpace    *string    `json:"colorSpace"`
	BitDepth      *int       `json:"bitDepth"`

	Title     *string `json:"title"`
	Caption   *string `json:"caption"`
	Creator   *string `json:"creator"`
	Copyright *string `json:"copyright"`

	Rating     *int               `json:"rating"`
	ColorLabel *domain.ColorLabel `json:"colorLabel"`
	Flag       *domain.Flag       `json:"flag"`
	Note       *string            `json:"note"`

	// ExtendedMetadata is the full extraction blob keyed by exiftool
	// "Group:Tag" names — everything observed that was not promoted to a
	// column (D11). omitempty: an asset with no blob carries no key.
	ExtendedMetadata map[string]any `json:"extendedMetadata,omitempty"`
}

// detailFromAsset projects the domain asset onto the wire shape. Pure — field
// selection is the only logic, so the seam owns exactly which internals cross.
func detailFromAsset(asset *domain.Asset) AssetDetail {
	return AssetDetail{
		ID:           asset.ID,
		SourceID:     asset.SourceID,
		Filename:     asset.Filename,
		Extension:    asset.Extension,
		MIMEType:     asset.MIMEType,
		FileType:     asset.FileType,
		FileStatus:   asset.FileStatus,
		RelativePath: asset.RelativePath,
		SizeBytes:    asset.SizeBytes,
		MTime:        asset.MTime,
		IngestedAt:   asset.IngestedAt,

		Width:         asset.Width,
		Height:        asset.Height,
		DurationSecs:  asset.DurationSecs,
		CapturedAt:    asset.CapturedAt,
		CameraMake:    asset.CameraMake,
		CameraModel:   asset.CameraModel,
		LensModel:     asset.LensModel,
		FocalLengthMM: asset.FocalLengthMM,
		Aperture:      asset.Aperture,
		ShutterSpeed:  asset.ShutterSpeed,
		ISO:           asset.ISO,
		GPSLat:        asset.GPSLat,
		GPSLon:        asset.GPSLon,
		ColorSpace:    asset.ColorSpace,
		BitDepth:      asset.BitDepth,

		Title:     asset.Title,
		Caption:   asset.Caption,
		Creator:   asset.Creator,
		Copyright: asset.Copyright,

		Rating:     asset.Rating,
		ColorLabel: asset.ColorLabel,
		Flag:       asset.Flag,
		Note:       asset.Note,

		ExtendedMetadata: asset.ExtendedMetadata,
	}
}
