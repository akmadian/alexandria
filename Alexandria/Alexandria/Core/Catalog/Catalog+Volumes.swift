//
//  Catalog+Volumes.swift
//  Alexandria
//

import Foundation
import GRDB

extension Catalog {
	/// Idempotent recording of an observed volume, keyed by identity
	/// (idx_volumes_identity). A nil identity finds nothing and always
	/// creates: NULL-distinct by ratified design — re-identification of
	/// unidentified volumes is a future user repair, never a guess.
	func findOrCreateVolume(_ observed: ObservedVolume) async throws -> Identifier<Volume> {
		try await databaseWriter.write { database in
			if let identity = observed.identity,
			   let existing = try Volume
					.filter(Volume.Columns.identity == identity)
					.fetchOne(database)
			{
				return existing.id
			}
			let volume = Volume(
				id: .mint(),
				identity: observed.identity,
				name: observed.name,
				kind: observed.kind
			)
			try volume.insert(database)
			return volume.id
		}
	}
}
