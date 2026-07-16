package seam_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/akmadian/alexandria/internal/ast"
	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/seam"
)

// --- fakes (structurally satisfy the seam's unexported enrichment interfaces) ---

type fakeEnrichmentView struct {
	running map[string][]domain.EnrichmentKind
	failed  map[string][]domain.EnrichmentKind
	failErr error
}

func (v *fakeEnrichmentView) RunningKinds(_ []string) map[string][]domain.EnrichmentKind {
	return v.running
}

func (v *fakeEnrichmentView) FailedKinds(_ context.Context, _ []string) (map[string][]domain.EnrichmentKind, error) {
	return v.failed, v.failErr
}

type fakeController struct {
	pausedAll, resumedAll bool
	pausedKind            string
	resumedKind           string
	effort                string
	hinted                []string
}

func (c *fakeController) PauseAll()              { c.pausedAll = true }
func (c *fakeController) ResumeAll()             { c.resumedAll = true }
func (c *fakeController) PauseKind(kind string)  { c.pausedKind = kind }
func (c *fakeController) ResumeKind(kind string) { c.resumedKind = kind }
func (c *fakeController) SetEffort(level string) { c.effort = level }
func (c *fakeController) Hint(assetIDs []string) { c.hinted = assetIDs }

type fakeEffortStore struct {
	saved string
	err   error
}

func (s *fakeEffortStore) SetEnrichmentEffort(level string) error {
	if s.err != nil {
		return s.err
	}
	s.saved = level
	return nil
}

// --- Decoration ---

// TestQueryAssets_DecoratesEnrichment is the task-21 acceptance: in a 200-row page,
// exactly the mid-enrichment rows carry the running kinds and exactly the failed
// rows carry the failed kinds; every idle row carries neither.
func TestQueryAssets_DecoratesEnrichment(t *testing.T) {
	rows := make([]catalog.AssetRow, 200)
	for index := range rows {
		rows[index] = catalog.AssetRow{ID: "asset-" + itoa(index)}
	}
	view := &fakeEnrichmentView{
		running: map[string][]domain.EnrichmentKind{
			"asset-5":   {domain.EnrichmentKindThumbnail},
			"asset-100": {domain.EnrichmentKindSharpness, domain.EnrichmentKindClipping},
		},
		failed: map[string][]domain.EnrichmentKind{
			"asset-7": {domain.EnrichmentKindThumbnail},
		},
	}
	service := seam.NewAssetService(&fakeAssets{rows: rows, total: 200}, &fakeAssets{}, seam.WithEnrichmentView(view))

	result, err := service.QueryAssets(validQuery(), ast.Arrangement{SortField: ast.SortIngestedAt, SortDir: ast.SortDesc}, ast.Page{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range result.Items {
		wantRunning := view.running[row.ID]
		wantFailed := view.failed[row.ID]
		if !slices.Equal(row.Enriching, wantRunning) {
			t.Errorf("%s enriching = %v, want %v", row.ID, row.Enriching, wantRunning)
		}
		if !slices.Equal(row.Failed, wantFailed) {
			t.Errorf("%s failed = %v, want %v", row.ID, row.Failed, wantFailed)
		}
	}
}

func TestQueryAssets_NoViewLeavesRowsUndecorated(t *testing.T) {
	rows := []catalog.AssetRow{{ID: "a"}, {ID: "b"}}
	service := seam.NewAssetService(&fakeAssets{rows: rows, total: 2}, &fakeAssets{})
	result, err := service.QueryAssets(validQuery(), filenameAsc, ast.Page{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range result.Items {
		if row.Enriching != nil || row.Failed != nil {
			t.Errorf("%s decorated without a view: %v / %v", row.ID, row.Enriching, row.Failed)
		}
	}
}

// TestQueryAssets_FailedDecorationBestEffort: a DLQ read error drops the failed
// half and logs, but the running half still decorates and the query still succeeds.
func TestQueryAssets_FailedDecorationBestEffort(t *testing.T) {
	rows := []catalog.AssetRow{{ID: "a"}}
	view := &fakeEnrichmentView{
		running: map[string][]domain.EnrichmentKind{"a": {domain.EnrichmentKindThumbnail}},
		failErr: errors.New("dlq unavailable"),
	}
	service := seam.NewAssetService(&fakeAssets{rows: rows, total: 1}, &fakeAssets{}, seam.WithEnrichmentView(view))
	result, err := service.QueryAssets(validQuery(), filenameAsc, ast.Page{Limit: 1})
	if err != nil {
		t.Fatalf("decoration error must not fail the query: %v", err)
	}
	if !slices.Equal(result.Items[0].Enriching, []domain.EnrichmentKind{domain.EnrichmentKindThumbnail}) {
		t.Errorf("running half must decorate despite the failed-read error, got %v", result.Items[0].Enriching)
	}
	if result.Items[0].Failed != nil {
		t.Errorf("failed half must be dropped on error, got %v", result.Items[0].Failed)
	}
}

// --- Controls ---

func TestEnrichmentControls_PassThrough(t *testing.T) {
	controller := &fakeController{}
	service := seam.NewEnrichmentEngineService(controller, &fakeEffortStore{})

	service.PauseAll()
	service.ResumeAll()
	service.PauseKind("thumbnail")
	service.ResumeKind("sharpness")
	service.Hint([]string{"a", "b"})

	if !controller.pausedAll || !controller.resumedAll {
		t.Error("pause/resume all did not pass through")
	}
	if controller.pausedKind != "thumbnail" || controller.resumedKind != "sharpness" {
		t.Errorf("per-kind pause/resume mismatch: %q / %q", controller.pausedKind, controller.resumedKind)
	}
	if len(controller.hinted) != 2 {
		t.Errorf("hint did not pass through: %v", controller.hinted)
	}
}

func TestEnrichmentEffort_PersistsThenApplies(t *testing.T) {
	controller := &fakeController{}
	store := &fakeEffortStore{}
	service := seam.NewEnrichmentEngineService(controller, store)

	if err := service.SetEffort("low"); err != nil {
		t.Fatal(err)
	}
	if store.saved != "low" || controller.effort != "low" {
		t.Errorf("effort: persisted=%q applied=%q, want low/low", store.saved, controller.effort)
	}
}

func TestEnrichmentEffort_UnknownLevelRejected(t *testing.T) {
	controller := &fakeController{}
	store := &fakeEffortStore{}
	service := seam.NewEnrichmentEngineService(controller, store)

	if err := service.SetEffort("turbo"); err == nil {
		t.Fatal("unknown effort level must error")
	}
	if store.saved != "" || controller.effort != "" {
		t.Error("a rejected level must not persist or apply")
	}
}

func TestEnrichmentEffort_PersistFailureDoesNotApply(t *testing.T) {
	controller := &fakeController{}
	store := &fakeEffortStore{err: errors.New("disk full")}
	service := seam.NewEnrichmentEngineService(controller, store)

	if err := service.SetEffort("full"); err == nil {
		t.Fatal("persist failure must surface")
	}
	if controller.effort != "" {
		t.Error("persist-first: a failed persist must not apply live")
	}
}

// --- Aggregate events ---

func TestEmitEnrichmentBatch_EmitsInvalidationAndProgress(t *testing.T) {
	emitter := &fakeEmitter{}
	seam.EmitEnrichmentBatch(emitter, map[string]int{"thumbnail": 3, "sharpness": 1})

	types := emitter.typesOf()
	if len(types) != 2 || types[0] != seam.EventCatalogChanged || types[1] != seam.EventJobProgress {
		t.Fatalf("want [changed, progress], got %v", types)
	}
	progress, ok := emitter.snapshot()[1].Payload.(seam.JobProgress)
	if !ok {
		t.Fatalf("progress payload type: %T", emitter.snapshot()[1].Payload)
	}
	if progress.State != seam.JobStateRunning || progress.QueueDepth["thumbnail"] != 3 {
		t.Errorf("progress = %+v, want running with depth", progress)
	}
}

func TestEmitEnrichmentBatch_ZeroDepthIsDone(t *testing.T) {
	emitter := &fakeEmitter{}
	seam.EmitEnrichmentBatch(emitter, map[string]int{})
	progress := emitter.snapshot()[1].Payload.(seam.JobProgress)
	if progress.State != seam.JobStateDone {
		t.Errorf("drained backlog must report done, got %q", progress.State)
	}
}

func TestEmitEnrichmentBatch_NilEmitterNoPanic(t *testing.T) {
	seam.EmitEnrichmentBatch(nil, map[string]int{"thumbnail": 1})
}

// --- helpers ---

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := []byte{}
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
