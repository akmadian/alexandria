//
//  ImportResumeTests.swift
//  AlexandriaTests
//
//  The resume ruling (2026-09-11): a run is identified by its source, so
//  restarting an import of the same directory picks the unfinished job back
//  up under its original id. Query behavior against an in-memory catalog;
//  the full pickup end-to-end against real TestData fixtures.
//

import Foundation
import GRDB
import Testing
@testable import Alexandria

/// repo-root/TestData, resolved from this source file's location.
private nonisolated var testData: URL {
	URL(fileURLWithPath: #filePath)
		.deletingLastPathComponent()   // AlexandriaTests/
		.deletingLastPathComponent()   // repo root
		.appending(path: "TestData")
}

struct UnfinishedImportTests {

	@Test func interruptedImportIsTheUnfinishedJob() async throws {
		let context = try await ImportContext.make()
		// ImportContext opens a bracket and never finishes it: interrupted.
		let found = try await context.catalog.unfinishedImport(inFolder: context.rootFolderId)
		#expect(found == context.importId)
	}

	@Test func completedImportYieldsNil() async throws {
		let context = try await ImportContext.make()
		try await context.catalog.recordImportFinished(id: context.importId, outcome: .completed)
		let found = try await context.catalog.unfinishedImport(inFolder: context.rootFolderId)
		#expect(found == nil)
	}

	@Test func canceledAndFailedCountAsUnfinished() async throws {
		let context = try await ImportContext.make()
		for outcome in [ImportOutcome.canceled, .failed] {
			try await context.catalog.recordImportFinished(id: context.importId, outcome: outcome)
			let found = try await context.catalog.unfinishedImport(inFolder: context.rootFolderId)
			#expect(found == context.importId, "\(outcome) should be resumable")
		}
	}

	@Test func latestImportDecidesNotHistory() async throws {
		let context = try await ImportContext.make()
		try await context.catalog.recordImportFinished(id: context.importId, outcome: .completed)
		// A later interrupted attempt on the same root: that one is the job.
		let second = Identifier<Import>.mint()
		try await context.catalog.recordImportStarted(id: second, folderId: context.rootFolderId)
		let found = try await context.catalog.unfinishedImport(inFolder: context.rootFolderId)
		#expect(found == second)
	}

	@Test func resumeReopensTheBracket() async throws {
		let context = try await ImportContext.make()
		try await context.catalog.recordImportFinished(id: context.importId, outcome: .canceled)
		try await context.catalog.recordImportResumed(id: context.importId)
		let row = try await context.catalog.reader.read { database in
			try Row.fetchOne(database, sql: "SELECT finished_at, outcome FROM imports")
		}
		#expect(row?["finished_at"] == nil as String?)
		#expect(row?["outcome"] == nil as String?)
	}
}

struct ImportResumeEndToEndTests {

	/// The full pickup: an interrupted import exists for TestData's root, so
	/// ImportService.startImport adopts its id, the run walks and records the
	/// files under it, and the bracket completes with every thumbnail
	/// resolved — stamped or residue, nothing pending.
	@MainActor
	@Test func restartingASourcePicksTheUnfinishedImportBackUp() async throws {
		let catalogDirectory = FileManager.default.temporaryDirectory
			.appending(path: "import-resume-\(UUID().uuidString)")
		try FileManager.default.createDirectory(at: catalogDirectory, withIntermediateDirectories: true)
		defer { try? FileManager.default.removeItem(at: catalogDirectory) }
		let catalog = try Catalog.open(at: catalogDirectory)

		// Manufacture the interrupted state the way a dead process leaves it:
		// a bracket opened for this source's root folder, never finished.
		let observed = try ObservedVolume(containing: testData)
		let volumeId = try await catalog.findOrCreateVolume(observed)
		let rootFolderId = try await catalog.findOrCreateRootFolder(
			named: testData.lastPathComponent, on: volumeId,
			rootPath: observed.relativePath(of: testData)
		)
		let interrupted = Identifier<Import>.mint()
		try await catalog.recordImportStarted(id: interrupted, folderId: rootFolderId)

		let run = try await ImportService(catalog: catalog).startImport(of: testData)
		#expect(run.id == interrupted)

		// The run completes in the background; the bracket stamping is the
		// completion signal.
		let deadline = Date().addingTimeInterval(60)
		var outcome: String? = nil
		while Date() < deadline {
			outcome = try await catalog.reader.read { database in
				try String.fetchOne(database, sql: "SELECT outcome FROM imports WHERE id = ?",
				                    arguments: [interrupted])
			}
			if outcome != nil { break }
			try await Task.sleep(for: .milliseconds(100))
		}
		#expect(outcome == "completed")

		// One import row (no sibling minted), and every thumbnail resolved.
		let importCount = try await catalog.reader.read { database in
			try Int.fetchOne(database, sql: "SELECT COUNT(*) FROM imports")
		}
		#expect(importCount == 1)
		let pending = try await catalog.thumbnailPending(
			inImport: interrupted, under: rootFolderId, limit: 10
		)
		#expect(pending.isEmpty)
		let counts = try await catalog.reader.read { database in
			try Row.fetchOne(database, sql: """
				SELECT sum(thumbnail_at IS NOT NULL) AS stamped,
				       (SELECT COUNT(*) FROM file_errors WHERE task = 'thumbnail') AS residue
				FROM files
				""")
		}
		#expect((counts?["stamped"] ?? 0) > 0)
		#expect((counts?["residue"] ?? 0) > 0)  // truncated.jpg and friends
	}
}
