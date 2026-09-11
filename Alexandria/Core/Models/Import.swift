//
//  Import.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import GRDB

struct Import: Identifiable, Codable, FetchableRecord, PersistableRecord {
	let id: Identifier<Import>
}
