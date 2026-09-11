//
//  RecordFileBatchTests.swift
//  AlexandriaTests
//

import Foundation
import Testing
import GRDB
@testable import Alexandria

/// recordNewFileBatch's invariants: skip-existing, sidecars unattached,
/// folder chains, error residue — each proven against an in-memory catalog.
struct RecordFileBatchTests {

	private struct Context {
		let catalog: Catalog
		let importId: Identifier<Import>
		let rootFolderId: Identifier<Folder>
		let rootURL: URL
	}

	private func makeContext() async throws -> Context {
		let catalog = try Catalog(DatabaseQueue(path: ":memory:"))
		let rootURL = URL(fileURLWithPath: "/Volumes/Test/Shoot")
		let volumeId = try await catalog.findOrCreateVolume(ObservedVolume(
			identity: .filesystemUUID("0FA1-BATCH-FIXTURE"),
			name: "Test",
			kind: .external,
			volumeRootURL: URL(fileURLWithPath: "/Volumes/Test")
		))
		let rootFolderId = try await catalog.findOrCreateRootFolder(
			named: "Shoot", on: volumeId, rootPath: "Shoot"
		)
		let importId = Identifier<Import>.mint()
		try await catalog.recordImportStarted(id: importId, folderId: rootFolderId)
		return Context(
			catalog: catalog, importId: importId,
			rootFolderId: rootFolderId, rootURL: rootURL
		)
	}

	/// A PreparedFile as the pipeline would mint it: registry-resolved format,
	/// ratified stem/extension derivation.
	private func prepared(
		_ path: String,
		metadata: FileMetadata? = nil,
		extractionFailed: Bool = false
	) -> PreparedFile {
		let url = URL(fileURLWithPath: path)
		let name = url.lastPathComponent
		let nameKey = name.precomposedStringWithCanonicalMapping
		let (stem, ext) = ImportRun.splitStem(nameKey)
		return PreparedFile(
			discovered: DiscoveredFile(
				url: url, size: 1024, modifiedAt: Date(timeIntervalSince1970: 1_700_000_000),
				format: FileFormat.resolve(extension: url.pathExtension, contentType: nil)
			),
			name: name, nameKey: nameKey, fileStem: stem, fileExtension: ext,
			contentHash: "deadbeef", metadata: metadata, extractionFailed: extractionFailed
		)
	}

	private func record(_ files: [PreparedFile], in context: Context) async throws -> BatchOutcome {
		try await context.catalog.recordNewFileBatch(
			files, importId: context.importId,
			rootFolderId: context.rootFolderId, rootUrl: context.rootURL
		)
	}

	@Test func nonSidecarsGetAssetsAndSidecarsStayUnattached() async throws {
		let context = try await makeContext()
		let outcome = try await record([
			prepared("/Volumes/Test/Shoot/_DSF0796.RAF"),
			prepared("/Volumes/Test/Shoot/_DSF0796.xmp"),
		], in: context)
		#expect(outcome.recorded == 2)
		#expect(outcome.skipped == 0)

		let rows = try await context.catalog.reader.read { database in
			try Row.fetchAll(database, sql: """
				SELECT kind, asset_id, formation_rule FROM files ORDER BY kind
				""")
		}
		#expect(rows.count == 2)
		let image = rows[0], sidecar = rows[1]
		#expect(image["kind"] == "image")
		#expect((image["asset_id"] as String?) != nil)
		#expect(image["formation_rule"] == "one_asset_per_file")
		#expect(sidecar["kind"] == "sidecar")
		#expect((sidecar["asset_id"] as String?) == nil)
		#expect((sidecar["formation_rule"] as String?) == nil)

		let assetCount = try await context.catalog.reader.read { database in
			try Int.fetchOne(database, sql: "SELECT COUNT(*) FROM assets")
		}
		#expect(assetCount == 1)
	}

	@Test func alreadyCatalogedFilesSkipWithoutError() async throws {
		let context = try await makeContext()
		let batch = [
			prepared("/Volumes/Test/Shoot/a.jpg"),
			prepared("/Volumes/Test/Shoot/b.jpg"),
		]
		_ = try await record(batch, in: context)
		let second = try await record(batch, in: context)
		#expect(second.recorded == 0)
		#expect(second.skipped == 2)

		let fileCount = try await context.catalog.reader.read { database in
			try Int.fetchOne(database, sql: "SELECT COUNT(*) FROM files")
		}
		#expect(fileCount == 2)
	}

	@Test func folderChainsMintOnceAndNest() async throws {
		let context = try await makeContext()
		_ = try await record([
			prepared("/Volumes/Test/Shoot/day1/raw/a.jpg"),
			prepared("/Volumes/Test/Shoot/day1/raw/b.jpg"),
			prepared("/Volumes/Test/Shoot/day1/c.jpg"),
		], in: context)

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

	@Test func extractionFailureLeavesResidueButStillMints() async throws {
		let context = try await makeContext()
		let outcome = try await record([
			prepared("/Volumes/Test/Shoot/truncated.jpg", extractionFailed: true)
		], in: context)
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
		let context = try await makeContext()
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
