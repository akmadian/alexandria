//
//  Catalog+Assets.swift
//  Alexandria
//

import Foundation
import GRDB

extension Catalog {
	/// The file whose thumbnail stands for each asset, batched for a visible
	/// window (grid round, 2026-09-12). `representative_file_id` when set;
	/// until representative picking lands (formation TODO), it never is, so
	/// the fallback carries the read: the asset's first file by id — stable,
	/// arbitrary, and honest about being a stand-in. File-less assets are
	/// absent from the result (the caller's placeholder case).
	func representativeFileIds(
		for assetIds: [Identifier<Asset>]
	) async throws -> [Identifier<Asset>: Identifier<File>] {
		guard !assetIds.isEmpty else { return [:] }
		return try await reader.read { database in
			let rows = try Row.fetchAll(
				database,
				sql: """
				SELECT assets.id AS asset_id,
				       COALESCE(
				           assets.representative_file_id,
				           (SELECT files.id FROM files
				            WHERE files.asset_id = assets.id
				            ORDER BY files.id LIMIT 1)
				       ) AS file_id
				FROM assets
				WHERE assets.id IN (\(databaseQuestionMarks(count: assetIds.count)))
				""",
				arguments: StatementArguments(assetIds)
			)
			var result: [Identifier<Asset>: Identifier<File>] = [:]
			for row in rows {
				guard let fileId: Identifier<File> = row["file_id"] else { continue }
				result[row["asset_id"]] = fileId
			}
			return result
		}
	}

	/// The formation round's write verb: applies resolved clusters in one
	/// transaction — mint or join per cluster, members bound with their
	/// admitting rule's provenance, absorbed scaffolding assets merged into
	/// the destination (batch-invariance ruling: files repointed with their
	/// provenance untouched, the emptied asset rows deleted). Decisions are
	/// AssetFormation's; this method only makes them durable.
	///
	/// Admissions are guarded on `asset_id IS NULL`, so a row that stopped
	/// being unformed between the pass's read and this write (a future
	/// manual attachment, a vanished file) is skipped, never clobbered —
	/// the returned count says how many were.
	@discardableResult
	func recordFormedAssets(_ clusters: [AssetFormation.Cluster]) async throws -> Int {
		guard !clusters.isEmpty else { return 0 }
		return try await databaseWriter.write { database in
			var skippedMembers = 0
			for cluster in clusters {
				let assetId: Identifier<Asset>
				switch cluster.destination {
				case .join(let existing):
					assetId = existing
				case .mint(let kind):
					let asset = Asset(
						id: .mint(),
						kind: kind,
						rating: nil,
						flag: nil,
						representativeFileId: nil // TODO: Representative picking
					)
					try asset.insert(database)
					assetId = asset.id
				}
				for absorbed in cluster.absorbed {
					_ = try File
						.filter(File.Columns.assetId == absorbed)
						.updateAll(database, File.Columns.assetId.set(to: assetId))
					try Asset.deleteOne(database, key: absorbed)
				}
				for member in cluster.members {
					let updated = try File
						.filter(File.Columns.id == member.file)
						.filter(File.Columns.assetId == nil)
						.updateAll(
							database,
							File.Columns.assetId.set(to: assetId),
							File.Columns.formationRule.set(to: member.rule)
						)
					skippedMembers += 1 - updated
				}
			}
			return skippedMembers
		}
	}
}
