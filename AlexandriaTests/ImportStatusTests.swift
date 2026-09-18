//
//  ImportStatusTests.swift
//  AlexandriaTests
//
//  The display arithmetic (import status round, 2026-09-18): the one
//  derivation from run counters to what the folder row shows. `ready` is
//  the settled count alone — the review caught the previous
//  thumbnailed+skipped sum double-counting on a resume and never filling
//  for unthumbnailable kinds; these tests pin the corrected invariant
//  (full ⇔ complete) from both sides.
//

import Foundation
import GRDB
import Testing
@testable import Alexandria

struct ImportStatusTests {

	private func status(
		phase: ImportRun.Phase,
		total: Int = 0,
		completed: Int = 0,
		skipped: Int = 0,
		settled: Int = 0,
		thumbnailFailures: Int = 0,
		walkFailures: Int = 0
	) -> ImportStatus {
		ImportStatus(
			phase: phase,
			totalFiles: total,
			completedFiles: completed,
			skippedFiles: skipped,
			settledFiles: settled,
			thumbnailFailures: thumbnailFailures,
			walkFailures: walkFailures
		)
	}

	@Test func indeterminatePhasesPassThrough() {
		#expect(status(phase: .walking) == .walking)
		#expect(status(phase: .finishing, total: 100, completed: 100) == .finishing)
	}

	@Test func freshRunCountsPlainly() {
		#expect(
			status(phase: .importing, total: 1000, completed: 340, settled: 220)
				== .progress(ready: 220, cataloged: 340, total: 1000)
		)
	}

	/// The resume regression (review finding, 2026-09-18): crash at 900
	/// cataloged / 200 thumbnailed, resume. The batch pass settles the 200
	/// already-stamped skips; the drain settles the other 700 skips plus
	/// 100 new files as it goes. Ready must track settled alone — the old
	/// thumbnailed+skipped sum read 1600/1000 and pegged the ring full a
	/// third of the way through the drain.
	@Test func resumedRunNeverExceedsTotal() {
		// Mid-drain: all 1000 cataloged (100 new + 900 skips), 200 settled
		// at batch time, 500 settled by the drain so far.
		let midDrain = status(
			phase: .importing, total: 1000,
			completed: 100, skipped: 900, settled: 700
		)
		#expect(midDrain == .progress(ready: 700, cataloged: 1000, total: 1000))
		// Drain done: exactly full, exactly once.
		let drained = status(
			phase: .importing, total: 1000,
			completed: 100, skipped: 900, settled: 1000
		)
		#expect(drained == .progress(ready: 1000, cataloged: 1000, total: 1000))
	}

	/// An empty source counts nothing — never a 0/0 determinate frame.
	@Test func zeroTotalNeverRendersDeterminate() {
		#expect(status(phase: .importing, total: 0) == .finishing)
	}

	@Test func doneCarriesOutcomeAndResidueCounts() {
		#expect(
			status(phase: .done(.completed), total: 1000, completed: 95, skipped: 900,
			       thumbnailFailures: 3, walkFailures: 2)
				== .done(outcome: .completed, imported: 995, failures: 5)
		)
		#expect(
			status(phase: .done(.canceled), completed: 400)
				== .done(outcome: .canceled, imported: 400, failures: 0)
		)
	}
}

/// The three-truth precedence (ruling 1): both view call sites route
/// through ImportStatus.current, so this pins the round's headline rule —
/// a run, even a lingering done one, outranks the durable unfinished flag.
@MainActor
struct ImportStatusPrecedenceTests {

	@Test func flagAloneReadsNeedsResume() {
		#expect(ImportStatus.current(run: nil, unfinished: true) == .needsResume)
	}

	@Test func nothingReadsNothing() {
		#expect(ImportStatus.current(run: nil, unfinished: false) == nil)
	}

	@Test func runOutranksTheFlag() async throws {
		// A resumed run is .walking before its bracket write — while the DB
		// still says unfinished. The run must win or a resume flashes the
		// alert badge (ruling 4's ordering exists for exactly this).
		let storeDirectory = FileManager.default.temporaryDirectory
			.appending(path: "import-status-\(UUID().uuidString)")
		try FileManager.default.createDirectory(at: storeDirectory, withIntermediateDirectories: true)
		defer { try? FileManager.default.removeItem(at: storeDirectory) }
		let run = ImportRun(
			id: .mint(),
			folderUrl: URL(fileURLWithPath: "/tmp"),
			rootFolderId: .mint(),
			volume: try ObservedVolume(containing: URL(fileURLWithPath: "/")),
			catalog: try Catalog(DatabaseQueue()),
			store: ThumbnailStore(catalogDirectory: storeDirectory),
			resuming: true
		)
		// Never started: phase stays .walking, nothing touches the catalog.
		#expect(ImportStatus.current(run: run, unfinished: true) == .walking)
	}
}
