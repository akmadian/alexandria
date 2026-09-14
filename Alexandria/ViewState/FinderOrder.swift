//
//  FinderOrder.swift
//  Alexandria
//
//  The one Finder-style ordering (browser round, 2026-09-11; extracted in
//  the collections round so the sidebar and the union walk cannot drift):
//  localizedStandardCompare on the display name — "Shoot 9" before
//  "Shoot 10" — with id as a total-order tiebreak. Deterministic per
//  locale, which is what a presentation ordering owes.
//
//  This is a DISPLAY rule and it lives beside the display layers on
//  purpose: Core orderings stay byte- or numeric-deterministic, and
//  nothing under Core/ may call this.
//

import Foundation

nonisolated enum FinderOrder {
	static func ascending(
		_ a: (name: String, id: UUID), _ b: (name: String, id: UUID)
	) -> Bool {
		switch a.name.localizedStandardCompare(b.name) {
		case .orderedAscending: true
		case .orderedDescending: false
		case .orderedSame: a.id.uuidString < b.id.uuidString
		}
	}
}
