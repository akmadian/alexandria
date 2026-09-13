//
//  CollectionMember.swift
//  Alexandria
//
//  Collections round, 2026-09-12.
//

import GRDB

/// A `collection_members` row: one asset's MANUAL membership in one
/// collection — future smart collections compute membership from a
/// predicate and never write here. Keyed by (collection, asset), so the
/// same asset joins a collection once; not Identifiable by design (the
/// composite key IS the identity, and no surface needs a single-value id).
///
/// `orderKey` is the manual order: a fractional index minted append-at-end
/// on add (added-order until the first drag), always present. Key math
/// lives in `OrderKey` (build chunk 2); this record only carries the value.
nonisolated struct CollectionMember: CatalogRecord {
	static let databaseTableName = "collection_members"

	let collectionId: Identifier<Collection>
	let assetId: Identifier<Asset>
	var orderKey: String

	enum CodingKeys: String, CodingKey {
		case collectionId = "collection_id"
		case assetId = "asset_id"
		case orderKey = "order_key"
	}

	enum Columns {
		static let collectionId = Column(CodingKeys.collectionId)
		static let assetId = Column(CodingKeys.assetId)
		static let orderKey = Column(CodingKeys.orderKey)
	}
}
