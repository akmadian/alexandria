//
//  FilterVocabulary.swift
//  Alexandria
//
//  What a filter token can say (filter round, 2026-09-16): the P0 fields —
//  rating, flag, kind, the three judgment/kind columns on `assets` — their
//  operators, and the value shapes, with the validation that keeps a
//  nonsense token from ever compiling to legal-but-always-empty SQL (an
//  unexplained empty grid is the worst diagnostic outcome available).
//
//  Operator sets are FIELD-owned, seeded from the value type but corrected
//  per field's domain (adversarial pass, S5): `kind` is NOT NULL, so it
//  never offers isUnset — the operator would be a dead affordance.
//
//  Deferred vocabulary, each on its own round: filename/text (wants FTS
//  thinking), capture date (wants range UI), `missing`, "not in any
//  collection". DELIBERATELY UNSETTLED: file-unit fields — their round must
//  first rule the quantifier scope of file-unit tokens under the asset lens
//  (one file satisfies ALL criteria vs any file per criterion; adversarial
//  pass N4). No per-field unit property exists until then — every P0 field
//  is asset-unit and a one-valued property is a speculative field.
//

/// A refused token or envelope, named so a fence can tell a human what was
/// wrong instead of surfacing an SQLite error or a silent empty grid.
nonisolated enum FilterError: Error, Equatable {
	case operatorNotAllowed(field: String, op: String)
	/// The value's shape doesn't fit the field (e.g. a string on rating),
	/// or a non-isUnset operator arrived with no value at all.
	case valueMismatch(field: String)
	/// Ratings are 1...5 in filters exactly as in judgments; 0 is not a
	/// rating and neither is 6.
	case ratingOutOfRange(Int)
	/// `between` bounds must be same-typed scalars with lo ≤ hi — an
	/// inverted range compiles to legal, always-empty SQL, so it is
	/// refused at the fence instead.
	case invalidRange
	/// An option value outside the field's domain (e.g. flag = "picked").
	case unknownOption(field: String, value: String)
	// Persistence-fence refusals (Filter+Persistence.swift):
	case unsupportedVersion(Int)
	case unknownField(String)
	case unknownOperator(String)
	case malformedValue
	/// A stored filter that normalizes to nothing: setFilter normalizes
	/// empty to nil before any save, so an empty stored root is corrupt
	/// data, not a quiet no-op.
	case emptyFilter
}

nonisolated extension FilterToken {

	/// The P0 vocabulary. All three live on `assets`; raw values are the
	/// serialized field names, so a rename is a format migration by
	/// definition.
	enum Field: String, CaseIterable, Sendable {
		case rating
		case flag
		/// Compared as the raw stored string, so a token survives the kind
		/// set being open (the FileKind enum's openness contradiction is
		/// tracked separately; the filter deliberately does not lean on it).
		case kind
	}

	enum Operator: String, CaseIterable, Sendable {
		case eq, lt, lte, gt, gte
		/// Inclusive on both ends (ruled: LrC's stance, and SQL BETWEEN's —
		/// the operator means exactly what the SQL says). The bounds ride
		/// in the value as `.range`.
		case between
		/// The deliberate route to unrated/unflagged. Takes no value.
		/// There is NO neq: "not equals" is the NOT toggle on eq — two
		/// spellings of one pill would let equal filters render unequal.
		case isUnset
	}

	/// A token's operand. `range` recurses so `between` is not welded to
	/// Int (adversarial pass, S4) — future date/size fields reuse it.
	indirect enum Value: Hashable, Sendable {
		case int(Int)
		case option(String)
		case range(Value, Value)
	}
}

nonisolated extension FilterToken.Field {
	/// Field-owned operator set, in the UI's presentation order.
	var operators: [FilterToken.Operator] {
		switch self {
		case .rating: return [.eq, .lt, .lte, .gt, .gte, .between, .isUnset]
		case .flag: return [.eq, .isUnset]
		case .kind: return [.eq]
		}
	}
}

// MARK: - Validation

nonisolated extension FilterToken {
	/// Refuses a token that could only ever compile to nonsense. Runs at
	/// the persistence fence (decode) and as the hub's debug assertion on
	/// setFilter — the P0 UI builds from these same enums, so an invalid
	/// token from a live gesture is a programmer error, while one from a
	/// stored blob is data corruption and gets the loud, typed refusal.
	func validate() throws {
		guard field.operators.contains(op) else {
			throw FilterError.operatorNotAllowed(field: field.rawValue, op: op.rawValue)
		}
		switch op {
		case .isUnset:
			guard value == nil else { throw FilterError.valueMismatch(field: field.rawValue) }
		case .between:
			guard case .range(let lo, let hi)? = value else {
				throw FilterError.valueMismatch(field: field.rawValue)
			}
			try validateScalar(lo)
			try validateScalar(hi)
			guard case (.int(let a), .int(let b)) = (lo, hi) else {
				// P0's one between field is int-typed; same-typed scalar
				// pairs of other kinds arrive with the fields that need them.
				throw FilterError.invalidRange
			}
			guard a <= b else { throw FilterError.invalidRange }
		case .eq, .lt, .lte, .gt, .gte:
			guard let value else { throw FilterError.valueMismatch(field: field.rawValue) }
			if case .range = value { throw FilterError.valueMismatch(field: field.rawValue) }
			try validateScalar(value)
		}
	}

	/// One scalar against the field's domain.
	private func validateScalar(_ scalar: Value) throws {
		switch (field, scalar) {
		case (.rating, .int(let n)):
			guard (1...5).contains(n) else { throw FilterError.ratingOutOfRange(n) }
		case (.flag, .option(let raw)):
			guard Asset.Flag(rawValue: raw) != nil else {
				throw FilterError.unknownOption(field: field.rawValue, value: raw)
			}
		case (.kind, .option(let raw)):
			guard !raw.isEmpty else {
				throw FilterError.unknownOption(field: field.rawValue, value: raw)
			}
		default:
			throw FilterError.valueMismatch(field: field.rawValue)
		}
	}
}

nonisolated extension FilterGroup {
	/// Every leaf beneath, one refusal short-circuits.
	func validate() throws {
		for child in children {
			try child.validate()
		}
	}
}

nonisolated extension FilterNode {
	/// The tree's one recursion idiom — the normalized() twin.
	func validate() throws {
		switch self {
		case .token(let token): try token.validate()
		case .group(let group): try group.validate()
		}
	}
}
