package domain

type Settings struct {
	XMPConflictResolution string
	ThumbnailQuality      int
	ImportBatchSize       int
	CatalogBackupCount    int
	UndoStackSize         int
	UpdateCheckEnabled    bool
	DefaultSortField      string
	DefaultSortDir        string
}
