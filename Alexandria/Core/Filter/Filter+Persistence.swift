//
//  Filter+Persistence.swift
//  Alexandria
//
//  How a filter survives (filter round, 2026-09-16): versioned JSON,
//  structure not SQL — stored SQL would round-trip into pills only through
//  a SQL parser (Finder's RawQuery is the shipping write-only cautionary
//  tale) and would bake column names into user data. The public face is
//  two members on FilterGroup: serialized() and init(serialized:). The
//  envelope is a private wire detail, not a domain noun.
//
//  `version` is the VOCABULARY generation, not just the format's: ANY new
//  field or operator bumps it (adversarial pass, S2). A reader that meets
//  a newer version refuses loudly and whole — skipping unknown tokens
//  would WIDEN the matched set, and silent widening is the one failure a
//  filter must never have. The future smart-collection round decodes this
//  lazily, per use, from a String? column — one corrupt predicate disables
//  one row, never a whole fetch (recorded there; constrained here).
//
//  Wire form, kept boring on purpose:
//    {"version":1,"root":{"combine":"and","children":[
//      {"token":{"field":"rating","op":"gte","value":{"int":3},"negated":false}},
//      {"group":{...}}]}}
//  Values are TAGGED ({"int":3}), never bare scalars — self-describing
//  beats pretty in a format that outlives schemas (S3). Encoding sorts
//  keys, so encode→decode→encode is byte-stable (pinned by test).
//

import Foundation

nonisolated extension FilterGroup {

	/// The current vocabulary generation. Bump for ANY change a version-1
	/// reader could misread: new field, new operator, new node kind.
	static let serializationVersion = 1

	/// The versioned JSON for the smart-collection predicate column.
	func serialized() throws -> String {
		let encoder = JSONEncoder()
		encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
		let data = try encoder.encode(Envelope(version: Self.serializationVersion, root: self))
		return String(decoding: data, as: UTF8.self)
	}

	/// The persistence fence: version check, then decode, then the same
	/// validation and normalization every filter passes at setFilter — a
	/// stored blob gets no trust a live gesture doesn't. Every refusal is
	/// a typed FilterError (or a DecodingError for structural JSON rot).
	init(serialized: String) throws {
		let data = Data(serialized.utf8)
		let decoder = JSONDecoder()
		// Version first, on its own probe, so a vocabulary refusal names
		// the real problem instead of surfacing as an unknown-field error
		// from deep inside the tree.
		let probe = try decoder.decode(VersionProbe.self, from: data)
		// Only the generations THIS reader knows — a sub-1 or garbage
		// version is as unidentifiable as a newer one (round review,
		// finding 5). Grows to a set as migrations arrive.
		guard probe.version == Self.serializationVersion else {
			throw FilterError.unsupportedVersion(probe.version)
		}
		// The blob parses twice (probe, then envelope). Fine for a hub
		// posture; the smart-collection round's per-row lazy decode should
		// fold the version check into one pass if it measures.
		let envelope = try decoder.decode(Envelope.self, from: data)
		try envelope.root.validate()
		guard let normalized = envelope.root.normalized() else {
			// setFilter normalizes empty to nil before any save, so an
			// empty stored root is corruption, not a quiet no-op.
			throw FilterError.emptyFilter
		}
		self = normalized
	}
}

private nonisolated struct Envelope: Codable {
	var version: Int
	var root: FilterGroup
}

private nonisolated struct VersionProbe: Decodable {
	var version: Int
}

// MARK: - Codable implementations

// Field and Operator ride their raw values; unknown raw values are turned
// into typed refusals by FilterToken's decode below.
nonisolated extension FilterToken.Field: Codable {}
nonisolated extension FilterToken.Operator: Codable {}
nonisolated extension FilterGroup.Combine: Codable {}

nonisolated extension FilterToken: Codable {
	private enum CodingKeys: String, CodingKey {
		case field, op, value, negated
	}

	init(from decoder: Decoder) throws {
		let container = try decoder.container(keyedBy: CodingKeys.self)
		// Raw strings first so an unknown name is a named vocabulary
		// refusal ("I parsed this node, I don't know this field"), never a
		// generic decode failure.
		let fieldRaw = try container.decode(String.self, forKey: .field)
		guard let field = Field(rawValue: fieldRaw) else {
			throw FilterError.unknownField(fieldRaw)
		}
		let opRaw = try container.decode(String.self, forKey: .op)
		guard let op = Operator(rawValue: opRaw) else {
			throw FilterError.unknownOperator(opRaw)
		}
		self.field = field
		self.op = op
		self.value = try container.decodeIfPresent(Value.self, forKey: .value)
		self.negated = try container.decodeIfPresent(Bool.self, forKey: .negated) ?? false
	}

	func encode(to encoder: Encoder) throws {
		var container = encoder.container(keyedBy: CodingKeys.self)
		try container.encode(field, forKey: .field)
		try container.encode(op, forKey: .op)
		try container.encodeIfPresent(value, forKey: .value)
		try container.encode(negated, forKey: .negated)
	}
}

nonisolated extension FilterToken.Value: Codable {
	private enum CodingKeys: String, CodingKey {
		case int, option, range
	}

	init(from decoder: Decoder) throws {
		let container = try decoder.container(keyedBy: CodingKeys.self)
		guard container.allKeys.count == 1, let key = container.allKeys.first else {
			throw FilterError.malformedValue
		}
		switch key {
		case .int:
			self = .int(try container.decode(Int.self, forKey: .int))
		case .option:
			self = .option(try container.decode(String.self, forKey: .option))
		case .range:
			let bounds = try container.decode([FilterToken.Value].self, forKey: .range)
			guard bounds.count == 2 else { throw FilterError.malformedValue }
			self = .range(bounds[0], bounds[1])
		}
	}

	func encode(to encoder: Encoder) throws {
		var container = encoder.container(keyedBy: CodingKeys.self)
		switch self {
		case .int(let n): try container.encode(n, forKey: .int)
		case .option(let s): try container.encode(s, forKey: .option)
		case .range(let lo, let hi): try container.encode([lo, hi], forKey: .range)
		}
	}
}

nonisolated extension FilterNode: Codable {
	private enum CodingKeys: String, CodingKey {
		case token, group
	}

	init(from decoder: Decoder) throws {
		let container = try decoder.container(keyedBy: CodingKeys.self)
		guard container.allKeys.count == 1, let key = container.allKeys.first else {
			throw FilterError.malformedValue
		}
		switch key {
		case .token:
			self = .token(try container.decode(FilterToken.self, forKey: .token))
		case .group:
			self = .group(try container.decode(FilterGroup.self, forKey: .group))
		}
	}

	func encode(to encoder: Encoder) throws {
		var container = encoder.container(keyedBy: CodingKeys.self)
		switch self {
		case .token(let token): try container.encode(token, forKey: .token)
		case .group(let group): try container.encode(group, forKey: .group)
		}
	}
}

nonisolated extension FilterGroup: Codable {
	// Hand-written only because Swift won't synthesize Codable in a
	// cross-file extension — this is the derived implementation, verbatim.
	private enum CodingKeys: String, CodingKey {
		case combine, children
	}

	init(from decoder: Decoder) throws {
		let container = try decoder.container(keyedBy: CodingKeys.self)
		self.combine = try container.decode(Combine.self, forKey: .combine)
		self.children = try container.decode([FilterNode].self, forKey: .children)
	}

	func encode(to encoder: Encoder) throws {
		var container = encoder.container(keyedBy: CodingKeys.self)
		try container.encode(combine, forKey: .combine)
		try container.encode(children, forKey: .children)
	}
}
