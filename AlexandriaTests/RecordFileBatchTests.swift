//
//  RecordFileBatchTests.swift
//  AlexandriaTests
//

import Foundation
import Testing
import GRDB
@testable import Alexandria

/// recordNewFileBatch's invariants: files commit unformed, skip-existing,
/// folder chains, error residue — proven against an in-memory catalog.
/// Formation's behavior lives in AssetFormationTests.
struct RecordFileBatchTests {

	@Test func filesRecordUnformedAcrossAllKinds() async throws {
		let context = try await ImportContext.make()
		let outcome = try await context.record([
			context.prepared("/Volumes/Test/Shoot/_DSF0796.RAF"),
			context.prepared("/Volumes/Test/Shoot/_DSF0796.xmp"),
		])
		#expect(outcome.recorded == 2)
		#expect(outcome.skipped == 0)

		let pending = try await context.catalog.reader.read { database in
			try Int.fetchOne(database, sql: "SELECT COUNT(*) FROM files WHERE asset_id IS NULL AND formation_rule IS NULL")
		}
		#expect(pending == 2)
		let assetCount = try await context.catalog.reader.read { database in
			try Int.fetchOne(database, sql: "SELECT COUNT(*) FROM assets")
		}
		#expect(assetCount == 0)
	}

	@Test func alreadyCatalogedFilesSkipWithoutError() async throws {
		let context = try await ImportContext.make()
		let batch = [
			context.prepared("/Volumes/Test/Shoot/a.jpg"),
			context.prepared("/Volumes/Test/Shoot/b.jpg"),
		]
		_ = try await context.record(batch)
		let second = try await context.record(batch)
		#expect(second.recorded == 0)
		#expect(second.skipped == 2)

		let fileCount = try await context.catalog.reader.read { database in
			try Int.fetchOne(database, sql: "SELECT COUNT(*) FROM files")
		}
		#expect(fileCount == 2)
	}

	@Test func folderChainsMintOnceAndNest() async throws {
		let context = try await ImportContext.make()
		_ = try await context.record([
			context.prepared("/Volumes/Test/Shoot/day1/raw/a.jpg"),
			context.prepared("/Volumes/Test/Shoot/day1/raw/b.jpg"),
			context.prepared("/Volumes/Test/Shoot/day1/c.jpg"),
		])

		// Root + day1 + raw, no duplicates from the repeated directory.
		let folders = try await context.catalog.reader.read { database in
			try Row.fetchAll(database, sql: "SELECT id, parent_id, name FROM folders")
		}
		#expect(folders.count == 3)
		let day1 = try #require(folders.first { $0["name"] == "day1" })
		let raw = try #require(folders.first { $0["name"] == "raw" })
		#expect(day1["parent_id"] == context.rootFolderId.rawValue.uuidString)
		#expect(raw["parent_id"] == (day1["id"] as String))

		let fileFolders = try await context.catalog.reader.read { database in
			try Row.fetchAll(database, sql: "SELECT name, folder_id FROM files ORDER BY name")
		}
		#expect(fileFolders[0]["folder_id"] == (raw["id"] as String))
		#expect(fileFolders[2]["folder_id"] == (day1["id"] as String))
	}

	@Test func extractionFailureLeavesResidueButStillRecords() async throws {
		let context = try await ImportContext.make()
		let outcome = try await context.record([
			context.prepared("/Volumes/Test/Shoot/truncated.jpg", extractionFailed: true)
		])
		#expect(outcome.recorded == 1)
		#expect(outcome.failed == 1)

		let residue = try await context.catalog.reader.read { database in
			try Row.fetchOne(database, sql: """
				SELECT file_errors.task, file_errors.reason_code, files.metadata
				FROM file_errors JOIN files ON files.id = file_errors.file_id
				""")
		}
		let row = try #require(residue)
		#expect(row["task"] == "metadata")
		#expect(row["reason_code"] == "decode_failed")
		#expect((row["metadata"] as String?) == nil)
	}

	@Test func walkFailuresLandInTheImportDLQ() async throws {
		let context = try await ImportContext.make()
		try await context.catalog.recordImportErrors(
			[("day1/locked.jpg", "read_failed", "permission denied")],
			importId: context.importId
		)
		let row = try await context.catalog.reader.read { database in
			try Row.fetchOne(database, sql: "SELECT path, reason_code, attempts FROM import_errors")
		}
		let residue = try #require(row)
		#expect(residue["path"] == "day1/locked.jpg")
		#expect(residue["reason_code"] == "read_failed")
		#expect(residue["attempts"] == 1)
	}

	@Test func stemDerivationFollowsTheFinalDotRule() {
		#expect(ImportRun.splitStem("Photo.RAW.XMP") == ("photo.raw", "xmp"))
		#expect(ImportRun.splitStem("IMG_0001.JPG") == ("img_0001", "jpg"))
		#expect(ImportRun.splitStem("README") == ("readme", ""))
		#expect(ImportRun.splitStem(".hidden") == (".hidden", ""))
		#expect(ImportRun.splitStem("trailing.") == ("trailing", ""))
	}
}
