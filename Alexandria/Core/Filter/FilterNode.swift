//
//  FilterNode.swift
//  Alexandria
//
//  The filter's shape (filter round, 2026-09-16): a tree whose leaves are
//  tokens and whose interior nodes are groups — ratified tree-capable from
//  the start, while P0 authoring stays flat (one AND root of tokens). The
//  vocabulary (fields, operators, values) lives in FilterVocabulary.swift;
//  SQL meaning in Filter+SQL.swift; persistence in Filter+Persistence.swift.
//
//  These are plain values on purpose: every UI gesture is a pure
//  (FilterGroup) → FilterGroup transform committed atomically through the
//  hub's setFilter, so torn intermediate trees are unobservable and a
//  previous value IS its own undo record. Tokens carry no identity — two
//  equal tokens are the same token. A pill UI that needs stable row
//  identity owns its own identified wrapper; an id here would poison the
//  serialized envelope and every equality (ruled at the adversarial pass).
//
//  DELIBERATELY UNSETTLED: tree AUTHORING. The path-addressed mutation
//  helpers (update(at:)/remove(at:)/modifyGroup(at:)) and their ordering
//  rules (remove before insert; batch removals in descending path order)
//  land with the tree-UI round — P0's flat bar needs only array ops on
//  the root's children, and helpers with zero callers don't ship.
//

/// A leaf condition: one field compared to one value. `negated` is the NOT
/// toggle and lives ONLY on leaves — groups carry no negation (ratified;
/// leaf-only negation is fully expressive, De Morgan holds under SQL's
/// three-valued logic).
nonisolated struct FilterToken: Hashable, Sendable {
	var field: Field
	var op: Operator
	/// nil exactly when the operator takes no operand (isUnset) — enforced
	/// by validate(), so equal filters can't differ by junk operands.
	var value: Value?
	var negated = false
}

/// An interior node: children joined by one combinator. Children mix tokens
/// and groups freely (forbidding the mix would force synthetic singleton
/// groups around every lone token beside a group). Root positions — the
/// hub's posture field, the serialized envelope — are typed FilterGroup,
/// not FilterNode, so a bare-token root is unrepresentable.
nonisolated struct FilterGroup: Hashable, Sendable {
	var combine: Combine
	var children: [FilterNode]

	enum Combine: String, Sendable {
		case and, or
	}
}

/// Either shape, where the tree recurses.
nonisolated indirect enum FilterNode: Hashable, Sendable {
	case token(FilterToken)
	case group(FilterGroup)
}

nonisolated extension FilterGroup {
	/// Prunes empty groups; an empty root becomes nil — so "no filter" has
	/// exactly one representation (nil) and the compiler's identity-element
	/// backstop for empty groups stays unreachable in practice. The
	/// Arrangement.normalized(for:) idiom: pure rule here, hub applies it in
	/// setFilter, decode applies it at the persistence fence. Singleton and
	/// nested same-combine groups are deliberately legal and untouched —
	/// degenerate shapes compile fine; only emptiness is meaningless.
	func normalized() -> FilterGroup? {
		let pruned = children.compactMap { $0.normalized() }
		return pruned.isEmpty ? nil : FilterGroup(combine: combine, children: pruned)
	}
}

nonisolated extension FilterNode {
	/// A token survives; a group survives if anything beneath it did.
	func normalized() -> FilterNode? {
		switch self {
		case .token:
			return self
		case .group(let group):
			return group.normalized().map(FilterNode.group)
		}
	}
}
