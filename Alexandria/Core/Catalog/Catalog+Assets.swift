//
//  Catalog+Assets.swift
//  Alexandria
//

import Foundation
import GRDB

extension Catalog {
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
