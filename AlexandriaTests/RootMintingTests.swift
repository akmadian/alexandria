//
//  RootMintingTests.swift
//  AlexandriaTests
//

import Foundation
import Testing
import GRDB
@testable import Alexandria

/// Phase 0 of an import run: volume recording, root folder minting, and the
/// imports-row bracket.
struct RootMintingTests {

	private func makeCatalog() throws -> Catalog {
		try Catalog(DatabaseQueue(path: ":memory:"))
	}

	private func makeObserved(identity: String? = "0FA1-MINT-TEST") -> ObservedVolume {
		ObservedVolume(
			identity: VolumeIdentity(uuid: identity),
			name: "Scratch SSD",
			kind: .external,
			volumeRootURL: URL(fileURLWithPath: "/Volumes/Scratch")
		)
	}

	@Test func identifiedVolumeIsFoundNotDuplicated() async throws {
		let catalog = try makeCatalog()
		let first = try await catalog.findOrCreateVolume(makeObserved())
		let second = try await catalog.findOrCreateVolume(makeObserved())
		#expect(first == second)
		let volumeCount = try await catalog.reader.read { database in
			try Int.fetchOne(database, sql: "SELECT COUNT(*) FROM volumes")
		}
		#expect(volumeCount == 1)
	}

	@Test func unidentifiedVolumesAlwaysMintFreshRows() async throws {
		let catalog = try makeCatalog()
		let first = try await catalog.findOrCreateVolume(makeObserved(identity: nil))
		let second = try await catalog.findOrCreateVolume(makeObserved(identity: nil))
		#expect(first != second)
	}

	@Test func rootFolderIsKeyedByVolumeAndPath() async throws {
		let catalog = try makeCatalog()
		let volumeId = try await catalog.findOrCreateVolume(makeObserved())
		let first = try await catalog.findOrCreateRootFolder(
			named: "2026-08 Iceland", on: volumeId, rootPath: "photos/2026-08 Iceland"
		)
		let again = try await catalog.findOrCreateRootFolder(
			named: "2026-08 Iceland", on: volumeId, rootPath: "photos/2026-08 Iceland"
		)
		let sibling = try await catalog.findOrCreateRootFolder(
			named: "2026-09 Faroes", on: volumeId, rootPath: "photos/2026-09 Faroes"
		)
		#expect(first == again)
		#expect(first != sibling)

		let fetched = try await catalog.reader.read { database in
			try Folder.fetchOne(database, key: first)
		}
		let root = try #require(fetched)
		#expect(root.parentId == nil)
		#expect(root.rootPath == "photos/2026-08 Iceland")
	}

	@Test func childFolderMintsUnderTheRoot() async throws {
		let catalog = try makeCatalog()
		let volumeId = try await catalog.findOrCreateVolume(makeObserved())
		let rootId = try await catalog.findOrCreateRootFolder(
			named: "photos", on: volumeId, rootPath: "photos"
		)
		let childId = try await catalog.findOrCreateFolder(named: "raw", under: rootId, on: volumeId)
		let childAgain = try await catalog.findOrCreateFolder(named: "raw", under: rootId, on: volumeId)
		#expect(childId == childAgain)
	}

	@Test func importBracketOpensWithNullFinishedAt() async throws {
		let catalog = try makeCatalog()
		let volumeId = try await catalog.findOrCreateVolume(makeObserved())
		let rootId = try await catalog.findOrCreateRootFolder(
			named: "photos", on: volumeId, rootPath: "photos"
		)
		let importId = Identifier<Import>.mint()
		try await catalog.recordImportStarted(id: importId, folderId: rootId)

		let row = try await catalog.reader.read { database in
			try Row.fetchOne(database, sql: "SELECT * FROM imports WHERE id = ?", arguments: [importId])
		}
		let imports = try #require(row)
		let startedAt: String = imports["started_at"]
		#expect(startedAt.hasSuffix("Z"))
		#expect(startedAt.contains("."))  // millisecond precision, per convention
		let finishedAt: String? = imports["finished_at"]
		#expect(finishedAt == nil)
	}

	@Test func relativePathDerivation() {
		let observed = makeObserved()
		#expect(observed.relativePath(of: URL(fileURLWithPath: "/Volumes/Scratch/photos/2026")) == "photos/2026")
		#expect(observed.relativePath(of: URL(fileURLWithPath: "/Volumes/Scratch")) == "")
	}

	@Test func probingARealDirectoryYieldsAVolume() throws {
		let observed = try ObservedVolume(containing: FileManager.default.temporaryDirectory)
		#expect(observed.identity != nil)  // the boot volume has a filesystem UUID
		#expect(observed.kind == .local)
	}
}
