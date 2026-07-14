package enrichment

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/akmadian/alexandria/internal/assettype"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/settings"
	"github.com/akmadian/alexandria/internal/sqlite"
	"github.com/akmadian/alexandria/internal/testutil"
	"github.com/charmbracelet/log"
)

// Internal dispatcher tests: the guard branches the black-box harness cannot
// steer into deterministically — ledger dedup, hint-survivor handling, edge
// emission's applicability gate, and refill-on-drain. State is built with the
// same newDispatcherState production wiring; no goroutines run, so every
// assertion reads the state directly.

func internalDefinition(kind string, applicable func(handler assettype.Handler) bool) JobDefinition {
	return JobDefinition{
		Kind:           kind,
		Lane:           LaneConvergent,
		Applicable:     applicable,
		ArtifactColumn: "thumbnail_at",
		DefaultWorkers: 1,
		TimeoutPolicy:  func(sizeBytes int64, fileType domain.FileType) time.Duration { return time.Second },
		Produce: func(ctx context.Context, asset *domain.Asset, heartbeat func()) (ApplyFunc, error) {
			return func(ctx context.Context, writer catalog.AssetDerivedWriter) error { return nil }, nil
		},
	}
}

func imageApplicable(handler assettype.Handler) bool { return handler.Type == domain.FileTypeImage }

func jpgOnlyApplicable(handler assettype.Handler) bool { return handler.Ext == "jpg" }

// internalEngineState builds an engine over a real migrated catalog with
// missingAssets missing-thumbnail assets, plus the dispatcher state — nothing
// started, nothing concurrent.
func internalEngineState(t *testing.T, definitions []JobDefinition, missingAssets int) (*Engine, *dispatcherState) {
	t.Helper()
	db := testutil.NewTestDB(t)
	source := testutil.NewTestSource(t, db, "internal")
	for index := range missingAssets {
		testutil.NewTestAsset(t, db, source.ID, "internal-"+string(rune('a'+index))+".jpg")
	}
	engine, err := New(&Config{
		Definitions: definitions,
		Reader:      &sqlite.AssetRepo{DB: db},
		Store:       sqlite.NewStore(db),
		Enrichment:  &sqlite.EnrichmentRepo{DB: db},
		Log:         log.New(io.Discard),
		Machine:     settings.DefaultMachine(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return engine, newDispatcherState(engine)
}

func TestEnqueueDeduplicatesAcrossSources(t *testing.T) {
	_, state := internalEngineState(t, []JobDefinition{internalDefinition("fake", imageApplicable)}, 0)
	when := time.Now().UTC()
	if !state.enqueue("fake", "asset-1", when) {
		t.Fatal("first enqueue refused")
	}
	// A second source (scan, hint, edge emission) colliding on the same JobKey
	// must be a no-op — the ledger is the sole double-dispatch defense.
	if state.enqueue("fake", "asset-1", when) {
		t.Fatal("duplicate enqueue accepted for an already-pending JobKey")
	}
	if state.pendingCount["fake"] != 1 || state.queues["fake"].Len() != 1 {
		t.Fatalf("pending=%d queued=%d after duplicate enqueue, want 1/1",
			state.pendingCount["fake"], state.queues["fake"].Len())
	}
}

func TestHandleHintKeepsSurvivorsAndRerankThem(t *testing.T) {
	_, state := internalEngineState(t, []JobDefinition{internalDefinition("fake", imageApplicable)}, 0)
	now := time.Now().UTC()
	state.enqueue("fake", "asset-a", now.Add(-time.Hour))
	state.enqueue("fake", "asset-b", now)

	state.handleHint([]string{"asset-a", "asset-b"})
	entryA := state.ledger[JobKey{AssetID: "asset-a", Kind: "fake"}]
	entryB := state.ledger[JobKey{AssetID: "asset-b", Kind: "fake"}]
	if entryA.priority != priorityHinted || entryA.hintRank != 0 || entryB.hintRank != 1 {
		t.Fatalf("first hint generation: a=%+v b=%+v", entryA, entryB)
	}

	// Both assets survive into the next generation with swapped ranks: neither
	// may demote; both re-rank.
	state.handleHint([]string{"asset-b", "asset-a"})
	if entryA.priority != priorityHinted || entryB.priority != priorityHinted {
		t.Fatalf("hint survivor demoted: a=%+v b=%+v", entryA, entryB)
	}
	if entryB.hintRank != 0 || entryA.hintRank != 1 {
		t.Fatalf("survivor ranks not updated: a.rank=%d b.rank=%d, want 1/0", entryA.hintRank, entryB.hintRank)
	}

	// Dropping an asset from the set demotes it; recency survives demotion.
	state.handleHint([]string{"asset-b"})
	if entryA.priority != priorityNormal {
		t.Fatalf("dropped hint not demoted: %+v", entryA)
	}
	if !entryA.ingestedAt.Equal(now.Add(-time.Hour)) {
		t.Fatalf("demotion clobbered ingest recency: %v", entryA.ingestedAt)
	}
}

func TestEdgeEmissionRespectsDependentApplicability(t *testing.T) {
	parent := internalDefinition("parent", imageApplicable)
	child := internalDefinition("child", jpgOnlyApplicable)
	child.Prerequisites = []string{"parent"}
	_, state := internalEngineState(t, []JobDefinition{parent, child}, 0)

	// A parent completion for a PNG asset must NOT emit into the jpg-only
	// child's queue; the same completion for a JPG asset must.
	state.handleCompletions(context.Background(), []completion{{
		key: JobKey{AssetID: "png-asset", Kind: "parent"}, applied: true,
		extension: "png", ingestedAt: time.Now().UTC(),
	}})
	if entry := state.ledger[JobKey{AssetID: "png-asset", Kind: "child"}]; entry != nil {
		t.Fatalf("emission ignored the child's applicability: %+v", entry)
	}
	state.handleCompletions(context.Background(), []completion{{
		key: JobKey{AssetID: "jpg-asset", Kind: "parent"}, applied: true,
		extension: "jpg", ingestedAt: time.Now().UTC(),
	}})
	if entry := state.ledger[JobKey{AssetID: "jpg-asset", Kind: "child"}]; entry == nil {
		t.Fatal("emission did not enqueue an applicable dependent")
	}
	// Failed/skipped completions never emit.
	state.handleCompletions(context.Background(), []completion{{
		key: JobKey{AssetID: "failed-asset", Kind: "parent"}, applied: false,
		extension: "jpg", ingestedAt: time.Now().UTC(),
	}})
	if entry := state.ledger[JobKey{AssetID: "failed-asset", Kind: "child"}]; entry != nil {
		t.Fatal("emission fired for a non-applied completion")
	}
}

func TestRefillOnDrainRescansTruncatedBacklog(t *testing.T) {
	_, state := internalEngineState(t, []JobDefinition{internalDefinition("fake", imageApplicable)}, 3)
	// Simulate "the last scan hit the page limit, and the queue has drained":
	// the completion handler must re-scan and refill from the catalog rather
	// than stall with derivable work still missing.
	state.moreToScan["fake"] = true
	state.handleCompletions(context.Background(), nil)
	if queued := state.queues["fake"].Len(); queued != 3 {
		t.Fatalf("refill scan queued %d jobs, want 3", queued)
	}
	if state.moreToScan["fake"] {
		t.Fatal("a non-full refill scan should clear moreToScan")
	}
}
