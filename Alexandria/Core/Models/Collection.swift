//
//  Collection.swift
//  Alexandria
//
//  Collections round, 2026-09-12.
//

import GRDB

/// A `collections` row: an authored, named set of assets that may also
/// contain collections — ONE noun, no separate set/group type. Children
/// hang off `parentId`; member assets live in `collection_members`; the
/// two relationships never mix. Names are free-form and may duplicate
/// (identity is the id; the create/rename verbs reject only emptiness).
///
/// A non-NULL `predicate` makes it a smart collection (smart-collection
/// round, 2026-09-18): membership computed from the stored filter, never
/// from `collection_members` — the membership verbs refuse it. Children
/// stay legal either way; the tree never consults the predicate.
///
/// Note: this shadows `Swift.Collection` for unqualified lookup inside
/// the module — stdlib-generic code here spells it `Swift.Collection`.
/// Accepted deliberately: the record carries the ratified noun's name.
nonisolated struct Collection: Identifiable, CatalogRecord {
	static let databaseTableName = "collections"

	let id: Identifier<Collection>
	var parentId: Identifier<Collection>?   // NULL = a root
	var name: String
	/// The smart collection's stored filter: the versioned envelope
	/// (`FilterGroup.serialized()`), NULL for a manual collection. Kept as
	/// the raw string here — decoding is lazy, per use, at each consumer
	/// (`FilterGroup(serialized:)` is the one fence), so a corrupt value
	/// disables one collection and never a fetch that lists rows.
	var predicate: String? = nil

	var isSmart: Bool { predicate != nil }

	enum CodingKeys: String, CodingKey {
		case id
		case parentId = "parent_id"
		case name
		case predicate
	}

	enum Columns {
		static let id = Column(CodingKeys.id)
		static let parentId = Column(CodingKeys.parentId)
		static let name = Column(CodingKeys.name)
		static let predicate = Column(CodingKeys.predicate)
	}
}
