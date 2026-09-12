//
//  WorkingSetQuery.swift
//  Alexandria
//
//  The question's vocabulary and its compilation (UI-layer design round,
//  2026-09-11). A query is lens + source + arrangement — FROM, WHERE, and
//  ORDER BY — compiled to one SQL statement over the catalog. The filter's
//  clause joins the WHERE when the filter round mints its type.
//

import Foundation
import GRDB

/// The unit the working set ranges over — the FROM clause. A source or
/// filter clause narrows membership; the lens decides what a member IS.
nonisolated enum Lens: Sendable {
	case assets
	case files
}

/// The sidebar's clause: where the user is looking. A kind of filter by
/// ruling (scope ⊂ filter, 2026-09-11) — it compiles to a WHERE like any
/// other — but held as its own posture field so the browser authors it
/// without read-modify-writing the future filter's tokens, and so it stays
/// separately clearable.
nonisolated enum Source: Hashable, Sendable {
	case library
	/// Direct membership only for now; whether a folder source reaches its
	/// subtree is deliberately unsettled, for the browser round.
	case folder(Identifier<Folder>)
	case `import`(Identifier<Import>)
}

/// The ORDER BY: sort key + direction. Orders the working set, never
/// changes membership. One key today — the sort vocabulary grows in its
/// own round; grouping (sections) is deliberately unsettled.
nonisolated struct Arrangement: Hashable, Sendable {
	enum Key: Sendable {
		/// Catalog-entry order at millisecond granularity: ids are UUIDv7
		/// (ms timestamp + random tail), so records minted in the same
		/// millisecond order arbitrarily — but stably, since ids persist.
		case added
	}

	enum Direction: Sendable {
		case ascending
		case descending
	}

	var key: Key = .added
	/// Dev default: newest first.
	var direction: Direction = .descending

	var reversed: Arrangement {
		var copy = self
		copy.direction = direction == .ascending ? .descending : .ascending
		return copy
	}
}

/// A working-set member's identity, self-describing about its lens — a
/// selection held across a lens flip can never masquerade as the other
/// unit; reconciliation drops it because it matches nothing.
nonisolated enum SubjectID: Hashable, Sendable {
	case asset(Identifier<Asset>)
	case file(Identifier<File>)
}

/// The compiled question. A pure value: testable without the store, and a
/// fresh one is minted per observation swap, so an observation's query
/// never mutates under it.
nonisolated struct WorkingSetQuery: Hashable, Sendable {
	var lens: Lens
	var source: Source
	var arrangement: Arrangement

	/// The working set: every id the question yields, in arrangement order.
	/// Ids only — record content is fetched by consumers on demand, so this
	/// stays cheap to re-run on every impactful commit.
	func fetchIdentifiers(_ database: Database) throws -> [SubjectID] {
		let direction = arrangement.direction == .ascending ? "ASC" : "DESC"
		// Exhaustive so a new key can't silently keep sorting by id.
		let column: String
		switch arrangement.key {
		case .added:
			// TEXT UUIDv7: lexicographic order is chronological, at ms
			// granularity (see Arrangement.Key.added).
			column = "id"
		}
		let order = "ORDER BY \(column) \(direction)"

		switch lens {
		case .assets:
			let sql: String
			var arguments: StatementArguments = []
			switch source {
			case .library:
				sql = "SELECT id FROM assets \(order)"
			case .folder(let folder):
				sql = """
					SELECT id FROM assets WHERE EXISTS (SELECT 1 FROM files \
					WHERE files.asset_id = assets.id AND files.folder_id = ?) \(order)
					"""
				arguments = [folder]
			case .import(let run):
				sql = """
					SELECT id FROM assets WHERE EXISTS (SELECT 1 FROM files \
					WHERE files.asset_id = assets.id AND files.import_id = ?) \(order)
					"""
				arguments = [run]
			}
			return try Identifier<Asset>
				.fetchAll(database, sql: sql, arguments: arguments)
				.map(SubjectID.asset)

		case .files:
			let sql: String
			var arguments: StatementArguments = []
			switch source {
			case .library:
				sql = "SELECT id FROM files \(order)"
			case .folder(let folder):
				sql = "SELECT id FROM files WHERE folder_id = ? \(order)"
				arguments = [folder]
			case .import(let run):
				sql = "SELECT id FROM files WHERE import_id = ? \(order)"
				arguments = [run]
			}
			return try Identifier<File>
				.fetchAll(database, sql: sql, arguments: arguments)
				.map(SubjectID.file)
		}
	}
}
