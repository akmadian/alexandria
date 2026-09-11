//
//  Import.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation
import GRDB

nonisolated enum ImportOutcome: String, Codable, Sendable {
	case completed
	case canceled
	case failed
}

nonisolated struct Import: Identifiable, Decodable, CatalogRecord {
	static let databaseTableName = "imports"
	
	let id: Identifier<Import>
	let folderId: Identifier<Folder>
	let startedAt: Date
	let finishedAt: Date?
	let outcome: ImportOutcome?
	
	
	// Explicit keys, not the snake_case strategy: Columns are built from
	// these, and a strategy-renamed column would make filters silently
	// match nothing.
	enum CodingKeys: String, CodingKey {
		case id
		case folderId = "folder_id"
		case startedAt = "started_at"
		case finishedAt = "finished_at"
		case outcome = "outcome"
	}
	
	enum Columns {
		static let id = Column(CodingKeys.id)
		static let folderId = Column(CodingKeys.folderId)
		static let startedAt = Column(CodingKeys.startedAt)
		static let finishedAt = Column(CodingKeys.finishedAt)
		static let outcome = Column(CodingKeys.outcome)
	}
}
