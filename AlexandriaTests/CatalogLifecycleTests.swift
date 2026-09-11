//
//  CatalogLifecycleTests.swift
//  AlexandriaTests
//

import Foundation
import Testing
import GRDB
@testable import Alexandria

struct CatalogLifecycleTests {

	@Test func openCreatesTheLayoutAndReopensWithData() throws {
		let directory = FileManager.default.temporaryDirectory
			.appending(path: "lifecycle-\(UUID().uuidString)")
		defer { try? FileManager.default.removeItem(at: directory) }

		let catalog = try Catalog.open(at: directory)
		let packageURL = directory.appending(path: "\(directory.lastPathComponent).alxcat")
		#expect(FileManager.default.fileExists(atPath: packageURL.appending(path: "catalog.sqlite").path))
		#expect(FileManager.default.fileExists(atPath: directory.appending(path: "Thumbnails").path))
		#expect(catalog.directory == directory)

		try catalog.databaseWriter.write { database in
			try database.execute(
				sql: "INSERT INTO volumes (id, identity, name, kind) VALUES (?, ?, ?, ?)",
				arguments: [UUID.v7().uuidString, "0FA1-LIFECYCLE", "Scratch", "external"]
			)
		}

		let reopened = try Catalog.open(at: directory)
		let journalMode = try reopened.databaseWriter.read { database in
			try String.fetchOne(database, sql: "PRAGMA journal_mode")
		}
		#expect(journalMode == "wal")
		let volumeCount = try reopened.databaseWriter.read { database in
			try Int.fetchOne(database, sql: "SELECT COUNT(*) FROM volumes")
		}
		#expect(volumeCount == 1)
	}
}
