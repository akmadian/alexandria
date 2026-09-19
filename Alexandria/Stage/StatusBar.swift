//
//  StatusBar.swift
//  Alexandria
//
//  Created by ari on 9/18/26.
//

import SwiftUI

struct StatusBar: View {
	@Environment(CatalogViewState.self) private var viewState
	
	var body: some View {
		HStack() {
			Text(summary)
				.font(.caption)
				.foregroundStyle(.secondary)
			
			Spacer()
			
			if viewState.viewMode == .grid {
				ControlGroup {
					Button(
						"Zoom Out",
						systemImage: "minus",
						action: { viewState.setGridColumns(viewState.gridColumns + 1) }
					)
					Button(
						"Zoom In",
						systemImage: "plus",
						action: { viewState.setGridColumns(viewState.gridColumns - 1) }
					)
				}
				ControlGroup {
					Button(
						"Reverse Sort Direction",
						systemImage: viewState.arrangement.direction == .ascending ? "arrow.up" : "arrow.down",
						action: {
							viewState.setArrangement(viewState.arrangement.reversed)
						}
					)
					Menu(viewState.arrangement.sortKey.rawValue.capitalized) {
						ForEach(Arrangement.SortKey.allCases) { key in
							Button {
								viewState.setArrangement(viewState.arrangement.setSortKey(to: key))
							} label: {
								if (viewState.arrangement.sortKey == key) {
									Label(key.rawValue.capitalized, systemImage: "checkmark")
								} else {
									Text(key.rawValue.capitalized)
								}
							}
						}
					}
				}
			}
		}
		.controlSize(.small)
		.padding(.horizontal, 4)
		.padding(.vertical, 2)
		.background(.bar)
		.overlay(alignment: .top) { Divider() }
	}
	
	/// "N items", "k of N selected", with a filtered marker — the lazy
	/// stand-in for Finder's "x of y" until an unfiltered count is worth
	/// a second observation.
	private var summary: String {
		let count = viewState.workingSet.count
		let base = viewState.selection.isEmpty
		? String(localized: "\(count) items")
		: "\(viewState.selection.count) of \(count) selected"
		return viewState.filter == nil ? base : base + "  ·  filtered"
	}
	
	/// Inverted on purpose: zoom IN = fewer columns. Grid-only tenant —
	/// zoom means nothing in loupe, and hiding beats disabling for a
	/// mode-irrelevant control.
	private var zoomStepper: some View {
		Stepper("Zoom") {
			viewState.setGridColumns(viewState.gridColumns - 1)
		} onDecrement: {
			viewState.setGridColumns(viewState.gridColumns + 1)
		}
		.labelsHidden()
		.help("Grid Zoom")
	}
	
	private var gridColumns: Binding<Int> {
		Binding (
			get: { viewState.gridColumns },
			set: { viewState.setGridColumns($0)}
		)
	}
}
