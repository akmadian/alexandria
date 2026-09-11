//
//  Folder.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import GRDB

nonisolated struct Folder: Identifiable, CatalogRecord {
	static let databaseTableName = "folders"

	let id: Identifier<Folder>
	let volumeId: Identifier<Volume>
	let parentId: Identifier<Folder>?  // NULL = a tracked root
	let name: String
	let nameKey: String
	let rootPath: String?              // roots only (schema shape CHECK)

	// Explicit keys, not the snake_case strategy: Columns are built from
	// these, and a strategy-renamed column would make filters silently
	// match nothing.
	enum CodingKeys: String, CodingKey {
		case id
		case volumeId = "volume_id"
		case parentId = "parent_id"
		case name
		case nameKey = "name_key"
		case rootPath = "root_path"
	}

	enum Columns {
		static let volumeId = Column(CodingKeys.volumeId)
		static let parentId = Column(CodingKeys.parentId)
		static let nameKey = Column(CodingKeys.nameKey)
		static let rootPath = Column(CodingKeys.rootPath)
	}
}
