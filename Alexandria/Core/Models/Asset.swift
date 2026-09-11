//
//  Asset.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import GRDB

struct Asset: Identifiable, CatalogRecord {
	let id: Identifier<Asset>
}
