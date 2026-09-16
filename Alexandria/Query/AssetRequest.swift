//
//  AssetRequest.swift
//  Alexandria
//
//  Created by ari on 9/16/26.
//

import GRDB
import GRDBQuery

/// Observes one asset by id. Feeds from the `\.databaseContext` environment
/// (injected at the app root), so the view re-renders whenever that asset's
/// record changes.
struct AssetRequest: ValueObservationQueryable {
	static var defaultValue: Asset? { nil }
	
	var id: Identifier<Asset>
	
	func fetch(_ db: Database) throws -> Asset? {
		try Asset.fetchOne(db, key: id)
	}
}
