//
//  FilterBar.swift
//  Alexandria
//
//  The filter's authoring surface (filter-bar round, 2026-09-16): a row of
//  pills, an add menu, clear-all. The bar owns the LEDGER — which pill is
//  which — because the engine's tokens deliberately carry no identity
//  (FilterNode.swift): Pill below is display identity only, minted on
//  screen, never serialized, stripped before setFilter.
//
//  Every gesture is the ratified shape: edit the local array, derive the
//  whole group, commit atomically through the hub's setFilter (live apply,
//  ruled — no apply button). The hub stays canonical: a filter value that
//  isn't the bar's own echo (cleared elsewhere; a future smart collection
//  applying one) wins and rebuilds the ledger with fresh identities.
//
//  DELIBERATELY UNSETTLED (this round, ruled to carry):
//  · Collapse/toolbar-toggle semantics (indicator-when-hidden vs
//    collapse-disables) — the bar is always visible until that ruling.
//  · Facet/match counts on pills or menu rows.
//  · Keybinds for add/clear.
//  · Tree authoring: this bar reads and writes ONE flat AND root. A tree
//    root from elsewhere would drop its groups here — asserted below, and
//    unreachable until something else can author trees.
//

import SwiftUI

struct FilterBar: View {
	@Environment(CatalogViewState.self) private var viewState

	/// One pill on screen: display identity + the value it shows.
	private struct Pill: Identifiable {
		let id = UUID()
		var token: FilterToken
	}

	@State private var pills: [Pill] = []
	/// The group this bar last committed — how an external hub change is
	/// told apart from our own value echoing back through observation.
	@State private var lastCommitted: FilterGroup?
	/// The pill whose editor opens on first appearance (the just-added one).
	@State private var newbornID: Pill.ID?

	var body: some View {
		HStack(spacing: 6) {
			ForEach(pills) { pill in
				FilterPill(
					token: pill.token,
					startsEditing: pill.id == newbornID,
					onReplace: { replace(pill.id, with: $0) },
					onRemove: { remove(pill.id) }
				)
			}
			addMenu
			if !pills.isEmpty {
				Button("Clear All") { clear() }
					.buttonStyle(.plain)
					.font(.callout)
					.foregroundStyle(.secondary)
			}
			Spacer()
		}
		.padding(.horizontal, 10)
		.padding(.vertical, 6)
		.onAppear { rebuild(from: viewState.filter) }
		.onChange(of: viewState.filter) { _, changed in
			guard changed != lastCommitted else { return }
			rebuild(from: changed)
		}
	}

	private var addMenu: some View {
		Menu {
			ForEach(FilterToken.Field.allCases, id: \.self) { field in
				Button(field.displayName, systemImage: field.icon) { add(field) }
			}
		} label: {
			Label("Add Filter", systemImage: "plus.circle")
				.labelStyle(.iconOnly)
		}
		.menuStyle(.borderlessButton)
		.fixedSize()
		.accessibilityLabel("Add filter")
	}

	// MARK: Gestures — each edits the ledger, then commits the whole group

	private func add(_ field: FilterToken.Field) {
		let pill = Pill(token: field.starterToken)
		pills.append(pill)
		newbornID = pill.id
		commit()
	}

	private func replace(_ id: Pill.ID, with token: FilterToken) {
		guard let index = pills.firstIndex(where: { $0.id == id }) else { return }
		pills[index].token = token
		commit()
	}

	private func remove(_ id: Pill.ID) {
		pills.removeAll { $0.id == id }
		commit()
	}

	private func clear() {
		pills = []
		commit()
	}

	/// Ledger → group → setFilter, ids stripped. Empty ledger commits nil
	/// (one representation of "no filter", same as the hub's normalize).
	private func commit() {
		let group = pills.isEmpty
			? nil
			: FilterGroup(combine: .and, children: pills.map { .token($0.token) })
		lastCommitted = group
		viewState.setFilter(group)
	}

	/// Hub → ledger, fresh identities. Flat P0: the root's children are all
	/// tokens; see the tree-authoring marker in the header.
	private func rebuild(from group: FilterGroup?) {
		lastCommitted = group
		newbornID = nil
		pills = (group?.children ?? []).compactMap { node in
			if case .token(let token) = node { return Pill(token: token) }
			assertionFailure("FilterBar cannot author tree-shaped filters yet")
			return nil
		}
	}
}

#if DEBUG
import GRDB

#Preview {
	let catalog = try! Catalog(DatabaseQueue())
	return FilterBar()
		.environment(\.catalog, catalog)
		.environment(CatalogViewState(catalog: catalog))
		.frame(width: 600)
}
#endif
