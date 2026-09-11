//
//  Identifier.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//
//	This is a catalog identifer - what a Subject is WITHIN the catalog.
//

import Foundation

struct Identifier<Subject>: Hashable, Sendable {
	let rawValue: UUID
	
	init(rawValue: UUID) {
		self.rawValue = rawValue
	}
	
	static func mint() -> Self {
		Self(rawValue: .v7())
	}
}

extension Identifier: Codable {
	init(from decoder: any Decoder) throws {
		rawValue = try decoder.singleValueContainer().decode(UUID.self)
	}
	
	func encode(to encoder: any Encoder) throws {
		var container = encoder.singleValueContainer()
		try container.encode(rawValue)
	}
}
