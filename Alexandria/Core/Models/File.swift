//
//  File.swift
//  Alexandria
//
//  Created by ari on 9/9/26.
//

import Foundation
import GRDB

/// A `files` row: one cataloged on-disk file — identity, observation, and
/// derivations (v0 schema, Core/Catalog/CatalogSchema.swift).
nonisolated struct File: Identifiable, CatalogRecord {
	static let databaseTableName = "files"
	
	var id: Identifier<File>
	var folderId: Identifier<Folder>
	var assetId: Identifier<Asset>?     // NULL = formation pending
	var importId: Identifier<Import>
	var name: String
	var nameKey: String
	var fileStem: String
	var fileExtension: String
	var kind: FileKind
	var sizeBytes: Int
	var modifiedAt: Date
	var contentHash: String?
	var missing: Bool
	var metadata: String?               // FileMetadata.databaseJSON(); NULL = none
	var thumbnailAt: Date?
	var formationRule: String?
	
	enum CodingKeys: String, CodingKey {
		case id
		case folderId = "folder_id"
		case assetId = "asset_id"
		case importId = "import_id"
		case name
		case nameKey = "name_key"
		case fileStem = "file_stem"
		case fileExtension = "file_extension"
		case kind
		case sizeBytes = "size_bytes"
		case modifiedAt = "modified_at"
		case contentHash = "content_hash"
		case missing
		case metadata
		case thumbnailAt = "thumbnail_at"
		case formationRule = "formation_rule"
	}
	
	enum Columns {
		static let id = Column(CodingKeys.id)
		static let folderId = Column(CodingKeys.folderId)
		static let assetId = Column(CodingKeys.assetId)
		static let importId = Column(CodingKeys.importId)
		static let nameKey = Column(CodingKeys.nameKey)
		static let fileStem = Column(CodingKeys.fileStem)
		static let formationRule = Column(CodingKeys.formationRule)
	}
}
