//
//  Catalog.swift
//  Alexandria
//

import Foundation
import GRDB

/// The catalog: the database and everything in it (_design/requirements.md,
/// Nouns). Wraps the one database connection per open catalog and migrates
/// on init. `nonisolated`: catalog work runs off the main actor by design.
nonisolated struct Catalog: Sendable {
	/// The per-catalog directory on disk; nil for in-memory catalogs (tests).
	let directory: URL?

	/// Writes go through named catalog methods (invariant-preserving
	/// transactions), minted per feature round; `internal` only because the
	/// schema-fence tests write raw SQL on purpose.
	let databaseWriter: any DatabaseWriter

	/// Read-only access for queries and observation.
	var reader: any DatabaseReader { databaseWriter }

	init(_ databaseWriter: any DatabaseWriter, directory: URL? = nil) throws {
		self.directory = directory
		self.databaseWriter = databaseWriter
		try Self.migrator.migrate(databaseWriter)
	}

	/// Opens the catalog living in `directory`, creating it on first use.
	/// Layout (ratified 2026-09-10): `<name>.alxcat` package holding the SQLite
	/// database and its WAL, beside a visible `Thumbnails/` store. `Logs/`
	/// joins when the logging round lands a sink.
	static func open(at directory: URL) throws -> Catalog {
		print("CATALOG: Opening catalog at \(directory)")
		let packageURL = directory.appending(path: "\(directory.lastPathComponent).alxcat")
		print("CATALOG: Package URL - \(packageURL)")
		try FileManager.default.createDirectory(at: packageURL, withIntermediateDirectories: true)
		try FileManager.default.createDirectory(at: directory.appending(path: "Thumbnails"), withIntermediateDirectories: true)
		let databasePool = try DatabasePool(path: packageURL.appending(path: "catalog.sqlite").path)
		return try Catalog(databasePool, directory: directory)
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
