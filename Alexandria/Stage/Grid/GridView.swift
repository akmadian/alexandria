//
//  GridView.swift
//  Alexandria
//
//  The grid's SwiftUI face (grid round, 2026-09-12): composes the AppKit
//  bridge with the grid's chrome — size control, empty states — so the
//  stage stays a mode switch that knows no grid internals. A pure renderer
//  of the hub: reads posture and answer, calls intents, owns no copies.
//

import SwiftUI

struct GridView: View {
	@Environment(CatalogViewState.self) private var viewState
	/// The grid reads the catalog directly for the things that aren't shared
	/// question/position — thumbnail resolution and the stamp watch — exactly
	/// as the browser reads it for its tree (hub doc: pixels never route
	/// through the hub).
	@Environment(\.catalog) private var catalog
	let imaging: StageImaging

	var body: some View {
		GridRepresentable(
			workingSet: viewState.workingSet,
			answeredQuery: viewState.answeredQuery,
			selection: viewState.selection,
			cursor: viewState.cursor,
			columns: viewState.gridColumns,
			catalog: catalog,
			imaging: imaging,
			onSelectionChange: { viewState.setSelection($0) },
			onCursorMove: { viewState.moveCursor(to: $0) },
			onActivate: { viewState.setViewMode(.loupe) }
		)
		.overlay { emptyState }
	}

	/// An unanswered question (nil) renders as nothing — the answer is in
	/// flight and the previous one stands; only a real empty answer says so.
	@ViewBuilder private var emptyState: some View {
		if viewState.answeredQuery != nil && viewState.workingSet.isEmpty {
			ContentUnavailableView("No Items", systemImage: "square.grid.2x2")
		}
	}
}
