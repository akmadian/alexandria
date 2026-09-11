//
//  Import.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation
import GRDB

struct Import: Identifiable, CatalogRecord {
	static let databaseTableName = "imports"
	static let databaseDateDecodingStrategy = DatabaseDateDecodingStrategy.iso8601
	static let databaseDateEncodingStrategy = DatabaseDateEncodingStrategy.iso8601
	
	let id: Identifier<Import>
	let folderId: Identifier<Folder>
	let startedAt: Date
	let finishedAt: Date
	let outcome: String
}
