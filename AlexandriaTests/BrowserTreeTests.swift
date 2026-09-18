//
//  BrowserTreeTests.swift
//  AlexandriaTests
//
//  The sidebar tree's pure value side: assembly from the volumes and
//  folders tables (nesting, deterministic ordering, multiple volumes) and
//  the filter's keep-matches-and-ancestors behavior — no view, no
//  observation in the loop.
//

import Foundation
import Testing
import GRDB
@testable import Alexandria

struct BrowserTreeTests {

	/// One volume ("Test") with root "Shoot" containing "Sub", plus a
	/// second volume ("Zeta") with root "Alpha" — enough shape for
	/// nesting, ordering, and sectioning assertions.
	private func makeFixtureTree() async throws -> BrowserTree {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/a.jpg"),
			context.prepared("/Volumes/Test/Shoot/Sub/d.jpg"),
		])
		let zeta = try await context.catalog.findOrCreateVolume(ObservedVolume(
			identity: .filesystemUUID("ZETA-FIXTURE"),
			name: "Zeta",
			kind: .external,
			volumeRootURL: URL(fileURLWithPath: "/Volumes/Zeta")
		))
		_ = try await context.catalog.findOrCreateRootFolder(
			named: "Alpha", on: zeta, rootPath: "Alpha"
		)
		return try await context.catalog.databaseWriter.read { try BrowserTree.fetch($0) }
	}

	@Test func assemblyNestsAndOrdersDeterministically() async throws {
		let tree = try await makeFixtureTree()

		// Volumes name-ordered: Test before Zeta.
		#expect(tree.volumes.map(\.name) == ["Test", "Zeta"])

		let test = tree.volumes[0]
		#expect(test.roots.map(\.name) == ["Shoot"])
		// The import minted Sub as a child of Shoot; assembly nests it.
		#expect(test.roots[0].children.map(\.name) == ["Sub"])
		#expect(test.roots[0].children[0].children.isEmpty)

		let zeta = tree.volumes[1]
		#expect(zeta.roots.map(\.name) == ["Alpha"])

		// The header's inputs ride the node (volume monitoring, chunk two):
		// identity and kind survive assembly.
		#expect(zeta.identity == .filesystemUUID("ZETA-FIXTURE"))
		#expect(zeta.kind == .external)
	}

	/// The title lookup over folders: nested nodes resolve by name, static
	/// sources answer nil (they name themselves), and a miss is nil. The
	/// collection side of the walk pins in the collections-assembly test,
	/// which already builds a nested collection tree.
	@Test func nameOfSourceWalksTheWholeTree() async throws {
		let tree = try await makeFixtureTree()
		let sub = tree.volumes[0].roots[0].children[0]
		#expect(tree.name(of: .folder(sub.id)) == "Sub")
		#expect(tree.name(of: .library) == nil)
		#expect(tree.name(of: .folder(.mint())) == nil)
	}

	@Test func outlineChildrenSpellsLeavesAsNil() async throws {
		let tree = try await makeFixtureTree()
		let shoot = tree.volumes[0].roots[0]
		#expect(shoot.outlineChildren?.count == 1)
		#expect(shoot.children[0].outlineChildren == nil)
	}

	@Test func filterKeepsMatchesAndTheirAncestors() async throws {
		let tree = try await makeFixtureTree()

		// Matching the nested folder keeps its ancestor so it stays
		// reachable, and drops the volume with no surviving roots.
		let sub = tree.filtered(by: "sub")
		#expect(sub.volumes.map(\.name) == ["Test"])
		#expect(sub.volumes[0].roots.map(\.name) == ["Shoot"])
		#expect(sub.volumes[0].roots[0].children.map(\.name) == ["Sub"])

		// A folder matching on its own name keeps its whole subtree (ruled
		// 2026-09-11): the sidebar must not show as a leaf what the grid
		// shows as a subtree.
		let shoot = tree.filtered(by: "shoot")
		#expect(shoot.volumes.map(\.name) == ["Test"])
		#expect(shoot.volumes[0].roots.map(\.name) == ["Shoot"])
		#expect(shoot.volumes[0].roots[0].children.map(\.name) == ["Sub"])

		// No match: everything drops.
		#expect(tree.filtered(by: "nope").volumes.isEmpty)

		// Empty and whitespace text return the tree untouched.
		#expect(tree.filtered(by: "") == tree)
		#expect(tree.filtered(by: "   ") == tree)
	}

	/// The collections section (collections round, chunk 4): same assembly
	/// shape as folders — roots are parentId NULL, siblings in Finder order
	/// ("Shoot 9" before "Shoot 10", disagreeing with lexicographic AND
	/// creation order), nesting recursive.
	@Test func collectionsAssembleRootedNestedAndFinderOrdered() async throws {
		let catalog = try Catalog(DatabaseQueue())
		let trips = try await catalog.createCollection(named: "Trips")
		let shoot10 = try await catalog.createCollection(named: "Shoot 10", under: trips.id)
		_ = try await catalog.createCollection(named: "Shoot 9", under: trips.id)
		_ = try await catalog.createCollection(named: "Grand", under: shoot10.id)
		_ = try await catalog.createCollection(named: "Picks")

		let tree = try await catalog.databaseWriter.read { try BrowserTree.fetch($0) }
		#expect(tree.collections.map(\.name) == ["Picks", "Trips"])
		let tripsNode = tree.collections[1]
		#expect(tripsNode.children.map(\.name) == ["Shoot 9", "Shoot 10"])
		#expect(tripsNode.children[1].children.map(\.name) == ["Grand"])

		// The title lookup's collection side rides the same nested tree.
		let grand = tripsNode.children[1].children[0]
		#expect(tree.name(of: .collection(grand.id)) == "Grand")
		#expect(tree.name(of: .collection(.mint())) == nil)

		// The folder filter leaves collections untouched (it's the FOLDER
		// filter by prompt; widening it is a future call, not a drift).
		#expect(tree.filtered(by: "nope").collections == tree.collections)
	}
}

/// The resume badge's durable truth (import status round, 2026-09-18):
/// unfinishedImports mirrors Catalog.unfinishedImport's latest-per-folder
/// predicate as a set for the tree observation.
struct BrowserTreeUnfinishedImportTests {

	@Test func interruptedImportFlagsItsRootFolder() async throws {
		// ImportContext opens a bracket and never finishes it: interrupted.
		let context = try await ImportContext.make()
		let tree = try await context.catalog.databaseWriter.read { try BrowserTree.fetch($0) }
		#expect(tree.unfinishedImports == [context.rootFolderId])
	}

	@Test func completedImportClearsTheFlag() async throws {
		let context = try await ImportContext.make()
		try await context.catalog.recordImportFinished(id: context.importId, outcome: .completed)
		let tree = try await context.catalog.databaseWriter.read { try BrowserTree.fetch($0) }
		#expect(tree.unfinishedImports.isEmpty)
	}

	/// Only the LATEST import decides — a failed history behind a completed
	/// latest is not a badge. Explicit timestamps keep the ordering out of
	/// same-millisecond tie-break territory.
	@Test func latestImportDecidesNotHistory() async throws {
		let context = try await ImportContext.make()
		try await context.catalog.recordImportFinished(id: context.importId, outcome: .failed)
		try await context.catalog.databaseWriter.write { database in
			try Import(
				id: .mint(),
				folderId: context.rootFolderId,
				startedAt: Date().addingTimeInterval(60),
				finishedAt: Date().addingTimeInterval(120),
				outcome: .completed
			).insert(database)
		}
		let tree = try await context.catalog.databaseWriter.read { try BrowserTree.fetch($0) }
		#expect(tree.unfinishedImports.isEmpty)
	}

	@Test func flagsAreIndependentPerFolder() async throws {
		let context = try await ImportContext.make()
		try await context.catalog.recordImportFinished(id: context.importId, outcome: .completed)
		// A second root on the same volume with its own interrupted import.
		let volumeId = try await context.catalog.reader.read { database in
			try Identifier<Volume>.fetchOne(database, sql: "SELECT id FROM volumes")
		}
		let other = try await context.catalog.findOrCreateRootFolder(
			named: "Other", on: volumeId!, rootPath: "Other"
		)
		try await context.catalog.recordImportStarted(id: .mint(), folderId: other)
		let tree = try await context.catalog.databaseWriter.read { try BrowserTree.fetch($0) }
		#expect(tree.unfinishedImports == [other])
	}
}

/// The master-equivalence pin (one concept, one implementation): the tree's
/// SQL copy must classify every folder exactly as Catalog.unfinishedImport
/// does, across the full outcome vocabulary and multi-generation history.
/// If either predicate is edited alone, this breaks.
struct BrowserTreeUnfinishedMasterEquivalenceTests {

	@Test func setMatchesTheMasterPredicateFolderByFolder() async throws {
		let context = try await ImportContext.make()
		let catalog = context.catalog
		let volumeId = try await catalog.reader.read { database in
			try Identifier<Volume>.fetchOne(database, sql: "SELECT id FROM volumes")
		}!

		// One folder per history shape. ImportContext's own root already
		// carries an interrupted (NULL) bracket.
		func root(_ name: String) async throws -> Identifier<Folder> {
			try await catalog.findOrCreateRootFolder(named: name, on: volumeId, rootPath: name)
		}
		func imported(
			_ folder: Identifier<Folder>, at seconds: TimeInterval, outcome: ImportOutcome?
		) async throws {
			try await catalog.databaseWriter.write { database in
				try Import(
					id: .mint(), folderId: folder,
					startedAt: Date(timeIntervalSinceReferenceDate: seconds),
					finishedAt: outcome == nil ? nil : Date(timeIntervalSinceReferenceDate: seconds + 1),
					outcome: outcome
				).insert(database)
			}
		}

		let canceled = try await root("Canceled")
		try await imported(canceled, at: 100, outcome: .canceled)
		let failed = try await root("Failed")
		try await imported(failed, at: 100, outcome: .failed)
		let completedOnly = try await root("CompletedOnly")
		try await imported(completedOnly, at: 100, outcome: .completed)
		let failedHistory = try await root("FailedHistory")  // failed then completed
		try await imported(failedHistory, at: 100, outcome: .failed)
		try await imported(failedHistory, at: 200, outcome: .completed)
		let regressed = try await root("Regressed")  // completed then interrupted
		try await imported(regressed, at: 100, outcome: .completed)
		try await imported(regressed, at: 200, outcome: nil)
		let untouched = try await root("Untouched")  // no imports at all

		let folders = [
			context.rootFolderId, canceled, failed, completedOnly,
			failedHistory, regressed, untouched,
		]
		var master: Set<Identifier<Folder>> = []
		for folder in folders {
			if try await catalog.unfinishedImport(inFolder: folder) != nil {
				master.insert(folder)
			}
		}

		let tree = try await catalog.databaseWriter.read { try BrowserTree.fetch($0) }
		#expect(tree.unfinishedImports == master)
		// And the master itself behaves as documented, so equivalence isn't
		// two copies of the same mistake.
		#expect(master == [context.rootFolderId, canceled, failed, regressed])
	}
}
