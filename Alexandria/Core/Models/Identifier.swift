//
//  Identifier.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//
//	This is a catalog identifer - what a Subject is WITHIN the catalog.
//

import Foundation
import GRDB

nonisolated struct Identifier<Subject>: Hashable, Sendable {
	let rawValue: UUID
	
	init(rawValue: UUID) {
		self.rawValue = rawValue
	}
	
	static func mint() -> Self {
		Self(rawValue: .v7())
	}
}

nonisolated extension Identifier: Codable {
	// Encodes as uuidString, never as UUID: GRDB's primitive form for a bare
	// UUID is a 16-byte BLOB, and the schema ratifies TEXT ids. The string
	// leaf keeps the column readable in any sqlite3 shell.
	init(from decoder: any Decoder) throws {
		let container = try decoder.singleValueContainer()
		let text = try container.decode(String.self)
		guard let uuid = UUID(uuidString: text) else {
			throw DecodingError.dataCorruptedError(
				in: container, debugDescription: "not a UUID: \(text)"
			)
		}
		rawValue = uuid
	}

	func encode(to encoder: any Encoder) throws {
		var container = encoder.singleValueContainer()
		try container.encode(rawValue.uuidString)
	}
}

// Same TEXT leaf for query arguments and filters.
nonisolated extension Identifier: DatabaseValueConvertible {
	var databaseValue: DatabaseValue { rawValue.uuidString.databaseValue }

	static func fromDatabaseValue(_ dbValue: DatabaseValue) -> Self? {
		String.fromDatabaseValue(dbValue)
			.flatMap(UUID.init(uuidString:))
			.map(Self.init(rawValue:))
	}
}
