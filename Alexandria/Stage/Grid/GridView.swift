//
//  GridView.swift
//  Alexandria
//
//  The grid's SwiftUI face (grid round, 2026-09-12): composes the AppKit
//  bridge with the grid's chrome — size control, empty states — so the
//  stage stays a mode switch that knows no grid internals. A pure renderer
//  of the hub: reads posture and answer, calls intents, owns no copies.
//
//  The switch confirmation (drag round, 2026-09-18) lives here: a reorder
//  drop under a non-manual sort proposes adoption, and only this dialog's
//  explicit yes writes — a judgment-class ordering is never silently
//  overwritten, by ruling. On yes the verb runs BEFORE the arrangement
//  intent, so the grid settles once, onto the adopted order.
//

import AppKit
import Logging
import SwiftUI

private nonisolated let log = Logger(label: "grid")

struct GridView: View {
	@Environment(CatalogViewState.self) private var viewState
	/// The grid reads the catalog directly for the things that aren't shared
	/// question/position — thumbnail resolution and the stamp watch — exactly
	/// as the browser reads it for its tree (hub doc: pixels never route
	/// through the hub).
	@Environment(\.catalog) private var catalog
	let imaging: StageImaging

	@State private var pendingReorder: PendingReorder?

	var body: some View {
		GridRepresentable(
			workingSet: viewState.workingSet,
			answeredQuery: viewState.answeredQuery,
			selection: viewState.selection,
			cursor: viewState.cursor,
			columns: viewState.gridColumns,
			stageClaimToken: viewState.stageClaimToken,
			stageClaimHandledToken: viewState.stageClaimHandledToken,
			onStageClaimHandled: { viewState.markStageClaimHandled($0) },
			catalog: catalog,
			imaging: imaging,
			onSelectionChange: { viewState.setSelection($0) },
			onCursorMove: { viewState.moveCursor(to: $0) },
			onActivate: { viewState.setViewMode(.loupe) },
			onReorderProposal: { pendingReorder = $0 }
		)
		.overlay { emptyState }
		.alert(
			"Switch to Manual Order?",
			isPresented: Binding(
				get: { pendingReorder != nil },
				set: { if !$0 { pendingReorder = nil } }
			),
			presenting: pendingReorder
		) { pending in
			Button("Switch") { adopt(pending) }
			Button("Cancel", role: .cancel) {}
		} message: { _ in
			Text("The order currently shown, with your change, becomes this collection's manual order.")
		}
	}

	/// The confirmed adoption: the verb first (one transaction), then the
	/// arrangement intent — the observation re-asks under manual and lands
	/// on the freshly minted keys, so the viewport settles exactly once.
	private func adopt(_ pending: PendingReorder) {
		let catalog = catalog
		let viewState = viewState
		Task {
			do {
				try await catalog.setManualOrder(pending.orderedAssets, in: pending.collection)
				viewState.setArrangement(viewState.arrangement.setSortKey(to: .manual))
			} catch {
				// A stale cover (the answer moved between drop and confirm):
				// refuse whole, tell the human, change nothing.
				NSSound.beep()
				log.error("manual-order adoption refused", metadata: ["error": "\(error)"])
			}
		}
	}

	/// An unanswered question (nil) renders as nothing — the answer is in
	/// flight and the previous one stands; only a real empty answer says so.
	@ViewBuilder private var emptyState: some View {
		if viewState.answeredQuery != nil && viewState.workingSet.isEmpty {
			ContentUnavailableView("No Items", systemImage: "square.grid.2x2")
		}
	}
}
