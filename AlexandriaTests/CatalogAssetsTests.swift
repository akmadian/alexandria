//
//  CatalogAssetsTests.swift
//  AlexandriaTests
//
//  The representative-file read the grid resolves asset thumbnails
//  through: with representative picking unbuilt (formation TODO), the
//  fallback carries every lookup — the asset's first file by id — and
//  file-less assets are absent, which is the caller's placeholder case.
//

import Foundation
import GRDB
import Testing
@testable import Alexandria

struct CatalogAssetsTests {

	@Test func representativeFallsBackToFirstFileByIdAndOmitsFileless() async throws {
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
		var expected: [Identifier<Asset>: Identifier<File>] = [:]
		for file in files {
			guard let assetId = file.assetId else { continue }
			// The fallback's rule verbatim: first file by id — the stored
			// TEXT ordering, matched here on uuidString.
			if let standing = expected[assetId],
				standing.rawValue.uuidString <= file.id.rawValue.uuidString {
				continue
			}
			expected[assetId] = file.id
		}
		#expect(expected.count == 2)

		let assetIds = Array(expected.keys) + [fileless]
		let resolved = try await context.catalog.representativeFileIds(for: assetIds)
		#expect(resolved == expected)
		#expect(resolved[fileless] == nil)

		let empty = try await context.catalog.representativeFileIds(for: [])
		#expect(empty.isEmpty)
	}
}
