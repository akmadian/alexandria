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
	/// Subtree by ruling (browser round, 2026-09-11): a folder shows
	/// everything in it AND its subfolders, no direct-only mode. A toggle
	/// is maybe-later — the LrC record shows the buried mode switch is the
	/// pain, not either behavior.
	case folder(Identifier<Folder>)
	/// One specific historical import. No sidebar row yet; a browsing UI
	/// for import history is a future round.
	case `import`(Identifier<Import>)
	/// "Previous Import": the most recent import, resolved at fetch time so
	/// each new import replaces the answer (LrC's settled meaning, ratified
	/// 2026-09-11) — and because the compiled statement reads the imports
	/// table, the observation re-delivers the moment a new import begins.
	case latestImport
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

		// The subtree walk (ratified: a folder source reaches everything
		// beneath it): the folder plus every descendant, by parent_id.
		// UNION (not UNION ALL) so a parent_id cycle terminates instead of
		// spinning — the schema doesn't forbid one, only the write path's
		// shape. The trailing space is the separator for concatenation.
		let subtree = "WITH RECURSIVE subtree(id) AS (SELECT ? UNION "
			+ "SELECT folders.id FROM folders JOIN subtree ON folders.parent_id = subtree.id) "
		// "Latest" is by started_at (ISO text: lexicographic order is
		// chronological), id as the same-millisecond tiebreak. Outcome is
		// deliberately ignored: a still-running import IS the previous
		// import, growing live.
		let latestImport = "(SELECT id FROM imports ORDER BY started_at DESC, id DESC LIMIT 1)"

		switch lens {
		case .assets:
			// Narrowing sources drive the asset lens from files, not from a
			// per-asset EXISTS over the whole assets table: measured 135x
			// cheaper on a narrowed source at 40k assets (round review,
			// 2026-09-11). `asset_id IS NOT NULL` is the formation-pending
			// exclusion the EXISTS form got implicitly.
			let sql: String
			var arguments: StatementArguments = []
			switch source {
			case .library:
				sql = "SELECT id FROM assets \(order)"
			case .folder(let folder):
				sql = subtree + """
					SELECT DISTINCT asset_id FROM files \
					WHERE folder_id IN subtree AND asset_id IS NOT NULL \
					ORDER BY asset_id \(direction)
					"""
				arguments = [folder]
			case .import(let run):
				sql = """
					SELECT DISTINCT asset_id FROM files \
					WHERE import_id = ? AND asset_id IS NOT NULL \
					ORDER BY asset_id \(direction)
					"""
				arguments = [run]
			case .latestImport:
				sql = """
					SELECT DISTINCT asset_id FROM files \
					WHERE import_id = \(latestImport) AND asset_id IS NOT NULL \
					ORDER BY asset_id \(direction)
					"""
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
				sql = subtree + "SELECT id FROM files WHERE folder_id IN subtree \(order)"
				arguments = [folder]
			case .import(let run):
				sql = "SELECT id FROM files WHERE import_id = ? \(order)"
				arguments = [run]
			case .latestImport:
				sql = "SELECT id FROM files WHERE import_id = \(latestImport) \(order)"
			}
			return try Identifier<File>
				.fetchAll(database, sql: sql, arguments: arguments)
				.map(SubjectID.file)
		}
	}
}
