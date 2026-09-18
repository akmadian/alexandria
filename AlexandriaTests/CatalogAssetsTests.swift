//
//  CatalogAssetsTests.swift
//  AlexandriaTests
//
//  The representative-records read the grid resolves cells through: the
//  stored pick (formation's, at mint — a raw leads its rendition) elects
//  first, the COALESCE fallback (first file by id) carries a null pick,
//  and a file-less asset rides with a nil file (the caller's placeholder
//  case) while its asset record still shows.
//
//  This test went stale once on exactly this seam: written against
//  fallback-only election, silently wrong from the commit that stored
//  formation's pick. It now pins both tiers.
//

import Foundation
import GRDB
import Testing
@testable import Alexandria

struct CatalogAssetsTests {

	@Test func electionPrefersTheStoredPickAndFallsBackToFirstFileById() async throws {
		let context = try await ImportContext.make()
		// a.jpg + a.raf form one asset with two files; b.jpg its own.
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/a.jpg"),
			context.prepared("/Volumes/Test/Shoot/a.raf"),
			context.prepared("/Volumes/Test/Shoot/b.jpg"),
		])
		try await context.form()
		let fileless = try await seedAsset(context.catalog, at: 1_000)

		let files = try await context.catalog.files(inImport: context.importId)
		func file(_ name: String) throws -> File {
			try #require(files.first { $0.name == name })
		}
		let raw = try file("a.raf")
		let rendition = try file("a.jpg")
		let single = try file("b.jpg")
		let pairAsset = try #require(raw.assetId)
		let singleAsset = try #require(single.assetId)

		// Tier 1, the stored pick: formation elected the raw at mint (a raw
		// leads its rendition), and whole records ride — the asset's own row
		// and the elected file's.
		let assetIds = [pairAsset, singleAsset, fileless]
		let resolved = try await context.catalog.representativeRecords(for: assetIds)
		#expect(resolved.count == 3)
		let pair = try #require(resolved[pairAsset])
		#expect(pair.asset.id == pairAsset)
		#expect(pair.file?.id == raw.id)
		#expect(resolved[singleAsset]?.file?.id == single.id)

		// File-less: the asset record shows, the file is honestly absent.
		let filelessRecords = try #require(resolved[fileless])
		#expect(filelessRecords.asset.id == fileless)
		#expect(filelessRecords.file == nil)

		// Tier 2, the fallback: strip the pair's stored pick (the residual-
		// null case — a rendition minted before its raw joined) and the
		// election lands on the first file by id. Which member that is stays
		// computed, not assumed: ids minted in the same millisecond order
		// arbitrarily (UUIDv7 random tail).
		let firstById = try #require(
			[rendition, raw].min { $0.id.rawValue.uuidString < $1.id.rawValue.uuidString }
		)
		try await context.catalog.databaseWriter.write { database in
			try database.execute(
				sql: "UPDATE assets SET representative_file_id = NULL WHERE id = ?",
				arguments: [pairAsset]
			)
		}
		let refetched = try await context.catalog.representativeRecords(for: [pairAsset])
		#expect(refetched[pairAsset]?.file?.id == firstById.id)

		let empty = try await context.catalog.representativeRecords(for: [])
		#expect(empty.isEmpty)
	}

	@Test func filesByIdFetchesExactlyTheAsked() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/a.jpg"),
			context.prepared("/Volumes/Test/Shoot/b.jpg"),
		])
		let files = try await context.catalog.files(inImport: context.importId)
		#expect(files.count == 2)
		// One asked, one not — and the result is a set: the query promises
		// no order.
		let asked = [files[0].id]
		let fetched = try await context.catalog.files(ids: asked)
		#expect(Set(fetched.map(\.id)) == Set(asked))
		#expect(fetched.first?.name == files[0].name)
		let none = try await context.catalog.files(ids: [])
		#expect(none.isEmpty)
	}
}
