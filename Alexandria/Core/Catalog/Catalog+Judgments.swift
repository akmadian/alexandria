//
//  Catalog+Judgments.swift
//  Alexandria
//
//  The judgments round's verbs (2026-09-14): rating and flag, the two
//  judgment columns the schema already carries on `assets`.
//
//  The contract both verbs keep:
//  · One gesture, one transaction, one commit — so a batch judgment lands
//    as a single observation delivery, never as a flicker of partial state.
//  · Each returns the PRIOR value per asset, keyed by id, for every id that
//    named a real record. That map is undo's input (undo itself is a later
//    chunk). An id with no record is simply absent — a ghost id is not an
//    error, it judged nothing.
//  · `nil` clears (unrated / unflagged). A rating outside 1...5 is refused
//    by the verb BEFORE the write: the schema's CHECK is the last fence,
//    not the user-facing refusal.
//

import Foundation
import GRDB
import Logging

private nonisolated let log = Logger(label: "catalog")

/// The judgment verbs' refusals, named so the UI surfaces a real message
/// instead of an SQLite constraint code.
nonisolated enum JudgmentError: Error, Equatable {
	/// Ratings are 1...5; `nil` is unrated and 0 is not a rating (the
	/// schema's `CHECK (rating BETWEEN 1 AND 5)`).
	case invalidRating(Int)
}

extension Catalog {

	/// Rates the given assets, or clears their rating with `nil`. Returns
	/// each existing asset's prior rating.
	func setRating(
		_ ids: [Identifier<Asset>], to rating: Int?
	) async throws -> [Identifier<Asset>: Int?] {
		// Before any write, and before the empty-set shortcut: an
		// out-of-range rating is a caller mistake whatever the set size.
		if let rating, !(1...5).contains(rating) {
			throw JudgmentError.invalidRating(rating)
		}
		let priors = try await setJudgment(.rating, of: ids, to: rating)
		log.info("rating set", metadata: [
			"assets": "\(priors.count)",
			"rating": "\(rating.map(String.init) ?? "unrated")",
		])
		return priors
	}

	/// Flags the given assets, or clears their flag with `nil`. Returns each
	/// existing asset's prior flag.
	func setFlag(
		_ ids: [Identifier<Asset>], to flag: Asset.Flag?
	) async throws -> [Identifier<Asset>: Asset.Flag?] {
		let priors = try await setJudgment(.flag, of: ids, to: flag)
		log.info("flag set", metadata: [
			"assets": "\(priors.count)",
			"flag": "\(flag?.rawValue ?? "unflagged")",
		])
		return priors
	}

	/// The judgment columns on `assets`. A closed set of literals, so the
	/// shared body can name a column in SQL without a caller-supplied string
	/// ever reaching a statement.
	private enum JudgmentColumn: String {
		case rating, flag
	}

	/// Both verbs' body: read the priors and write the new value inside ONE
	/// transaction, so no reader can observe the set half-judged and the
	/// priors handed back are exactly the values this write replaced.
	///
	/// Two statements, both set-wide: one SELECT for the priors, one UPDATE
	/// for the write. Records that don't exist are absent from the SELECT
	/// and matched by neither statement.
	private func setJudgment<Value: DatabaseValueConvertible & Sendable>(
		_ column: JudgmentColumn, of ids: [Identifier<Asset>], to value: Value?
	) async throws -> [Identifier<Asset>: Value?] {
		guard !ids.isEmpty else { return [:] }
		return try await databaseWriter.write { database in
			let placeholders = databaseQuestionMarks(count: ids.count)
			let rows = try Row.fetchAll(
				database,
				sql: "SELECT id, \(column.rawValue) FROM assets WHERE id IN (\(placeholders))",
				arguments: StatementArguments(ids)
			)
			var priors: [Identifier<Asset>: Value?] = [:]
			for row in rows {
				let prior: Value? = row[column.rawValue]
				// updateValue, never the subscript: `priors[id] = prior`
				// with an Optional value type is the shape that silently
				// means "remove the key" for a nil prior.
				priors.updateValue(prior, forKey: row["id"])
			}
			try database.execute(
				sql: "UPDATE assets SET \(column.rawValue) = ? WHERE id IN (\(placeholders))",
				arguments: StatementArguments(
					[value?.databaseValue ?? .null] + ids.map(\.databaseValue)
				)
			)
			return priors
		}
	}
}

// MARK: - The judgment door (keybind round, 2026-09-20)

private nonisolated let doorLog = Logger(label: "judgments")

/// Fire-and-forget judgment writes, shared by every judgment door — the
/// menu's items, the inspector's controls. One implementation of the
/// pattern: call the verb, discard the returned prior values (undo is a
/// later chunk), log the failure. The verbs are total: an empty target list
/// writes nothing.
///
/// Each gesture is its own unstructured Task; nothing orders two gestures a
/// few milliseconds apart (key repeat, "3" then "5"), so the writer sees
/// them in resume order — FIFO in practice today. When undo lands and
/// ordering becomes load-bearing, this is where serialization goes.
extension Catalog {
	func applyRating(_ ids: [Identifier<Asset>], _ rating: Int?) {
		Task {
			do {
				_ = try await setRating(ids, to: rating)
			} catch {
				doorLog.error("rating failed", metadata: [
					"assets": "\(ids.count)",
					"error": "\(error)",
				])
			}
		}
	}

	func applyFlag(_ ids: [Identifier<Asset>], _ flag: Asset.Flag?) {
		Task {
			do {
				_ = try await setFlag(ids, to: flag)
			} catch {
				doorLog.error("flagging failed", metadata: [
					"assets": "\(ids.count)",
					"error": "\(error)",
				])
			}
		}
	}
}
