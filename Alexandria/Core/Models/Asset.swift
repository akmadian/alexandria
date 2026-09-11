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
	var kind: String                    // open set by design; v0 writes FileKind raw values
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
}
