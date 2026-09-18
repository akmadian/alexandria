//
//  Catalog+Assets.swift
//  Alexandria
//

import Foundation
import GRDB

extension Catalog {
	/// The asset and its representative file, as records, batched for a
	/// visible window (grid round 2026-09-12; widened from ids to records at
	/// the cell round 2026-09-18 — cells render judgments and file fields
	/// straight off the canonical records, same as the inspector). Election
	/// semantics unchanged: `representative_file_id` when set — formation
	/// picks it at mint (AssetFormation.pickRepresentative) — with the
	/// COALESCE fallback carrying the residual nulls (a rendition minted
	/// before its raw joined): the asset's first file by id, stable,
	/// arbitrary, and honest about being a stand-in. File-less assets carry
	/// a nil file (the caller's placeholder case); their asset record still
	/// rides, so judgments show without pixels.
	func representativeRecords(
		for assetIds: [Identifier<Asset>]
	) async throws -> [Identifier<Asset>: (asset: Asset, file: File?)] {
		guard !assetIds.isEmpty else { return [:] }
		return try await reader.read { database in
			// The election rides the record query as one extra column (the
			// row-with-extra-column shape fileURL's `depth` established);
			// the record decode ignores it.
			let rows = try Row.fetchAll(
				database,
				sql: """
				SELECT assets.*,
				       COALESCE(
				           assets.representative_file_id,
				           (SELECT files.id FROM files
				            WHERE files.asset_id = assets.id
				            ORDER BY files.id LIMIT 1)
				       ) AS elected_file_id
				FROM assets
				WHERE assets.id IN (\(databaseQuestionMarks(count: assetIds.count)))
				""",
				arguments: StatementArguments(assetIds)
			)
			var assets: [Asset] = []
			var fileIdOf: [Identifier<Asset>: Identifier<File>] = [:]
			for row in rows {
				let asset = try Asset(row: row)
				assets.append(asset)
				if let fileId: Identifier<File> = row["elected_file_id"] {
					fileIdOf[asset.id] = fileId
				}
			}
			let files = fileIdOf.isEmpty
				? []
				: try File.fetchAll(
					database,
					sql: "SELECT * FROM files WHERE id IN (\(databaseQuestionMarks(count: fileIdOf.count)))",
					arguments: StatementArguments(Array(fileIdOf.values))
				)
			let fileById = Dictionary(uniqueKeysWithValues: files.map { ($0.id, $0) })
			var result: [Identifier<Asset>: (asset: Asset, file: File?)] = [:]
			for asset in assets {
				result[asset.id] = (asset, fileIdOf[asset.id].flatMap { fileById[$0] })
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
						representativeFileId: cluster.representative
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
	
	// TODO: Make undoable
	func setRepresentativeFile(_ forAssetId: Identifier<Asset>, _ toId: Identifier<File> ) async throws -> Void {
		try await databaseWriter.write { database in
			try Asset
				.filter(Asset.Columns.id == forAssetId)
				.updateAll(database, Asset.Columns.representativeFileId.set(to: toId))
		}
	}
}
