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
	func recordImportStarted(id: Identifier<Import>, folderId: Identifier<Folder>) async throws {
		try await databaseWriter.write { database in
			let importRecord = Import(
				id: id,
				folderId: folderId,
				startedAt: .now,
				finishedAt: nil,
				outcome: nil
			)
			try importRecord.insert(database)
		}
	}
	
	func recordImportFinished(id: Identifier<Import>, outcome: ImportOutcome) async throws {
		try await databaseWriter.write { database in
			_ = try Import
				.filter(Import.Columns.id == id)
				.updateAll(
					database,
					Import.Columns.finishedAt.set(to: catalogTimestamp()),
					Import.Columns.outcome.set(to: outcome.rawValue)
				)
		}
	}

	/// The pre-identity DLQ (import_errors): paths the walk saw but couldn't
	/// read, so they never became file rows. Residue over silence.
	func recordImportErrors(
		_ failures: [(path: String, reasonCode: String, message: String)],
		importId: Identifier<Import>
	) async throws {
		guard !failures.isEmpty else { return }
		try await databaseWriter.write { database in
			for failure in failures {
				try database.execute(
					sql: """
					INSERT INTO import_errors (id, import_id, path, reason_code, message)
					VALUES (?, ?, ?, ?, ?)
					""",
					arguments: [
						UUID.v7().uuidString, importId,
						failure.path, failure.reasonCode, failure.message,
					]
				)
			}
		}
	}
}
