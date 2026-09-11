//
//  Folder.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import GRDB

struct Folder: Identifiable, Codable, FetchableRecord, PersistableRecord {
	static let databaseTableName: String = "folders"
	static let databaseColumnDecodingStrategy: DatabaseColumnDecodingStrategy = .convertFromSnakeCase
	static let databaseColumnEncodingStrategy: DatabaseColumnEncodingStrategy = .convertToSnakeCase
	
	let id: Identifier<Folder>
	let volumeId: Identifier<Volume>
	let parentId: Identifier<Folder>
	let name: String
	let nameKey: String
	let rootPath: String?
}
