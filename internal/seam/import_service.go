package seam

import (
	"context"
	"errors"

	"github.com/charmbracelet/log"

	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/importer"
)

// jobKindImport is the kind string for import jobs — the first C9 producer. Every
// future background kind (reconcile, integrity, xmp_sync, thumb_rebuild, enrich)
// is another such string reporting through the same JobProgress/JobDone envelope,
// zero new seam surface (C9: no private progress paths).
const jobKindImport = "import"

// folderGetter is the read slice ImportService needs to resolve a tracked folder
// before ingesting. Matches sqlite.FolderRepo.Get.
type folderGetter interface {
	Get(ctx context.Context, id string) (*domain.Folder, error)
}

// volumeGetter resolves a folder's volume for the up-front offline check. Matches
// sqlite.VolumeRepo.Get.
type volumeGetter interface {
	Get(ctx context.Context, id string) (*domain.Volume, error)
}

// runImport executes one import run over a tracked folder, invoking onProgress
// for each engine Progress tick, and returns the final result. The composition
// root supplies the real implementation (resolve the folder's volume to a mount,
// build a wired importer, set OnProgress, call RunJob over an importer.Target);
// tests supply a fake so ImportService's job/event behavior — including
// cancellation — is unit-testable without a database or a real walk.
//
// This is the D1 seam: the engine hands over its existing OnProgress callback and
// RunJob; internal/seam adapts them to the job envelope and the emitter. The
// engine imports no Wails.
type runImport func(ctx context.Context, jobID string, folder *domain.Folder, onProgress func(importer.Progress)) (importer.ImportResult, error)

// ImportService is the seam face of the import pipeline: start a cancelable import
// job, report progress and completion through the C9 envelope. It owns no pipeline
// logic — it resolves the folder + its volume, launches the injected run under the
// Jobs registry, and maps engine Progress/ImportResult to events.
type ImportService struct {
	folders folderGetter
	volumes volumeGetter
	jobs    *importer.Jobs
	run     runImport
	emitter Emitter
}

// NewImportService constructs the bound import service. The emitter is required
// (unlike the read/write services' optional emitter) — a job producer whose whole
// job is to report progress has nothing to do without it; a nil emitter would
// silently swallow every progress event. The host always has a real emitter by
// the time it builds this.
func NewImportService(folders folderGetter, volumes volumeGetter, jobs *importer.Jobs, run runImport, emitter Emitter) *ImportService {
	if emitter == nil {
		emitter = nopEmitter{}
	}
	return &ImportService{folders: folders, volumes: volumes, jobs: jobs, run: run, emitter: emitter}
}

// StartImport resolves the folder + its volume and launches an import job,
// returning its id immediately (progress arrives via jobs/progress events,
// completion via jobs/done). An offline volume is rejected up front with
// volume_offline rather than failing deep in the walk. The job runs under the
// Jobs registry so CancelJob can stop it; the terminal event distinguishes
// done/failed/cancelled.
func (s *ImportService) StartImport(folderID string) (string, error) {
	folder, err := s.folders.Get(seamContext(), folderID)
	if err != nil {
		log.Error("seam: StartImport resolve folder failed", "folder", folderID, "err", err)
		return "", normalizeError(err)
	}
	volume, err := s.volumes.Get(seamContext(), folder.VolumeID)
	if err != nil {
		log.Error("seam: StartImport resolve volume failed", "volume", folder.VolumeID, "err", err)
		return "", normalizeError(err)
	}
	if volume.Connectivity == domain.VolumeOffline {
		return "", normalizeError(&domain.VolumeOfflineError{VolumeID: volume.ID, Path: folder.Path})
	}

	jobID := s.jobs.Start(jobKindImport, func(ctx context.Context, id string) {
		onProgress := func(progress importer.Progress) {
			s.emitter.Emit(EventJobProgress, jobProgressFrom(id, progress))
		}
		result, runErr := s.run(ctx, id, folder, onProgress)
		s.emitter.Emit(EventJobDone, jobDoneFrom(id, &result, runErr))
	})
	log.Info("seam: started import", "job", jobID, "folder", folderID)
	return jobID, nil
}

// CancelJob requests cancellation of a running job (no-op if unknown or already
// finished). The job's cancelable context unwinds the pipeline, which emits a
// terminal jobs/done with state cancelled; the batch-commit invariant holds
// (already-committed assets are fully processed — the importer's guarantee).
func (s *ImportService) CancelJob(jobID string) error {
	s.jobs.Cancel(jobID)
	log.Info("seam: cancel requested", "job", jobID)
	return nil
}

// jobProgressFrom maps an engine Progress tick to the C9 progress payload. Kind
// falls back to the import kind when the engine leaves it empty; import jobs are
// always cancelable.
func jobProgressFrom(jobID string, progress importer.Progress) JobProgress {
	kind := progress.Kind
	if kind == "" {
		kind = jobKindImport
	}
	return JobProgress{
		JobID:      jobID,
		Kind:       kind,
		Label:      jobLabelKey(kind),
		State:      JobStateRunning,
		Done:       progress.Done,
		Total:      progress.Total,
		TotalKnown: progress.TotalKnown,
		Stage:      progress.Stage,
		Cancelable: true,
	}
}

// jobDoneFrom maps a finished run to the terminal jobs/done payload. Classification
// keys off the run's error, not the context: a run returning nil completed
// successfully and is "done" even if a cancel raced in at the very end — RunJob
// returns context.Canceled only when it actually stopped early (pipeline.go:208).
// A cancelled run still returns a valid partial result, so its summary rides along;
// any other error is a failure. The summary is always attached — even a
// failed/cancelled run committed some assets.
func jobDoneFrom(jobID string, result *importer.ImportResult, runErr error) JobDone {
	summary := jobSummaryFrom(result)
	done := JobDone{JobID: jobID, Kind: jobKindImport, Summary: &summary}

	switch {
	case runErr == nil:
		done.State = JobStateDone
	case errors.Is(runErr, context.Canceled):
		done.State = JobStateCancelled
	default:
		done.State = JobStateFailed
		done.Error = runErr.Error() // diagnostic detail, not user copy
		log.Error("seam: import job failed", "job", jobID, "err", runErr)
	}
	return done
}

// jobSummaryFrom flattens the engine's ImportResult counts into the UI summary.
// Moved/Dups/Missing fold into "updated"-adjacent reporting later if a consumer
// wants them broken out; the four-count summary is the C9 shape today.
func jobSummaryFrom(result *importer.ImportResult) JobSummary {
	return JobSummary{
		Added:   result.Added,
		Updated: result.Updated,
		Skipped: result.Skipped,
		Errors:  len(result.Errors),
	}
}
