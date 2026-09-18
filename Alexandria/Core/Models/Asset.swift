//
//  Asset.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import GRDB

nonisolated struct Asset: Identifiable, CatalogRecord {
	static let databaseTableName = "assets"
	
	let id: Identifier<Asset>
	var kind: FileKind                  // typed on read; the openness lives in the TEXT column (stores the raw value), FileKind.other is the open tail
	var rating: Int?
	var flag: Flag?
	var representativeFileId: Identifier<File>?
	
	enum Flag: String, Codable, Sendable {
		case pick, reject
	}
	
	enum CodingKeys: String, CodingKey {
		case id, kind, rating, flag
		case representativeFileId = "representative_file_id"
	}
	
	enum Columns {
		static let id = Column(CodingKeys.id)
		static let representativeFileId = Column(CodingKeys.representativeFileId)
	}
}

// The flag is a stored column, so it speaks the database's language directly
// (GRDB derives both directions from the String raw value) and not only
// Codable's. Judgment writes bind it; reads decode it.
nonisolated extension Asset.Flag: DatabaseValueConvertible {}

extension Asset {
	/// The representative file's id, as a SQL scalar expression keyed on an
	/// asset-id expression from the surrounding query (`assets.id`,
	/// `m.asset_id`, …): the stored override (formation's pick at mint), else
	/// the asset's first file by id — a stable, arbitrary stand-in that carries
	/// the residual nulls (a rendition minted before its raw joined). The sort
	/// join's form of the election; `Catalog.representativeRecords` is the
	/// batched-read master and this COALESCE shape must preserve its semantics
	/// exactly. `assetID` is always a query-authored column reference, never
	/// external input.
	static func representativeFileID(ofAssetID assetID: String) -> String {
		// The inner tables are aliased (rep_a/rep_f) so they can't shadow an
		// outer `assets`/`files` of the same name — `\(assetID)` must correlate
		// to the surrounding query, not rebind inside these subqueries.
		"""
		COALESCE(
		    (SELECT rep_a.representative_file_id FROM assets rep_a WHERE rep_a.id = \(assetID)),
		    (SELECT rep_f.id FROM files rep_f WHERE rep_f.asset_id = \(assetID) ORDER BY rep_f.id LIMIT 1)
		)
		"""
	}
}
