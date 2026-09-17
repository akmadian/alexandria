//
//  WorkingSetQueryFilterTests.swift
//  AlexandriaTests
//
//  The filter's seam into the working set (filter round, 2026-09-16).
//  Two guarantees pinned here:
//  · BYTE IDENTITY — with filter nil, every compiled statement is exactly
//    the pre-round text. The SQL-literal conversion (adversarial pass, B2)
//    re-plumbed the composition; these pins prove it changed nothing the
//    walked queries ratified.
//  · The splices — filtered memberships, the files lens's via-its-asset
//    rule with pending files dropping out, and the manual walk's
//    intersect keeping authored order.
//

import Foundation
import GRDB
import Testing
@testable import Alexandria

struct WorkingSetQueryFilterTests {

	// MARK: - Helpers

	/// The statements a fetch actually executes, raw (`?` unexpanded).
	private func statements(
		of query: WorkingSetQuery, in catalog: Catalog
	) async throws -> [String] {
		try await catalog.reader.read { database in
			var collected: [String] = []
			database.trace(options: .statement) { event in
				if case .statement(let statement) = event {
					collected.append(statement.sql)
				}
			}
			_ = try query.fetchIdentifiers(database)
			database.trace(options: .statement, nil)
			return collected
		}
	}

	private func fetch(_ query: WorkingSetQuery, in catalog: Catalog) async throws -> [SubjectID] {
		try await catalog.reader.read { try query.fetchIdentifiers($0) }
	}

	private func ratingAtLeast(_ n: Int) -> FilterGroup {
		FilterGroup(combine: .and, children: [
			.token(FilterToken(field: .rating, op: .gte, value: .int(n))),
		])
	}

	// MARK: - Byte identity when filter is nil

	@Test func nilFilterCompilesTheExactPreRoundStatements() async throws {
		let catalog = try Catalog(DatabaseQueue(path: ":memory:"))
		let folder = Identifier<Folder>.mint()
		let run = Identifier<Import>.mint()
		let collection = Identifier<Collection>.mint()

		func query(_ lens: Lens, _ source: Source, _ sortKey: Arrangement.SortKey = .added) -> WorkingSetQuery {
			var arrangement = Arrangement()
			arrangement.sortKey = sortKey
			return WorkingSetQuery(lens: lens, source: source, arrangement: arrangement)
		}
		func first(_ q: WorkingSetQuery) async throws -> String {
			try #require(try await statements(of: q, in: catalog).first)
		}

		#expect(try await first(query(.assets, .library))
			== "SELECT asset_id FROM (SELECT id AS asset_id FROM assets) ORDER BY asset_id DESC")

		#expect(try await first(query(.assets, .folder(folder)))
			== "WITH RECURSIVE subtree(id) AS (SELECT id FROM folders WHERE id = ? UNION "
			+ "SELECT folders.id FROM folders JOIN subtree ON folders.parent_id = subtree.id) "
			+ "SELECT asset_id FROM (SELECT DISTINCT asset_id FROM files "
			+ "WHERE folder_id IN subtree AND asset_id IS NOT NULL) ORDER BY asset_id DESC")

		#expect(try await first(query(.assets, .collection(collection)))
			== Collection.subtreeCTE + " "
			+ "SELECT asset_id FROM (SELECT DISTINCT asset_id FROM collection_members "
			+ "WHERE collection_id IN subtree) ORDER BY asset_id DESC")

		// The captured shape, composed from the same election constant the
		// query composes from — the one implementation, compared exactly.
		#expect(try await first(query(.assets, .library, .captured))
			== "SELECT m.asset_id FROM (SELECT id AS asset_id FROM assets) AS m "
			+ "LEFT JOIN files rep ON rep.id = \(Asset.representativeFileID(ofAssetID: "m.asset_id")) "
			+ "ORDER BY rep.capture_sort IS NULL, rep.capture_sort DESC, m.asset_id DESC")

		#expect(try await first(query(.files, .library))
			== "SELECT id FROM files ORDER BY id DESC")

		#expect(try await first(query(.files, .import(run)))
			== "SELECT id FROM files WHERE import_id = ? ORDER BY id DESC")
	}

	// MARK: - The splices

	/// Three formed assets (a/b/c) and one pending file, keyed by name.
	private struct Fixture {
		let context: ImportContext
		var catalog: Catalog { context.catalog }
		let asset: [String: Identifier<Asset>]
	}

	/// a.jpg b.jpg c.jpg formed into assets; d.jpg recorded but NOT formed
	/// (asset_id NULL — the formation-pending state).
	private func fixture() async throws -> Fixture {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/a.jpg"),
			context.prepared("/Volumes/Test/Shoot/b.jpg"),
			context.prepared("/Volumes/Test/Shoot/c.jpg"),
		])
		try await context.form()
		try await context.record([context.prepared("/Volumes/Test/Shoot/d.jpg")])
		let files = try await context.catalog.files(inImport: context.importId)
		var byName: [String: Identifier<Asset>] = [:]
		for file in files {
			if let assetId = file.assetId { byName[String(file.name.prefix(1))] = assetId }
		}
		return Fixture(context: context, asset: byName)
	}

	@Test func filteredLibraryAssetsNarrowToMatches() async throws {
		let fx = try await fixture()
		let a = try #require(fx.asset["a"]), c = try #require(fx.asset["c"])
		_ = try await fx.catalog.setRating([a], to: 5)
		_ = try await fx.catalog.setRating([c], to: 3)

		let query = WorkingSetQuery(
			lens: .assets, source: .library, arrangement: Arrangement(),
			filter: ratingAtLeast(4)
		)
		#expect(try await fetch(query, in: fx.catalog) == [.asset(a)])
	}

	@Test func filteredFolderSourceIntersectsScopeAndFilter() async throws {
		// scope ⊂ filter: the source narrows, the filter narrows further.
		let fx = try await fixture()
		let b = try #require(fx.asset["b"])
		_ = try await fx.catalog.setRating([b], to: 4)

		let query = WorkingSetQuery(
			lens: .assets, source: .folder(fx.context.rootFolderId),
			arrangement: Arrangement(), filter: ratingAtLeast(4)
		)
		#expect(try await fetch(query, in: fx.catalog) == [.asset(b)])
	}

	@Test func filesLensMatchesThroughTheAssetAndDropsPendingFiles() async throws {
		// Ruled: a file matches if its ASSET matches. d.jpg has no asset
		// yet (formation pending), so it fails ANY active filter —
		// intentional, and pinned here so it can never become an accident.
		let fx = try await fixture()

		let unfiltered = WorkingSetQuery(
			lens: .files, source: .library, arrangement: Arrangement()
		)
		#expect(try await fetch(unfiltered, in: fx.catalog).count == 4)

		// flag isUnset matches every FORMED asset — the filter that keeps
		// all three and still drops only the pending file.
		let allUnflagged = FilterGroup(combine: .and, children: [
			.token(FilterToken(field: .flag, op: .isUnset, value: nil)),
		])
		var filtered = unfiltered
		filtered.filter = allUnflagged
		#expect(try await fetch(filtered, in: fx.catalog).count == 3)
	}

	@Test func filteredFilesLensOverAFolderUsesTheConcatenatedSplice() async throws {
		// Round review, finding 3: .folder rides the andAssetMatch
		// concatenation branch (unlike .library's standalone WHERE) — the
		// site where a missing space or a folder-vs-filter argument
		// transposition would live. A correct narrowed answer pins both.
		let fx = try await fixture()
		let a = try #require(fx.asset["a"])
		_ = try await fx.catalog.setRating([a], to: 5)
		let files = try await fx.catalog.files(inImport: fx.context.importId)
		let aFile = try #require(files.first { $0.name == "a.jpg" }).id

		let query = WorkingSetQuery(
			lens: .files, source: .folder(fx.context.rootFolderId),
			arrangement: Arrangement(), filter: ratingAtLeast(4)
		)
		#expect(try await fetch(query, in: fx.catalog) == [.file(aFile)])
	}

	@Test func filteredCapturedSortOverAFolderComposesTheDeepestStatement() async throws {
		// Round review, finding 4: subtree CTE + filtered() + the
		// representative-election subquery is the deepest composition —
		// the statement where nesting and binding order are hardest to
		// eyeball. A correct answer through it pins the whole stack.
		let fx = try await fixture()
		let c = try #require(fx.asset["c"])
		_ = try await fx.catalog.setRating([c], to: 4)

		var arrangement = Arrangement()
		arrangement.sortKey = .captured
		let query = WorkingSetQuery(
			lens: .assets, source: .folder(fx.context.rootFolderId),
			arrangement: arrangement, filter: ratingAtLeast(4)
		)
		#expect(try await fetch(query, in: fx.catalog) == [.asset(c)])
	}

	@Test func manualCollectionOrderIntersectsWithTheFilter() async throws {
		// Ruled (adversarial pass, B1): the manual walk INTERSECTS — the
		// clause runs once, non-members drop, authored order survives for
		// what remains. Without this, a filtered manual view silently
		// showed everything.
		let fx = try await fixture()
		let a = try #require(fx.asset["a"]), b = try #require(fx.asset["b"]), c = try #require(fx.asset["c"])
		let collection = try await fx.catalog.createCollection(named: "Walk")
		// Authored order deliberately NOT sorted order.
		_ = try await fx.catalog.addMembers([c, a, b], to: collection.id)
		_ = try await fx.catalog.setRating([a, c], to: 5)

		var arrangement = Arrangement()
		arrangement.sortKey = .manual
		let query = WorkingSetQuery(
			lens: .assets, source: .collection(collection.id),
			arrangement: arrangement, filter: ratingAtLeast(4)
		)
		// Survivors keep the authored sequence: c before a, b gone.
		#expect(try await fetch(query, in: fx.catalog) == [.asset(c), .asset(a)])
	}
}
