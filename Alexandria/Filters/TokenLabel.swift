//
//  TokenLabel.swift
//  Alexandria
//
//  The display vocabulary for one token (filter-bar round, 2026-09-16):
//  structured parts, not a preformatted string, so the pill's composition
//  can style field, operator, and value independently — stars as stars, a
//  dimmed glyph, an icon-only field. Lives in the UI layer on purpose: the
//  engine is ratified UI-free, so how a token READS is decided here.
//

import Foundation

struct TokenLabel {
	/// SF Symbol for the field.
	let icon: String
	let fieldName: String
	/// Nil when the value text carries the whole story (between's "2–4").
	let operatorGlyph: String?
	/// Nil when the operator takes no operand (isUnset — the glyph says it).
	let valueText: String?
	let negated: Bool

	init(for token: FilterToken) {
		icon = token.field.icon
		fieldName = token.field.displayName
		negated = token.negated
		switch token.op {
		case .isUnset:
			operatorGlyph = "is unset"
			valueText = nil
		case .between:
			operatorGlyph = nil
			valueText = Self.text(for: token.value)
		default:
			operatorGlyph = token.op.glyph
			valueText = Self.text(for: token.value)
		}
	}

	private static func text(for value: FilterToken.Value?) -> String? {
		switch value {
		case .int(let n):
			return "\(n)"
		case .option(let raw):
			return raw.capitalized
		case .range(let lo, let hi):
			return [text(for: lo), text(for: hi)].compactMap(\.self).joined(separator: "–")
		case nil:
			return nil
		}
	}
}

extension FilterToken.Field {
	var icon: String {
		switch self {
		case .rating: return "star"
		case .flag: return "flag"
		case .kind: return "photo"
		}
	}

	var displayName: String {
		rawValue.capitalized
	}
}

extension FilterToken.Operator {
	/// The compact in-pill form. isUnset and between never render through
	/// this (TokenLabel special-cases both); their glyphs exist so a picker
	/// row can still show something terse beside the name.
	var glyph: String {
		switch self {
		case .eq: return "="
		case .lt: return "<"
		case .lte: return "≤"
		case .gt: return ">"
		case .gte: return "≥"
		case .between: return "–"
		case .isUnset: return "∅"
		}
	}

	/// The editor picker's row text.
	var displayName: String {
		switch self {
		case .eq: return "is"
		case .lt: return "less than"
		case .lte: return "at most"
		case .gt: return "more than"
		case .gte: return "at least"
		case .between: return "between"
		case .isUnset: return "is unset"
		}
	}
}
