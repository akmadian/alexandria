//
//  CatalogTestSupport.swift
//  AlexandriaTests
//
//  Shared fixtures for tests that drive the import write path against an
//  in-memory catalog.
//

import Foundation
import GRDB
@testable import Alexandria

/// An in-memory catalog with one volume, one tracked root, and an open
/// import bracket — the substrate recordNewFileBatch and AssetFormation
/// operate on.
struct ImportContext {
	let catalog: Catalog
	let importId: Identifier<Import>
	let rootFolderId: Identifier<Folder>
	let rootURL: URL

	static func make() async throws -> ImportContext {
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
		return ImportContext(
			catalog: catalog, importId: importId,
			rootFolderId: rootFolderId, rootURL: rootURL
		)
	}

	/// A PreparedFile as the pipeline would mint it: registry-resolved
	/// format, ratified stem/extension derivation.
	func prepared(
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

	@discardableResult
	func record(_ files: [PreparedFile]) async throws -> BatchOutcome {
		try await catalog.recordNewFileBatch(
			files, importId: importId,
			rootFolderId: rootFolderId, rootUrl: rootURL
		)
	}

	/// One formation pass, composed the way ImportRun composes it: read,
	/// decide, persist.
	@discardableResult
	func form() async throws -> AssetFormation.Resolution {
		let files = try await catalog.files(inImport: importId)
		let resolution = AssetFormation.form(files: files)
		try await catalog.recordFormedAssets(resolution.clusters)
		return resolution
	}
}
