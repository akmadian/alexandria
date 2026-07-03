package domain

import "time"

type GroupRole string

const (
	GroupRoleRAW         GroupRole = "raw"
	GroupRoleJPEGSidecar GroupRole = "jpeg_sidecar"
	GroupRoleSource      GroupRole = "source"
	GroupRoleExport      GroupRole = "export"
	GroupRoleMember      GroupRole = "member"
)

type AssetGroup struct {
	ID           string
	CoverAssetID *string
	CreatedAt    time.Time
}

type AssetGroupMember struct {
	GroupID string
	AssetID string
	Role    GroupRole
}
