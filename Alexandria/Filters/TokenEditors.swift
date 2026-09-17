//
//  TokenEditors.swift
//  Alexandria
//
//  One editor view per value shape, one thin router, and each field's
//  starter token (filter-bar round, 2026-09-16). The shared contract:
//  (token in, onCommit out) — an editor keeps its draft in local @State and
//  fires onCommit only with a token that validates, so the bar's array
//  never holds an invalid token and live-apply never compiles nonsense
//  (ruled: invalidity can exist only inside an open editor). A new
//  vocabulary field later = one new editor + one router case.
//
//  Starter tokens live HERE, not in the vocabulary: what a field's pill
//  says on birth is a UI opinion (rating defaults to ≥ 3 — the LrC
//  "and higher" habit), not engine truth.
//

import SwiftUI

extension FilterToken.Field {
	/// The valid token a freshly added pill is born with — a pill is never
	/// in a half-built state (ruled: add inserts this, editor already open).
	var starterToken: FilterToken {
		switch self {
		case .rating:
			return FilterToken(field: .rating, op: .gte, value: .int(3))
		case .flag:
			return FilterToken(field: .flag, op: .eq, value: .option(Asset.Flag.pick.rawValue))
		case .kind:
			return FilterToken(field: .kind, op: .eq, value: .option(FileKind.image.rawValue))
		}
	}
}

/// Routes a token to its field's editor. The pill presents whatever this
/// returns in its popover and never knows which editor it got.
struct TokenEditor: View {
	let token: FilterToken
	let onCommit: (FilterToken) -> Void

	var body: some View {
		switch token.field {
		case .rating: RatingTokenEditor(token: token, onCommit: onCommit)
		case .flag: FlagTokenEditor(token: token, onCommit: onCommit)
		case .kind: KindTokenEditor(token: token, onCommit: onCommit)
		}
	}
}

// MARK: - Rating

struct RatingTokenEditor: View {
	let onCommit: (FilterToken) -> Void

	@State private var op: FilterToken.Operator
	@State private var scalar: Int
	@State private var lo: Int
	@State private var hi: Int
	@State private var negated: Bool

	init(token: FilterToken, onCommit: @escaping (FilterToken) -> Void) {
		self.onCommit = onCommit
		_op = State(initialValue: token.op)
		_negated = State(initialValue: token.negated)
		var scalar = 3, lo = 2, hi = 4
		switch token.value {
		case .int(let n): scalar = n
		case .range(.int(let a), .int(let b)): lo = a; hi = b
		default: break
		}
		_scalar = State(initialValue: scalar)
		_lo = State(initialValue: lo)
		_hi = State(initialValue: hi)
	}

	var body: some View {
		VStack(alignment: .leading, spacing: 8) {
			Picker("Rating", selection: $op) {
				ForEach(FilterToken.Field.rating.operators, id: \.self) { op in
					Text(op.displayName).tag(op)
				}
			}
			switch op {
			case .isUnset:
				EmptyView()
			case .between:
				// Setting a bound past the other drags the other along, so
				// the draft can't pass through an inverted range.
				LabeledContent("From") {
					StarRating(lo) { if let n = $0 { lo = n; hi = max(hi, n) } }
				}
				LabeledContent("To") {
					StarRating(hi) { if let n = $0 { hi = n; lo = min(lo, n) } }
				}
			default:
				// A reclick on the current star reports nil (StarRating's
				// unrate); a comparison always needs an operand, so keep it.
				StarRating(scalar) { if let n = $0 { scalar = n } }
			}
			Toggle("Exclude matches", isOn: $negated)
		}
		.onChange(of: op) { commit() }
		.onChange(of: scalar) { commit() }
		.onChange(of: lo) { commit() }
		.onChange(of: hi) { commit() }
		.onChange(of: negated) { commit() }
	}

	private var draft: FilterToken {
		let value: FilterToken.Value?
		switch op {
		case .isUnset: value = nil
		case .between: value = .range(.int(lo), .int(hi))
		default: value = .int(scalar)
		}
		return FilterToken(field: .rating, op: op, value: value, negated: negated)
	}

	private func commit() {
		let token = draft
		guard (try? token.validate()) != nil else { return }
		onCommit(token)
	}
}

// MARK: - Flag

struct FlagTokenEditor: View {
	let onCommit: (FilterToken) -> Void

	@State private var op: FilterToken.Operator
	@State private var flag: Asset.Flag
	@State private var negated: Bool

	init(token: FilterToken, onCommit: @escaping (FilterToken) -> Void) {
		self.onCommit = onCommit
		_op = State(initialValue: token.op)
		_negated = State(initialValue: token.negated)
		var flag = Asset.Flag.pick
		if case .option(let raw)? = token.value, let stored = Asset.Flag(rawValue: raw) {
			flag = stored
		}
		_flag = State(initialValue: flag)
	}

	var body: some View {
		VStack(alignment: .leading, spacing: 8) {
			Picker("Flag", selection: $op) {
				ForEach(FilterToken.Field.flag.operators, id: \.self) { op in
					Text(op.displayName).tag(op)
				}
			}
			if op == .eq {
				Picker("Value", selection: $flag) {
					Text("Pick").tag(Asset.Flag.pick)
					Text("Reject").tag(Asset.Flag.reject)
				}
				.pickerStyle(.segmented)
				.labelsHidden()
			}
			Toggle("Exclude matches", isOn: $negated)
		}
		.onChange(of: op) { commit() }
		.onChange(of: flag) { commit() }
		.onChange(of: negated) { commit() }
	}

	private func commit() {
		let token = FilterToken(
			field: .flag, op: op,
			value: op == .isUnset ? nil : .option(flag.rawValue),
			negated: negated
		)
		guard (try? token.validate()) != nil else { return }
		onCommit(token)
	}
}

// MARK: - Kind

struct KindTokenEditor: View {
	let onCommit: (FilterToken) -> Void

	@State private var kind: FileKind
	@State private var negated: Bool

	init(token: FilterToken, onCommit: @escaping (FilterToken) -> Void) {
		self.onCommit = onCommit
		_negated = State(initialValue: token.negated)
		var kind = FileKind.image
		if case .option(let raw)? = token.value, let stored = FileKind(rawValue: raw) {
			kind = stored
		}
		_kind = State(initialValue: kind)
	}

	var body: some View {
		VStack(alignment: .leading, spacing: 8) {
			// kind's operator set is {eq}, so there is no operator picker —
			// a one-element picker is a dead affordance. The editor offers
			// the known kinds; the token stores the raw string (the column
			// is open, FilterVocabulary's kind note).
			Picker("Kind", selection: $kind) {
				ForEach(FileKind.allCases, id: \.self) { kind in
					Text(kind.rawValue.capitalized).tag(kind)
				}
			}
			Toggle("Exclude matches", isOn: $negated)
		}
		.onChange(of: kind) { commit() }
		.onChange(of: negated) { commit() }
	}

	private func commit() {
		let token = FilterToken(
			field: .kind, op: .eq, value: .option(kind.rawValue), negated: negated
		)
		guard (try? token.validate()) != nil else { return }
		onCommit(token)
	}
}

#if DEBUG
#Preview("Rating") {
	RatingTokenEditor(token: FilterToken.Field.rating.starterToken) { _ in }
		.padding()
}

#Preview("Flag") {
	FlagTokenEditor(token: FilterToken.Field.flag.starterToken) { _ in }
		.padding()
}

#Preview("Kind") {
	KindTokenEditor(token: FilterToken.Field.kind.starterToken) { _ in }
		.padding()
}
#endif
