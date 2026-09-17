//
//  FilterPill.swift
//  Alexandria
//
//  One token on screen (filter-bar round, 2026-09-16). Contract B, ruled:
//  value in, events out — the pill renders a token and PROPOSES a
//  replacement or its own removal; the bar turns proposals into a whole-
//  group transform through setFilter. No binding: commit policy belongs to
//  the bar, and a pill handed a token and two closures previews with no
//  hub and no catalog.
//
//  The body below is deliberately plain-but-working scaffold — Ari owns
//  the composition. The wiring a restyle must keep: tap presents
//  TokenEditor in the popover, edits arrive via onReplace, the close
//  control fires onRemove.
//

import SwiftUI

struct FilterPill: View {
	let token: FilterToken
	let onReplace: (FilterToken) -> Void
	let onRemove: () -> Void

	@State private var editing: Bool

	/// `startsEditing` opens the popover on first appearance — a freshly
	/// added pill is born with its editor up (ruled). It seeds @State, so
	/// later renders passing false never slam an open editor shut.
	init(
		token: FilterToken,
		startsEditing: Bool = false,
		onReplace: @escaping (FilterToken) -> Void,
		onRemove: @escaping () -> Void
	) {
		self.token = token
		self.onReplace = onReplace
		self.onRemove = onRemove
		_editing = State(initialValue: startsEditing)
	}

	var body: some View {
		let label = TokenLabel(for: token)
//		HStack(spacing: 4) {
//			ControlGroup {
//				Menu {
//					Picker("Field", selection)
//				}
//			}
//		}
		HStack(spacing: 4) {
			Image(systemName: label.icon)
			if label.negated {
				Text("not")
					.italic()
					.foregroundStyle(.secondary)
			}
			Text(label.fieldName)
			if let glyph = label.operatorGlyph {
				Text(glyph)
					.foregroundStyle(.secondary)
			}
			if let value = label.valueText {
				Text(value)
			}
			Button {
				onRemove()
			} label: {
				Image(systemName: "xmark.circle.fill")
					.foregroundStyle(.secondary)
			}
			.buttonStyle(.plain)
			.accessibilityLabel("Remove filter")
		}
		.font(.callout)
		.padding(.horizontal, 8)
		.padding(.vertical, 3)
		.background(.quaternary, in: Capsule())
		.contentShape(Capsule())
		.onTapGesture { editing = true }
		.popover(isPresented: $editing, arrowEdge: .bottom) {
			TokenEditor(token: token, onCommit: onReplace)
				.padding()
				.frame(minWidth: 220)
		}
	}
}

#if DEBUG
#Preview("Pills") {
	VStack(alignment: .leading, spacing: 10) {
		FilterPill(
			token: FilterToken(field: .rating, op: .gte, value: .int(3)),
			onReplace: { _ in }, onRemove: {}
		)
		FilterPill(
			token: FilterToken(field: .rating, op: .between, value: .range(.int(2), .int(4)), negated: true),
			onReplace: { _ in }, onRemove: {}
		)
		FilterPill(
			token: FilterToken(field: .flag, op: .isUnset, value: nil),
			onReplace: { _ in }, onRemove: {}
		)
		FilterPill(
			token: FilterToken(field: .kind, op: .eq, value: .option("image")),
			onReplace: { _ in }, onRemove: {}
		)
	}
	.padding()
}
#endif
