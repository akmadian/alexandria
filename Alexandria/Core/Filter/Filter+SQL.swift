//
//  Filter+SQL.swift
//  Alexandria
//
//  What a filter means in the database (filter round, 2026-09-16): a pure
//  walk from the tree to one WHERE predicate over `assets` columns, built
//  as a GRDB SQL literal so every operand rides WITH its placeholder —
//  binding order is structural, never positional bookkeeping (adversarial
//  pass, B2). The one consumer is WorkingSetQuery, which splices the
//  predicate into its membership subqueries; nobody else ever sees SQL.
//
//  Columns are table-qualified (`assets.kind`, not `kind`) because the
//  splice sites join `files`, which carries columns of the same names.
//
//  NULL semantics, both ratified:
//  · Positive comparisons are literal three-valued logic — an unrated
//    asset matches NO rating comparison (isUnset is the deliberate route
//    to unrated; LrC's unrated-as-zero conflation is its wart, not its
//    wisdom).
//  · negated compiles to `(expr) IS NOT TRUE` — the true complement, so
//    NULL rows land on the NOT side. A naive NOT(expr) is NULL for NULL
//    operands and silently drops unrated rows from BOTH sides.
//

import GRDB
import Logging

private nonisolated let log = Logger(label: "filter")

nonisolated extension FilterGroup {
	/// The group's predicate: children joined by the combinator,
	/// parenthesized. An empty group compiles to its combinator's identity
	/// element (AND → 1, OR → 0) so the walk is total — normalized() makes
	/// the case unreachable through the hub and the persistence fence, and
	/// a test pins the backstop.
	func sqlPredicate() -> SQL {
		guard !children.isEmpty else {
			return combine == .and ? SQL(sql: "1") : SQL(sql: "0")
		}
		let separator = combine == .and ? " AND " : " OR "
		let joined = children.map { $0.sqlPredicate() }.joined(separator: separator)
		return "(" + joined + ")"
	}
}

nonisolated extension FilterNode {
	func sqlPredicate() -> SQL {
		switch self {
		case .token(let token): return token.sqlPredicate()
		case .group(let group): return group.sqlPredicate()
		}
	}
}

nonisolated extension FilterToken {
	/// One leaf's predicate. Assumes a token the fences validated; a
	/// malformed one fails CLOSED — it compiles to `0` (matches nothing)
	/// with a debug assertion, never to a predicate that silently widens
	/// the set (the one failure a filter must never have).
	func sqlPredicate() -> SQL {
		let column = SQL(sql: field.column)
		let positive: SQL
		switch op {
		case .isUnset:
			// The inner expression is never NULL, so the complement
			// normalizes to readable SQL instead of (… IS NULL) IS NOT TRUE.
			return negated ? column + " IS NOT NULL" : column + " IS NULL"
		case .eq, .lt, .lte, .gt, .gte:
			guard let operand = value?.scalarDatabaseValue else { return failClosed() }
			positive = column + SQL(sql: " \(op.comparison) ") + "\(operand)"
		case .between:
			guard case .range(let lo, let hi)? = value,
			      let lower = lo.scalarDatabaseValue,
			      let upper = hi.scalarDatabaseValue
			else { return failClosed() }
			positive = column + " BETWEEN \(lower) AND \(upper)"
		}
		return negated ? "(" + positive + ") IS NOT TRUE" : positive
	}

	/// The assertion is debug-only, so release ALSO logs — otherwise this
	/// path is the unexplained empty grid the round declared the worst
	/// diagnostic outcome (round review, finding 1).
	private func failClosed() -> SQL {
		assertionFailure("unvalidated filter token reached the compiler")
		log.error("unvalidated filter token compiled fail-closed (matches nothing)", metadata: [
			"field": "\(field.rawValue)",
			"op": "\(op.rawValue)",
		])
		return SQL(sql: "0")
	}
}

private nonisolated extension FilterToken.Field {
	var column: String {
		switch self {
		case .rating: return "assets.rating"
		case .flag: return "assets.flag"
		case .kind: return "assets.kind"
		}
	}
}

private nonisolated extension FilterToken.Operator {
	/// The comparison spellings; between and isUnset have their own shapes.
	var comparison: String {
		switch self {
		case .eq: return "="
		case .lt: return "<"
		case .lte: return "<="
		case .gt: return ">"
		case .gte: return ">="
		case .between, .isUnset: preconditionFailure("not a comparison operator")
		}
	}
}

private nonisolated extension FilterToken.Value {
	/// The bindable form of a scalar; nil for a range (which only between
	/// consumes, bound per bound).
	var scalarDatabaseValue: DatabaseValue? {
		switch self {
		case .int(let n): return n.databaseValue
		case .option(let s): return s.databaseValue
		case .range: return nil
		}
	}
}
