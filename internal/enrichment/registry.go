package enrichment

import (
	"context"
	"fmt"
	"time"

	"github.com/akmadian/alexandria/internal/assettype"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/thumbnailer"
)

// This file is the job-definition registry: ONE file that is the whole graph
// (D28 legibility commitment #1). Each definition is a node; its Prerequisites
// are the incoming edges; reading this file is knowing everything that can
// happen to an asset after commit. Add a capability = add a row (C10). The
// hierarchy people want to see (thumbnail → {sharpness, clipping, phash}) is
// a *presentation* over these flat rows — `cmd/dev jobs graph` renders it
// (task 22); the storage stays flat.

// Lane classifies a definition by whether its work's existence is derivable
// (D28).
type Lane string

const (
	// LaneConvergent — work derived from missing artifacts by a scan: no job
	// rows, no run identity, crash recovery = rescan. Enrichment lives here.
	LaneConvergent Lane = "convergent"
	// LaneIntent — user commands no scan can reconstruct (export, transcode…).
	// Durable rows + River, adopted when the first P3 intent feature lands.
	LaneIntent Lane = "intent"
)

// ProduceFunc computes one artifact value for one asset. The asset is the
// operand (the identity the job is about); what the producer physically READS
// is its parents' artifacts (a signal job reads the thumbnail file, not the
// original bytes). It returns the ApplyFunc that will commit the side effect
// — production (slow, parallel, unlimited reads) and catalog mutation (fast,
// single writer goroutine) are separated on purpose. heartbeat resets the
// stall watchdog for Watchdog definitions; others may ignore it (never nil).
// A failing producer wraps its error with Fail(reasonCode, err) to control
// the DLQ taxonomy; a bare error records reason "produce_failed".
type ProduceFunc func(ctx context.Context, asset *domain.Asset, heartbeat func()) (ApplyFunc, error)

// ApplyFunc commits one produced artifact — the job's durable side effect. It
// runs inside the enrichment writer's batch transaction and receives ONLY the
// derived writer interface: touching a judgment or observation column is a
// compile error, not a review catch (writer-class doctrine, data-model.md §1).
type ApplyFunc func(ctx context.Context, writer catalog.AssetDerivedWriter) error

// JobDefinition is one registry row: a node of the graph, the class every
// transient job is stamped from. It is pure data — the engine reifies it into
// a runtime cell (input queue, worker pool, tracker bit) at construction.
type JobDefinition struct {
	// Kind is the registry key ("thumbnail", "sharpness", …) — the stable
	// string naming this job type everywhere: the DLQ kind column, the
	// Workers.Enrichment settings key, the span name suffix. (River uses the
	// same word for the same concept.)
	Kind string
	Lane Lane
	// Applicable reports whether this definition applies to a file type,
	// expressed over the assettype capability table so capability truth is
	// never duplicated (e.g. thumbnail: handler.Thumb != nil). A definition
	// applicable to no registered handler fails validation.
	Applicable func(handler assettype.Handler) bool
	// ArtifactColumn is the derived assets column whose NULL means "missing" —
	// the artifact state machine's storage. Must be on the sqlite allowlist.
	ArtifactColumn string
	// Prerequisites are the incoming edges: kinds whose artifacts must be
	// present before this definition's job may run (D28: the graph is nothing
	// but this unlock rule). Validation topo-sorts these — cycles and dangling
	// references fail boot.
	Prerequisites []string
	// DefaultWorkers is the pool size when machine.json carries no override.
	DefaultWorkers int
	// TimeoutPolicy returns the per-job time budget — a policy function, not a
	// constant (D28): base + per-byte rate, shaped per definition.
	// Non-positive means no deadline (own that choice explicitly in the row).
	TimeoutPolicy func(sizeBytes int64, fileType domain.FileType) time.Duration
	// Watchdog switches the budget from a wall clock to a progress-resettable
	// stall watchdog — for long-running subprocess definitions
	// (transcode-class) where elapsed time is meaningless but silence means
	// stuck.
	Watchdog bool
	// Priority is the definition's class for ordering scan passes (lower scans
	// first). Distinct from a job's queue priority, which is hint-derived.
	// Cross-definition budget arbitration under contention is deliberately not
	// built yet — the first real second definition (task 20) is the evidence
	// that decides whether it is needed.
	Priority int
	// Weight returns the CPU-budget tokens a job acquires, by estimated input
	// size — heavy decodes reserve proportionally, bounding peak memory by
	// construction (D28). Nil means 1 token. Results are clamped to [1, the
	// DIALED capacity], so an over-budget job serializes at the current effort
	// level rather than deadlocking against tokens the dial will never
	// release. ponytail: the size→token mapping is a stated assumption until
	// gospan's samples table exists to calibrate it against measured heap.
	Weight func(sizeBytes int64) int64
	// Produce is the producer — the code that makes the artifact.
	Produce ProduceFunc
}

// Definitions returns the canonical registry, wired with the runtime
// dependencies its producers need. Tasks 20+ append their rows here. Tests
// inject their own rows through Config.Definitions — the canonical table and a
// test's fakes flow through identical validation and dispatch.
func Definitions(thumbnails *thumbnailer.Thumbnailer, sources SourceResolver) []JobDefinition {
	return []JobDefinition{
		{
			Kind: "thumbnail",
			Lane: LaneConvergent,
			// Capability truth stays in the assettype table: a row has a thumbnail
			// strategy or it does not (decode vs. RAW embedded preview is the
			// strategy's business, invisible here).
			Applicable:     func(handler assettype.Handler) bool { return handler.Thumb != nil },
			ArtifactColumn: "thumbnail_at",
			DefaultWorkers: 2, // the old ingest thumb pool's default; machine.json overrides
			TimeoutPolicy:  thumbnailTimeout,
			Priority:       0, // thumbnails first, always — every signal kind gates on them (task 20)
			Weight:         thumbnailWeight,
			Produce:        thumbnailProducer(thumbnails, sources),
		},
		// The cheap signals (task 20): each gates on the thumbnail artifact and
		// reads it off disk. Applicable wherever a thumbnail exists (same predicate
		// as thumbnail). Fixed small weight (nil → 1 token) — the input is a fixed
		// analysis thumbnail, not the original file. Priority 1: right behind
		// thumbnails, so the signals that make culling fast are there when the user
		// sits down (D25/D28).
		{
			Kind:           "sharpness",
			Lane:           LaneConvergent,
			Applicable:     func(handler assettype.Handler) bool { return handler.Thumb != nil },
			ArtifactColumn: "sharpness",
			Prerequisites:  []string{"thumbnail"},
			DefaultWorkers: 2,
			TimeoutPolicy:  signalTimeout,
			Priority:       1,
			Produce:        sharpnessProducer(thumbnails),
		},
		{
			Kind:           "clipping",
			Lane:           LaneConvergent,
			Applicable:     func(handler assettype.Handler) bool { return handler.Thumb != nil },
			ArtifactColumn: "clipping_highlights", // sentinel; the producer writes highlights + shadows together
			Prerequisites:  []string{"thumbnail"},
			DefaultWorkers: 2,
			TimeoutPolicy:  signalTimeout,
			Priority:       1,
			Produce:        clippingProducer(thumbnails),
		},
		{
			Kind:           "phash",
			Lane:           LaneConvergent,
			Applicable:     func(handler assettype.Handler) bool { return handler.Thumb != nil },
			ArtifactColumn: "phash",
			Prerequisites:  []string{"thumbnail"},
			DefaultWorkers: 2,
			TimeoutPolicy:  signalTimeout,
			Priority:       1,
			Produce:        phashProducer(thumbnails),
		},
	}
}

// maxDefinitions bounds the registry because the in-flight tracker is a
// 64-bit KindSet (one bit per definition).
const maxDefinitions = 64

// MustValidate panics on an invalid registry. It is exercised as a table test
// (an incomplete row fails the suite, C10), and New runs Validate on every
// construction — so when task 19's composition root builds the engine, boot
// validation comes with it; there is no unvalidated path to a running engine.
func MustValidate(definitions []JobDefinition) {
	if err := Validate(definitions); err != nil {
		panic(err)
	}
}

// Validate checks the whole registry: unique complete rows, applicability
// non-empty, artifact columns on the sqlite allowlist, and a topo-sortable
// prerequisite graph (cycles and dangling references fail).
func Validate(definitions []JobDefinition) error {
	if len(definitions) > maxDefinitions {
		return fmt.Errorf("enrichment: %d definitions exceeds the %d-bit tracker", len(definitions), maxDefinitions)
	}
	byKind := make(map[string]*JobDefinition, len(definitions))
	for index := range definitions {
		definition := &definitions[index]
		if err := validateDefinition(definition); err != nil {
			return err
		}
		if _, duplicate := byKind[definition.Kind]; duplicate {
			return fmt.Errorf("enrichment: duplicate definition %q", definition.Kind)
		}
		byKind[definition.Kind] = definition
	}
	for index := range definitions {
		for _, prerequisite := range definitions[index].Prerequisites {
			if _, known := byKind[prerequisite]; !known {
				return fmt.Errorf("enrichment: definition %q requires unknown prerequisite %q", definitions[index].Kind, prerequisite)
			}
		}
	}
	return validateAcyclic(definitions, byKind)
}

func validateDefinition(definition *JobDefinition) error {
	if definition.Kind == "" {
		return fmt.Errorf("enrichment: definition with empty kind")
	}
	if definition.Lane != LaneConvergent && definition.Lane != LaneIntent {
		return fmt.Errorf("enrichment: definition %q has unknown lane %q", definition.Kind, definition.Lane)
	}
	if definition.Produce == nil {
		return fmt.Errorf("enrichment: definition %q has no producer", definition.Kind)
	}
	if definition.TimeoutPolicy == nil {
		return fmt.Errorf("enrichment: definition %q has no timeout policy", definition.Kind)
	}
	if definition.DefaultWorkers <= 0 {
		return fmt.Errorf("enrichment: definition %q has non-positive default workers", definition.Kind)
	}
	if !sqlite.IsDerivedArtifactColumn(definition.ArtifactColumn) {
		return fmt.Errorf("enrichment: definition %q artifact column %q is not a derived artifact column", definition.Kind, definition.ArtifactColumn)
	}
	if definition.Applicable == nil {
		return fmt.Errorf("enrichment: definition %q has no applicability predicate", definition.Kind)
	}
	if len(applicableExtensions(definition)) == 0 {
		return fmt.Errorf("enrichment: definition %q is applicable to no registered asset type", definition.Kind)
	}
	return nil
}

// validateAcyclic runs a depth-first three-color walk over the prerequisite
// edges; a back edge is a cycle.
func validateAcyclic(definitions []JobDefinition, byKind map[string]*JobDefinition) error {
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	colors := make(map[string]int, len(definitions))
	var visit func(kind string) error
	visit = func(kind string) error {
		switch colors[kind] {
		case visiting:
			return fmt.Errorf("enrichment: prerequisite cycle through definition %q", kind)
		case done:
			return nil
		}
		colors[kind] = visiting
		for _, prerequisite := range byKind[kind].Prerequisites {
			if err := visit(prerequisite); err != nil {
				return err
			}
		}
		colors[kind] = done
		return nil
	}
	for index := range definitions {
		if err := visit(definitions[index].Kind); err != nil {
			return err
		}
	}
	return nil
}

// applicableExtensions projects a definition's applicability predicate onto
// the assettype table, yielding the extension set its missing-artifact scan
// filters on.
func applicableExtensions(definition *JobDefinition) []string {
	var extensions []string
	for _, handler := range assettype.All() {
		if definition.Applicable(handler) {
			extensions = append(extensions, handler.Ext)
		}
	}
	return extensions
}

// ReasonError carries a machine-readable DLQ reason code with a producer
// failure. Producers wrap with Fail; everything else defaults to
// "produce_failed".
type ReasonError struct {
	ReasonCode string
	Err        error
}

func (e *ReasonError) Error() string { return e.ReasonCode + ": " + e.Err.Error() }
func (e *ReasonError) Unwrap() error { return e.Err }

// Fail wraps a producer error with the DLQ reason code it should record.
func Fail(reasonCode string, err error) error {
	return &ReasonError{ReasonCode: reasonCode, Err: err}
}
