//
//  Catalog.swift
//  Alexandria
//

import Foundation
import GRDB

/// The catalog: the database and everything in it (_design/requirements.md,
/// Nouns). Wraps the one database connection per open catalog and migrates
/// on init. `nonisolated`: catalog work runs off the main actor by design.
nonisolated struct Catalog {
	let databaseWriter: any DatabaseWriter

	init(_ databaseWriter: any DatabaseWriter) throws {
		self.databaseWriter = databaseWriter
		try Self.migrator.migrate(databaseWriter)
	}

	static var migrator: DatabaseMigrator {
		var migrator = DatabaseMigrator()
		#if DEBUG
		// Pre-1.0 the v0 migration is edited in place; a schema change wipes
		// and rebuilds dev catalogs instead of limping on a stale one.
		migrator.eraseDatabaseOnSchemaChange = true
		#endif
		migrator.registerMigration("v0") { database in
			try database.execute(sql: CatalogSchema.v0)
		}
		return migrator
	}
}
