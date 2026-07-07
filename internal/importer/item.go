package importer

import (
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/metadata"
)

// action is the identity-matrix verdict for one hashed file (03-data-model.md §6).
type action int

const (
	actionNew action = iota
	actionReimport
	actionDuplicate
)

func (a action) String() string {
	switch a {
	case actionNew:
		return "new"
	case actionReimport:
		return "reimport"
	case actionDuplicate:
		return "duplicate"
	default:
		return "unknown"
	}
}

// pipelineItem threads one file through the pipeline; each stage fills in its own
// fields. It is the shared state the six stage files (stage_*.go) read and write
// as it flows SCAN → HASH → MATCH → EXTRACT → THUMB → WRITE.
type pipelineItem struct {
	scanned           scannedFile
	isSidecar         bool
	head              []byte // first ≤64KB, held only across HASH (for Sniff), then released
	hash              string
	rejected          bool // terminal: log the error rows, mint no identity
	verdict           action
	existing          *domain.Asset
	assetID           string
	extractedMetadata metadata.Metadata
	thumbnailedAt     *time.Time
	mismatchMarker    map[string]any // extension_mismatch marker → extended_metadata
	stageErrors       []stageError
}

// stageError is one file's failure at one stage — it becomes an import_errors
// (DLQ) row at WRITE. Accumulating these on the item, rather than aborting, is
// the self-heal doctrine (D13): the asset still indexes where it can.
type stageError struct {
	stage      string
	reasonCode string
	message    string
}

func (item *pipelineItem) addError(stage, reasonCode, message string) {
	item.stageErrors = append(item.stageErrors, stageError{stage, reasonCode, message})
}
