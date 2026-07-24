package seam

import (
	"reflect"
	"time"

	"github.com/charmbracelet/log"
)

// This file is the seam's asynchronous channel (C8): the one envelope shape, the
// topic + event-type catalog, and the payload structs — all pure Go, no webview
// dependency. The Wails implementation that actually pushes an envelope to the
// frontend lives in events_wails.go (the single runtime.EventsEmit caller,
// enforced by forbidigo). Services never touch Wails; they hold an Emitter and
// call emit(), so they stay unit-testable with a fake.
//
// C8 in one sentence: every backend→frontend event is `{topic, type, payload,
// timestamp}` over four named topics, every type declared once in the catalog
// below, and an emit of a type not in the catalog is structurally a bug (the
// generator publishes the unions to TS; forbidigo bans ad-hoc EventsEmit).

// Topic is the coarse channel an event rides. Wails events are named channels,
// so the topic string IS the channel name; the Envelope is the payload. The four
// topics are fixed (C8) — a new *kind* of async fact is a new event type under an
// existing topic, not a new topic.
type Topic string

const (
	// TopicCatalog carries catalog mutations (asset/collection writes, history) —
	// the frontend's TanStack-invalidation signal.
	TopicCatalog Topic = "catalog"
	// TopicJobs carries background-work progress and completion (C9).
	TopicJobs Topic = "jobs"
	// TopicWatcher carries volume connectivity / watcher status. Producer lands
	// with the impl/12 watcher supervisor (DEFERRED §2); the type is declared now.
	TopicWatcher Topic = "watcher"
	// TopicSync carries XMP conflict/apply notifications. Reserved for the impl/06
	// remainder; no producer yet, an empty catalog under a declared topic is fine.
	TopicSync Topic = "sync"
)

// EventType is the fine-grained fact within a topic. Every value here has exactly
// one row in eventCatalog (its topic + payload), enforced by ValidateEventCatalog
// as a table test. The generator publishes this union to TS.
type EventType string

const (
	// EventCatalogChanged (topic catalog): assets or collections changed; coarse
	// by default (see CatalogChange). Consumers invalidate the active view.
	EventCatalogChanged EventType = "changed"
	// EventHistoryChanged (topic catalog): the undo/redo stack moved; drives the
	// menu labels. Producer lands with the undo service (DEFERRED §7); declared now.
	EventHistoryChanged EventType = "historyChanged"
	// EventJobProgress (topic jobs): one progress tick of a running job.
	EventJobProgress EventType = "progress"
	// EventJobDone (topic jobs): a job reached a terminal state; carries the summary.
	EventJobDone EventType = "done"
	// EventVolumeStatus (topic watcher): a volume's connectivity/watch mode changed.
	// Producer lands with the watcher supervisor (DEFERRED §2); declared now.
	EventVolumeStatus EventType = "volumeStatus"
)

// JobState is a job's lifecycle state. running rides on progress events; the three
// terminal states ride on the done event. Its own type so the generator emits it
// as a TS union the frontend switches on.
type JobState string

const (
	JobStateRunning   JobState = "running"
	JobStateDone      JobState = "done"
	JobStateFailed    JobState = "failed"
	JobStateCancelled JobState = "cancelled"
)

// Envelope is the single wire shape for every event (C8). Emitted on the topic's
// channel name; the frontend event pump reads topic+type to route and payload to
// act. Timestamp is emit-time RFC3339 (spec §5) — display/debug metadata, never
// ordering truth (events are hints; Wails delivery is fire-and-forget).
type Envelope struct {
	Topic     Topic     `json:"topic"`
	Type      EventType `json:"type"`
	Payload   any       `json:"payload"`
	Timestamp string    `json:"timestamp"`
}

// --- Payload structs -------------------------------------------------------
//
// These mirror the hand-authored interfaces in frontend/src/api/contract.ts so
// the deferred TS generation (see below) is a no-op reconciliation. JobProgress
// and JobDone intentionally carry more than contract.ts's older sketch — that is
// the C9 design target (spec §3), and contract.ts is the thing being reconciled
// away, not a shape to preserve.
//
// DEFERRAL — payload TS types are NOT generated yet. The topic/type/JobState
// *unions* are generated (events.ts); the payload interfaces stay hand-written in
// contract.ts until the `wails dev` reconciliation pass (DEFERRED §7) wires a
// Go-struct→TS emitter. Trigger: the frontend rebuild's event pump needing typed
// payloads. These structs are shaped to match contract.ts today so that pass is
// mechanical. Tracked in DEFERRED §7 and this file's json tags are the contract.

// CatalogChange is the catalog/changed payload. Coarse by default at launch
// (spec §5): scope names the changed area; ids stays empty until a consumer
// measurably needs selective invalidation. Consumers may ignore it entirely and
// invalidate the active view.
type CatalogChange struct {
	Scope string   `json:"scope,omitempty"` // "assets" | "collections" | "tags" | "volumes"
	IDs   []string `json:"ids,omitempty"`   // reserved; not populated at launch
}

// Catalog-change scopes. Named so producers can't typo the coarse bucket.
const (
	ScopeAssets      = "assets"
	ScopeCollections = "collections"
)

// JobProgress is the jobs/progress payload (C9 §3). label is an i18n KEY (C14),
// derived from kind — display text is frontend-owned.
type JobProgress struct {
	JobID      string   `json:"jobId"`
	Kind       string   `json:"kind"`
	Label      string   `json:"label"`
	State      JobState `json:"state"`
	Done       int      `json:"done"`
	Total      int      `json:"total"`
	TotalKnown bool     `json:"totalKnown"`
	Stage      string   `json:"stage,omitempty"`
	Cancelable bool     `json:"cancelable"`
	Message    string   `json:"message,omitempty"`
	// QueueDepth is the enrichment backlog by kind (task 21): jobs not yet
	// complete (queued or in-flight) per kind, the "how much left" signal for a
	// job with no fixed total (the convergent lane has no run identity, so
	// done/total stay 0). Omitted for jobs that report done/total (import), so the
	// one envelope serves both (C9).
	QueueDepth map[string]int `json:"queueDepth,omitempty"`
}

// JobSummary is the completion tally, carried by JobDone. Mirrors the engine's
// ImportResult counts (added/updated/skipped/errors) flattened for the UI.
type JobSummary struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Errors  int `json:"errors"`
}

// JobDone is the jobs/done payload: a terminal state plus the summary. Error is
// diagnostic detail for the dev corner and logs (like ApiError.Detail) — not
// user-facing copy; the UI renders from state + summary.
type JobDone struct {
	JobID   string      `json:"jobId"`
	Kind    string      `json:"kind"`
	State   JobState    `json:"state"`
	Summary *JobSummary `json:"summary,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// HistoryState is the catalog/historyChanged payload: the undo/redo stack's face.
type HistoryState struct {
	CanUndo   bool   `json:"canUndo"`
	CanRedo   bool   `json:"canRedo"`
	UndoLabel string `json:"undoLabel,omitempty"`
	RedoLabel string `json:"redoLabel,omitempty"`
}

// VolumeStatus is the watcher/volumeStatus payload. Shaped so DEFERRED §2's fuller
// snapshot (mode events|polling|offline, last reconcile, dirty count) extends it
// additively — no new event type — when the watcher supervisor lands.
type VolumeStatus struct {
	VolumeID string `json:"volumeId"`
	Status   string `json:"status"` // domain.VolumeConnectivity string
}

// --- The catalog: single source of truth for topic+payload per type ---------

type eventSpec struct {
	topic   Topic
	payload any // zero-value exemplar; its dynamic type gates emitted payloads
}

// eventCatalog is the C8 catalog. Every EventType const above has exactly one row;
// ValidateEventCatalog asserts that both ways (declared type ⇒ a payload, and no
// duplicate payloads) as a table test, and Emit consults it so a type on the wrong
// topic or with the wrong payload cannot cross the seam.
var eventCatalog = map[EventType]eventSpec{
	EventCatalogChanged: {TopicCatalog, CatalogChange{}},
	EventHistoryChanged: {TopicCatalog, HistoryState{}},
	EventJobProgress:    {TopicJobs, JobProgress{}},
	EventJobDone:        {TopicJobs, JobDone{}},
	EventVolumeStatus:   {TopicWatcher, VolumeStatus{}},
}

// validTopics is the closed set an event may ride. TopicSync has no types yet
// (reserved), which is legal — a declared topic with an empty catalog is fine.
var validTopics = map[Topic]struct{}{
	TopicCatalog: {}, TopicJobs: {}, TopicWatcher: {}, TopicSync: {},
}

// buildEnvelope validates (eventType, payload) against the catalog and returns the
// wire envelope. A caller that passes an uncataloged type or a payload of the
// wrong Go type gets ok=false — the emitter logs and drops rather than pushing a
// malformed event (events are hints, C8; no delivery guarantee, spec §5). Real
// mis-emits can't ship: ValidateEventCatalog covers every declared type as a test.
func buildEnvelope(eventType EventType, payload any) (Envelope, bool) {
	spec, ok := eventCatalog[eventType]
	if !ok {
		log.Error("seam: emit of uncataloged event type", "type", eventType)
		return Envelope{}, false
	}
	if reflect.TypeOf(payload) != reflect.TypeOf(spec.payload) {
		log.Error("seam: emit payload type mismatch", "type", eventType,
			"want", reflect.TypeOf(spec.payload), "got", reflect.TypeOf(payload))
		return Envelope{}, false
	}
	return Envelope{
		Topic:     spec.topic,
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}, true
}

// ValidateEventCatalog checks the shipping catalog's internal integrity: every
// entry has a known topic and a non-nil payload, and no two types share a payload
// struct (the "every type has a payload, and vice versa" completeness of C8). Run
// as a table test; mirrors the registry MustValidate idiom (C10). The check itself
// is validateCatalog, so a test can drive its failure branches with a bad map.
func ValidateEventCatalog() error {
	return validateCatalog(eventCatalog)
}

func validateCatalog(catalog map[EventType]eventSpec) error {
	seenPayload := map[reflect.Type]EventType{}
	for eventType, spec := range catalog {
		if _, ok := validTopics[spec.topic]; !ok {
			return &catalogError{eventType: eventType, reason: "unknown topic " + string(spec.topic)}
		}
		payloadType := reflect.TypeOf(spec.payload)
		if payloadType == nil {
			return &catalogError{eventType: eventType, reason: "nil payload exemplar"}
		}
		if other, dup := seenPayload[payloadType]; dup {
			return &catalogError{eventType: eventType, reason: "payload " + payloadType.String() + " already used by " + string(other)}
		}
		seenPayload[payloadType] = eventType
	}
	return nil
}

type catalogError struct {
	eventType EventType
	reason    string
}

func (e *catalogError) Error() string {
	return "event catalog: " + string(e.eventType) + ": " + e.reason
}

// --- Emitter: the seam-side interface services depend on --------------------

// Emitter pushes a validated event to the frontend. Services hold one and call
// emit(); the production implementation is *WailsEmitter (events_wails.go), the
// only caller of runtime.EventsEmit. Tests inject a fake.
type Emitter interface {
	Emit(eventType EventType, payload any)
}

// nopEmitter is the default when no emitter is wired (unit tests that don't assert
// on events; a service constructed without WithEmitter). Dropping events is safe —
// they are hints, and the seam's correctness never depends on delivery.
type nopEmitter struct{}

func (nopEmitter) Emit(EventType, any) {}

// emitting is embedded by every service that emits catalog/changed. It carries the
// injected Emitter and a nil-safe emit() helper, so a write method is one line:
// `s.emit(EventCatalogChanged, CatalogChange{Scope: ScopeAssets})`. Kept DRY here
// rather than repeated per service.
type emitting struct {
	emitter Emitter
}

func (e *emitting) setEmitter(em Emitter) { e.emitter = em }

func (e *emitting) emit(eventType EventType, payload any) {
	if e.emitter != nil {
		e.emitter.Emit(eventType, payload)
	}
}

// Option configures a bound service at construction. Today the only option is the
// emitter; kept as a functional option so adding one never breaks the ~40 existing
// NewXService(...) call sites (they pass no options and compile unchanged).
type Option func(serviceOption)

type serviceOption interface{ setEmitter(Emitter) }

// WithEmitter wires the event emitter into a service. The composition root passes
// the real *WailsEmitter; an emit-asserting test passes a fake.
func WithEmitter(emitter Emitter) Option {
	return func(s serviceOption) { s.setEmitter(emitter) }
}

// jobLabelKey derives a job's i18n label key from its kind (C14) — the frontend
// maps the key to display text. Centralized so every producer keys jobs the same.
func jobLabelKey(kind string) string { return "jobs.kind." + kind }
