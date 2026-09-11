//
//  Volume.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation
import GRDB

enum VolumeKind: String, Codable {
	// Matches the schema CHECK: ('local', 'external', 'network').
	case local = "local"
	case external = "external"
	case network = "network"
}

struct Volume: Identifiable, Codable, FetchableRecord, PersistableRecord {
	static let databaseTableName: String = "volumes"
	static let databaseColumnDecodingStrategy: DatabaseColumnDecodingStrategy = .convertFromSnakeCase
	static let databaseColumnEncodingStrategy: DatabaseColumnEncodingStrategy = .convertToSnakeCase
	
	let id: Identifier<Volume>
	let identity: VolumeIdentity?
	let name: String
	let kind: VolumeKind
}
