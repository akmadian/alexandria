//
//  FilesAssociatedWithAssetRequest.swift
//  Alexandria
//
//  Created by ari on 9/16/26.
//

import GRDB
import GRDBQuery

struct AssociatedFilesRequest: ValueObservationQueryable {
	static var defaultValue: [File]? { nil }
	
	var assetId: Identifier<Asset>
	
	func fetch(_ db: Database) throws -> [File]? {
		try File
			.filter(Column("asset_id") == assetId)
			.fetchAll(db)
	}
}
