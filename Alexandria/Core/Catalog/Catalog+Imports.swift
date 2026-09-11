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

/// The ratified timestamp form: ISO 8601 UTC, millisecond precision, 'Z'
/// suffix — lexicographic order is chronological order.
nonisolated func catalogTimestamp(_ date: Date = .now) -> String {
	let formatter = ISO8601DateFormatter()
	formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
	return formatter.string(from: date)
}
