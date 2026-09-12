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
}
