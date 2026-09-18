//
//  ImportStatus.swift
//  Alexandria
//
//  What the browser's folder row is told about an import — a value, never
//  the run. All display arithmetic lives in the one initializer so the
//  resume fold (skipped = already cataloged = done) is pinned in a single
//  testable place: a resumed 90%-done import must read ~90%, not ~0%.
//

import Foundation

nonisolated enum ImportStatus: Equatable, Sendable {
	/// Sizing the import — indeterminate by honest necessity.
	case walking
	/// The counted middle. `ready` = fully usable (thumbnailed or already
	/// cataloged before this run); `cataloged` includes ready; `total` > 0.
	case progress(ready: Int, cataloged: Int, total: Int)
	/// The post-drain tail — indeterminate again, briefly.
	case finishing
	/// Lingering until dismissed: a check when clean, an alert otherwise.
	case done(outcome: ImportOutcome, imported: Int, failures: Int)
	/// No live run, but the folder's latest import never finished — the
	/// resume badge (same predicate as Catalog.unfinishedImport).
	case needsResume
}

extension ImportStatus {
	/// The three-truth precedence (ruling 1): a live or lingering run
	/// outranks the tree's durable unfinished flag — an import being
	/// finished, or lingering as done, must never read as "reimport me" —
	/// and the flag outranks nothing. Both call sites (BrowserView's no-run
	/// branch, ImportStatusSlot's run branch) route through here.
	@MainActor static func current(run: ImportRun?, unfinished: Bool) -> ImportStatus? {
		if let run {
			ImportStatus(run: run)
		} else if unfinished {
			.needsResume
		} else {
			nil
		}
	}

	@MainActor init(run: ImportRun) {
		self.init(
			phase: run.phase,
			totalFiles: run.totalFiles,
			completedFiles: run.completedFiles,
			skippedFiles: run.skippedFiles,
			settledFiles: run.settledFiles,
			thumbnailFailures: run.thumbnailFailures,
			walkFailures: run.walkFailures
		)
	}

	/// The pure arithmetic, on raw counters — what the tests pin. `ready`
	/// is settledFiles alone: batch-time settlements (skips the drain won't
	/// revisit, unthumbnailable kinds) and drain settlements (generated +
	/// residue) are disjoint by the worklist predicate, so no file counts
	/// twice — the resume double-count was a review finding (2026-09-18).
	init(
		phase: ImportRun.Phase,
		totalFiles: Int,
		completedFiles: Int,
		skippedFiles: Int,
		settledFiles: Int,
		thumbnailFailures: Int,
		walkFailures: Int
	) {
		switch phase {
		case .walking:
			self = .walking
		case .finishing:
			self = .finishing
		case .importing:
			// An empty source counts nothing; never render a 0/0 frame.
			guard totalFiles > 0 else {
				self = .finishing
				return
			}
			self = .progress(
				ready: settledFiles,
				cataloged: completedFiles + skippedFiles,
				total: totalFiles
			)
		case .done(let outcome):
			self = .done(
				outcome: outcome,
				imported: completedFiles + skippedFiles,
				failures: walkFailures + thumbnailFailures
			)
		}
	}
}
