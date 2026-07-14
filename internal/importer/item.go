package importer

import (
	"context"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/metadata"
	"github.com/akmadian/gospan"
	"github.com/charmbracelet/log"
)

// action is the identity-matrix verdict for one hashed file (docs/data-model.md §6).
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
	logger *log.Logger // child logger with "asset" key baked in

	// ctx carries the item's root trace span across stage goroutines (the gospan
	// pipeline recipe: the span rides the item, not the call stack). It is never
	// used for cancellation — stages keep the run ctx for that. Set at SCAN emit;
	// the root span ends after WRITE commits the item.
	ctx context.Context
	// awaitCommitSpan is the item's write-wait span (the fan-in recipe): started
	// by WRITE when the item arrives, ended at commit, tagged with batch_seq.
	awaitCommitSpan *gospan.Span

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
