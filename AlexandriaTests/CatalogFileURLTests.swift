//
//  CatalogFileURLTests.swift
//  AlexandriaTests
//

import Foundation
import Testing
import GRDB
@testable import Alexandria

struct CatalogFileURLTests {

	/// The happy path: the file's URL composes the folder's reconstructed
	/// volume-relative path (root_path + the ancestor names) under the volume's
	/// live mount point. Anchored on the boot volume — always mounted — so the
	/// mount resolves; the expected URL is built from currentMountURL too, so
	/// the assertion isolates the walk-and-compose logic from the machine's
	/// specific mount path.
	@Test func currentURLComposesTheReconstructedPathUnderTheMount() async throws {
		let bootUUID = try #require(
			try URL(fileURLWithPath: "/")
				.resourceValues(forKeys: [.volumeUUIDStringKey]).volumeUUIDString
		)
		let context = try await ImportContext.make()
		// root "Shoot" (root_path "Shoot") → child "Sub" → a.jpg.
		try await context.record([context.prepared("/Volumes/Test/Shoot/Sub/a.jpg")])
		try await context.form()
		// Re-identify the fixture volume as the boot volume so the mount resolves.
		try await context.catalog.databaseWriter.write { db in
			try db.execute(sql: "UPDATE volumes SET identity = ?", arguments: [bootUUID])
		}

		let fileID = try await context.catalog.reader.read { db in
			try File.fetchAll(db).first { $0.name == "a.jpg" }!.id
		}
		let url = try await context.catalog.reader.read { db in
			try Catalog.fileURL(db, of: fileID)
		}

		let mount = try #require(currentMountURL(of: .filesystemUUID(bootUUID)))
		#expect(url == mount
			.appending(path: "Shoot")
			.appending(path: "Sub")
			.appending(path: "a.jpg"))
	}

	/// An unmounted volume resolves to nil — the offline case the loupe renders
	/// as a placeholder. The fixture volume's identity matches nothing mounted.
	@Test func currentURLIsNilWhenTheVolumeIsNotMounted() async throws {
		let context = try await ImportContext.make()
		try await context.record([context.prepared("/Volumes/Test/Shoot/a.jpg")])
		try await context.form()
		let fileID = try await context.catalog.reader.read { db in
			try File.fetchAll(db).first!.id
		}
		let url = try await context.catalog.reader.read { db in
			try Catalog.fileURL(db, of: fileID)
		}
		#expect(url == nil)
	}
}
