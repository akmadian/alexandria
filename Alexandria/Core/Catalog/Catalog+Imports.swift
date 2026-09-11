//
//  Catalog+Imports.swift
//  Alexandria
//

import Foundation
import GRDB

extension Catalog {
	/// Opens the run's bracket: the imports row, started now, finished NULL —
	/// which stays NULL only if the run never completes (the interrupted
	/// signal).
	func recordImportStarted(id: Identifier<Import>, folderId: Identifier<Folder>?) async throws {
		try await databaseWriter.write { database in
			try database.execute(
				sql: "INSERT INTO imports (id, folder_id, started_at) VALUES (?, ?, ?)",
				arguments: [id, folderId, catalogTimestamp()]
			)
		}
	}
}
