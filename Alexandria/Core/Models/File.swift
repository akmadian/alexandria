//
//  File.swift
//  Alexandria
//
//  Created by ari on 9/9/26.
//

import Foundation
import GRDB

/// Scaffold placeholder; the records round mints the real File against the
/// v0 schema (Core/Catalog/CatalogSchema.swift).
struct File: Identifiable, Codable, FetchableRecord, PersistableRecord {
	static let databaseTableName: String = "files"
	static let databaseColumnDecodingStrategy: DatabaseColumnDecodingStrategy = .convertFromSnakeCase
	static let databaseColumnEncodingStrategy: DatabaseColumnEncodingStrategy = .convertToSnakeCase
	
	var id: Identifier<File>
	var folderId: Identifier<Folder>
	var assetId: Identifier<Asset>
	var importId: Identifier<Import>
	
	var name: String
	var nameKey: String
	var fileStem: String
	var fileExtension: String
	
	var kind: FileKind
	var sizeByes: Int
	var modifiedAt: Date
	var contentHash: String?
	var missing: Bool
	var metadata: [String: String]
	var thumbnailAt: Date?
	var formationRule: String?
}
