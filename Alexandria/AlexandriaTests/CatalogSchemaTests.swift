//
//  CatalogSchemaTests.swift
//  AlexandriaTests
//

import Foundation
import Testing
import GRDB
@testable import Alexandria

/// The v0 schema's fences, proven: every constraint that exists to stop bad
/// writes gets one test that fails if the fence comes down.
struct CatalogSchemaTests {

	private func makeCatalog() throws -> Catalog {
		try Catalog(DatabaseQueue(path: ":memory:"))
	}

	private struct SeededChain {
		let volumeID = UUID.v7().uuidString
		let folderID = UUID.v7().uuidString
		let importID = UUID.v7().uuidString
		let assetID = UUID.v7().uuidString
		let fileID = UUID.v7().uuidString
	}

	/// The minimal volume → folder → import → asset → file chain.
	private func seedChain(_ catalog: Catalog) throws -> SeededChain {
		let chain = SeededChain()
		try catalog.databaseWriter.write { database in
			try database.execute(
				sql: "INSERT INTO volumes (id, identity, name, kind) VALUES (?, ?, ?, ?)",
				arguments: [chain.volumeID, "0FA1-4E2B-FIXTURE", "Working SSD", "external"]
			)
			try database.execute(
				sql: "INSERT INTO folders (id, volume_id, parent_id, name, name_key, root_path) VALUES (?, ?, NULL, ?, ?, ?)",
				arguments: [chain.folderID, chain.volumeID, "2026-08 Iceland", "2026-08 Iceland", "photos/2026-08 Iceland"]
			)
			try database.execute(
				sql: "INSERT INTO imports (id, folder_id, started_at) VALUES (?, ?, ?)",
				arguments: [chain.importID, chain.folderID, "2026-09-10T05:00:00.000Z"]
			)
			try database.execute(
				sql: "INSERT INTO assets (id, kind) VALUES (?, ?)",
				arguments: [chain.assetID, "image"]
			)
			try database.execute(
				sql: """
				INSERT INTO files (id, folder_id, asset_id, import_id, name, name_key, stem, extension, kind, size_bytes, modified_at, content_hash)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				""",
				arguments: [
					chain.fileID, chain.folderID, chain.assetID, chain.importID,
					"DSC_0142.RAF", "DSC_0142.RAF", "dsc_0142", "raf", "image",
					34_512_128, "2026-08-14T09:31:02.000Z", "f00dfeed",
				]
			)
		}
		return chain
	}

	@Test func migrationCreatesEveryTableAndEnforcesForeignKeys() throws {
		let catalog = try makeCatalog()
		try catalog.databaseWriter.read { database in
			for table in ["volumes", "folders", "imports", "assets", "files", "import_errors", "file_errors"] {
				#expect(try database.tableExists(table), "missing table: \(table)")
			}
			let foreignKeysEnabled = try Bool.fetchOne(database, sql: "PRAGMA foreign_keys")
			#expect(foreignKeysEnabled == true)
		}
	}

	@Test func importedChainReadsBackIntact() throws {
		let catalog = try makeCatalog()
		let chain = try seedChain(catalog)
		let row = try catalog.databaseWriter.read { database in
			try Row.fetchOne(database, sql: "SELECT * FROM files WHERE id = ?", arguments: [chain.fileID])
		}
		let file = try #require(row)
		let name: String = file["name"]
		let stem: String = file["stem"]
		let assetID: String = file["asset_id"]
		let missing: Bool = file["missing"]
		#expect(name == "DSC_0142.RAF")
		#expect(stem == "dsc_0142")
		#expect(assetID == chain.assetID)
		#expect(missing == false)
	}

	@Test func nonSidecarFileCannotCommitWithoutAnAsset() throws {
		let catalog = try makeCatalog()
		let chain = try seedChain(catalog)
		#expect(throws: DatabaseError.self) {
			try catalog.databaseWriter.write { database in
				try database.execute(
					sql: """
					INSERT INTO files (id, folder_id, asset_id, import_id, name, name_key, stem, extension, kind, size_bytes, modified_at)
					VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?)
					""",
					arguments: [
						UUID.v7().uuidString, chain.folderID, chain.importID,
						"DSC_0143.RAF", "DSC_0143.RAF", "dsc_0143", "raf", "image",
						1024, "2026-08-14T09:31:04.000Z",
					]
				)
			}
		}
	}

	@Test func sidecarFileCommitsUnattached() throws {
		let catalog = try makeCatalog()
		let chain = try seedChain(catalog)
		try catalog.databaseWriter.write { database in
			try database.execute(
				sql: """
				INSERT INTO files (id, folder_id, asset_id, import_id, name, name_key, stem, extension, kind, size_bytes, modified_at)
				VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?)
				""",
				arguments: [
					UUID.v7().uuidString, chain.folderID, chain.importID,
					"DSC_0142.xmp", "DSC_0142.xmp", "dsc_0142", "xmp", "sidecar",
					4096, "2026-08-14T09:31:03.000Z",
				]
			)
		}
		let sidecarCount = try catalog.databaseWriter.read { database in
			try Int.fetchOne(database, sql: "SELECT COUNT(*) FROM files WHERE kind = 'sidecar'")
		}
		#expect(sidecarCount == 1)
	}

	@Test func folderShapeConstraintHolds() throws {
		let catalog = try makeCatalog()
		let chain = try seedChain(catalog)

		// A root without a root_path is refused.
		#expect(throws: DatabaseError.self) {
			try catalog.databaseWriter.write { database in
				try database.execute(
					sql: "INSERT INTO folders (id, volume_id, parent_id, name, name_key, root_path) VALUES (?, ?, NULL, ?, ?, NULL)",
					arguments: [UUID.v7().uuidString, chain.volumeID, "loose", "loose"]
				)
			}
		}
		// A child carrying a root_path is refused.
		#expect(throws: DatabaseError.self) {
			try catalog.databaseWriter.write { database in
				try database.execute(
					sql: "INSERT INTO folders (id, volume_id, parent_id, name, name_key, root_path) VALUES (?, ?, ?, ?, ?, ?)",
					arguments: [UUID.v7().uuidString, chain.volumeID, chain.folderID, "raw", "raw", "photos/raw"]
				)
			}
		}
		// A proper child commits.
		try catalog.databaseWriter.write { database in
			try database.execute(
				sql: "INSERT INTO folders (id, volume_id, parent_id, name, name_key, root_path) VALUES (?, ?, ?, ?, ?, NULL)",
				arguments: [UUID.v7().uuidString, chain.volumeID, chain.folderID, "raw", "raw"]
			)
		}
	}

	@Test func ratingIsBoundedAndNullable() throws {
		let catalog = try makeCatalog()
		let chain = try seedChain(catalog)
		for invalidRating in [0, 6] {
			#expect(throws: DatabaseError.self) {
				try catalog.databaseWriter.write { database in
					try database.execute(
						sql: "UPDATE assets SET rating = ? WHERE id = ?",
						arguments: [invalidRating, chain.assetID]
					)
				}
			}
		}
		try catalog.databaseWriter.write { database in
			try database.execute(sql: "UPDATE assets SET rating = 3 WHERE id = ?", arguments: [chain.assetID])
			try database.execute(sql: "UPDATE assets SET rating = NULL WHERE id = ?", arguments: [chain.assetID])
		}
	}

	@Test func deletionOfReferencedRowsIsRestricted() throws {
		let catalog = try makeCatalog()
		let chain = try seedChain(catalog)
		let restrictedDeletions = [
			("DELETE FROM folders WHERE id = ?", chain.folderID),
			("DELETE FROM imports WHERE id = ?", chain.importID),
			("DELETE FROM assets WHERE id = ?", chain.assetID),
			("DELETE FROM volumes WHERE id = ?", chain.volumeID),
		]
		for (sql, id) in restrictedDeletions {
			#expect(throws: DatabaseError.self, "should be fenced: \(sql)") {
				try catalog.databaseWriter.write { database in
					try database.execute(sql: sql, arguments: [id])
				}
			}
		}
	}

	@Test func deletingAFileReleasesItsOverrideAndItsErrorRows() throws {
		let catalog = try makeCatalog()
		let chain = try seedChain(catalog)
		try catalog.databaseWriter.write { database in
			try database.execute(
				sql: "UPDATE assets SET representative_file_id = ? WHERE id = ?",
				arguments: [chain.fileID, chain.assetID]
			)
			try database.execute(
				sql: "INSERT INTO file_errors (file_id, task, reason_code, message) VALUES (?, 'thumbnail', 'decode_failed', 'truncated file')",
				arguments: [chain.fileID]
			)
			try database.execute(sql: "DELETE FROM files WHERE id = ?", arguments: [chain.fileID])
		}
		try catalog.databaseWriter.read { database in
			let representative = try String.fetchOne(
				database,
				sql: "SELECT representative_file_id FROM assets WHERE id = ?",
				arguments: [chain.assetID]
			)
			#expect(representative == nil)
			let orphanedErrors = try Int.fetchOne(database, sql: "SELECT COUNT(*) FROM file_errors")
			#expect(orphanedErrors == 0)
		}
	}

	@Test func duplicateFileIdentityWithinAFolderIsRejected() throws {
		let catalog = try makeCatalog()
		let chain = try seedChain(catalog)
		#expect(throws: DatabaseError.self) {
			try catalog.databaseWriter.write { database in
				try database.execute(
					sql: """
					INSERT INTO files (id, folder_id, asset_id, import_id, name, name_key, stem, extension, kind, size_bytes, modified_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
					""",
					arguments: [
						UUID.v7().uuidString, chain.folderID, chain.assetID, chain.importID,
						"DSC_0142.RAF", "DSC_0142.RAF", "dsc_0142", "raf", "image",
						1, "2026-08-14T09:31:05.000Z",
					]
				)
			}
		}
	}

	@Test func unidentifiedVolumesNeverCollideButIdentifiedOnesDo() throws {
		let catalog = try makeCatalog()
		try catalog.databaseWriter.write { database in
			for _ in 0..<2 {
				try database.execute(
					sql: "INSERT INTO volumes (id, identity, name, kind) VALUES (?, NULL, ?, 'network')",
					arguments: [UUID.v7().uuidString, "unidentified share"]
				)
			}
			try database.execute(
				sql: "INSERT INTO volumes (id, identity, name, kind) VALUES (?, ?, ?, 'network')",
				arguments: [UUID.v7().uuidString, "smb://nas/photos", "NAS"]
			)
		}
		#expect(throws: DatabaseError.self) {
			try catalog.databaseWriter.write { database in
				try database.execute(
					sql: "INSERT INTO volumes (id, identity, name, kind) VALUES (?, ?, ?, 'network')",
					arguments: [UUID.v7().uuidString, "smb://nas/photos", "NAS again"]
				)
			}
		}
	}
}
