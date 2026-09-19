//
//  StageView.swift
//  Alexandria
//
//  Created by ari on 9/11/26.
//

import SwiftUI

struct StageView: View {
	@Environment(CatalogViewState.self) private var viewState

	/// The image engine lives HERE, above the renderers, so its decoded-image
	/// cache outlives a grid↔loupe swap (grid.md invariant 10). @State keeps
	/// the one instance across body switches; loupe will share it when it
	/// grows real pixels.
	@State private var imaging = StageImaging()

	var body: some View {
		VStack(spacing: 0) {
			// Always visible for now — collapse/toolbar-toggle semantics are
			// the filter-bar round's open ruling (R2); see FilterBar's header.
			if (viewState.filterBarPresented) {
				FilterBar()
				Divider()
			}
			stage
		}
		.safeAreaInset(edge: .bottom, spacing: 0) { StatusBar() }
	}

	@ViewBuilder private var stage: some View {
		if viewState.predicateUnreadable {
			// The ruled notice (smart-collection round, 2026-09-18): a
			// smart collection whose stored predicate can't be decoded
			// answers empty — but never SILENTLY empty, that reads as data
			// loss. Corruption today; version skew once the vocabulary
			// grows past generation 1.
			// TODO: (smart-collection editor round) a fuller story —
			// distinguish version-skew ("needs a newer Alexandria") from
			// corruption, and offer repair/recreate.
			ContentUnavailableView {
				Label("Can't Read This Collection's Filter", systemImage: "exclamationmark.triangle")
			} description: {
				Text("The saved filter couldn't be read. It may have been created by a newer version of Alexandria.")
			}
		} else {
			modeStage
		}
	}

	@ViewBuilder private var modeStage: some View {
		switch viewState.viewMode {
		case .grid:
			GridView(imaging: imaging)
		case .loupe:
			switch viewState.cursor {
			case .asset(let id):
				LoupeView(id: id)
			case .file:
				ContentUnavailableView("No file loupe yet", systemImage: "doc")
			case nil:
				ContentUnavailableView("Nothing Selected", systemImage: "info")
			}
		}
	}
}
