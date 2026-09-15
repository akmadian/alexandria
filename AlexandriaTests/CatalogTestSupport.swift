//
//  CatalogTestSupport.swift
//  AlexandriaTests
//
//  Shared fixtures for tests that drive the import write path against an
//  in-memory catalog.
//

import Foundation
import Testing
import GRDB
@testable import Alexandria

/// Thrown by `eventually` so a timed-out wait stops its test at the real
/// failure instead of cascading into secondary ones.
struct PollTimeout: Error {}

/// Polls until `condition` holds. Hub deliveries are async main-actor hops
/// away and catalog writes commit off the main actor, so assertions on either
/// must wait rather than sample once. On timeout it records the label and
/// THROWS.
@MainActor
func eventually(
	_ label: String,
	timeout: Duration = .seconds(2),
	_ condition: () async throws -> Bool
) async throws {
	let clock = ContinuousClock()
	let deadline = clock.now.advanced(by: timeout)
	while clock.now < deadline {
		if try await condition() { return }
		try await Task.sleep(for: .milliseconds(10))
	}
	Issue.record("timed out waiting for: \(label)")
	throw PollTimeout()
}

/// A file-less asset minted at a fixed instant, inserted through the raw
/// writer. This is the ONE fence exception for behavioral tests (besides
/// the schema-fence tests' raw SQL): view-state ordering tests need
/// id-CONTROLLED fixtures, and pipeline-minted UUIDv7 ids order randomly
/// within a millisecond. The trade is real: a file-less asset is a state
/// formation never produces and is invisible to folder/import sources —
/// fixtures that need source reach go through ImportContext instead.
nonisolated func seedAsset(
	_ catalog: Catalog, at seconds: TimeInterval
) async throws -> Identifier<Asset> {
	let id = Identifier<Asset>(rawValue: .v7(at: Date(timeIntervalSince1970: seconds)))
	let asset = Asset(id: id, kind: "image", rating: nil, flag: nil, representativeFileId: nil)
	try await catalog.databaseWriter.write { try asset.insert($0) }
	return id
}

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
