//
//  Volume.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation
import GRDB

nonisolated enum VolumeKind: String, Codable {
	// Matches the schema CHECK: ('local', 'external', 'network').
	case local = "local"
	case external = "external"
	case network = "network"
}

nonisolated struct Volume: Identifiable, CatalogRecord {
	static let databaseTableName: String = "volumes"
	static let databaseColumnDecodingStrategy: DatabaseColumnDecodingStrategy = .convertFromSnakeCase
	static let databaseColumnEncodingStrategy: DatabaseColumnEncodingStrategy = .convertToSnakeCase
	
	let id: Identifier<Volume>
	let identity: VolumeIdentity?
	let name: String
	let kind: VolumeKind

	enum Columns {
		static let identity = Column(CodingKeys.identity)
	}
}
