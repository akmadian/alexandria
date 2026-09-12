//
//  Catalog+Imports.swift
//  Alexandria
//

import Foundation
import GRDB

extension Catalog {
	/// The unfinished job for a source, or nil (resume ruling, 2026-09-11):
	/// the folder's latest import if its outcome isn't 'completed' —
	/// interrupted (NULL), canceled, and failed all mean the task didn't
	/// finish, and restarting the same source picks it back up. Pure find:
	/// the caller mints on nil.
	func unfinishedImport(inFolder folderId: Identifier<Folder>) async throws -> Identifier<Import>? {
		try await reader.read { database in
			// The id tie-break makes same-millisecond starts deterministic
			// (UUIDv7 ids are time-ordered, so it is also chronological).
			let latest = try Import
				.filter(Import.Columns.folderId == folderId)
				.order(Import.Columns.startedAt.desc, Import.Columns.id.desc)
				.fetchOne(database)
			guard let latest, latest.outcome != .completed else { return nil }
			return latest.id
		}
	}

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
	
	/// Reopens a picked-back-up bracket (resume ruling, 2026-09-11): a resumed
	/// canceled/failed row must read as running again — finished_at and
	/// outcome return to NULL until this run stamps its own ending.
	func recordImportResumed(id: Identifier<Import>) async throws {
		try await databaseWriter.write { database in
			_ = try Import
				.filter(Import.Columns.id == id)
				.updateAll(
					database,
					Import.Columns.finishedAt.set(to: nil as String?),
					Import.Columns.outcome.set(to: nil as String?)
				)
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
