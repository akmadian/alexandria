//
//  Shell.swift
//  Alexandria
//
//  Created by ari on 9/11/26.
//

import SwiftUI

struct ShellView: View {
	@Environment(CatalogViewState.self) private var viewState

	var body: some View {
		NavigationSplitView {
			BrowserView()
				.navigationSplitViewColumnWidth(
					min: Theme.Pane.browserMinimumWidth,
					ideal: Theme.Pane.browserIdealWidth,
					max: Theme.Pane.browserMaximumWidth)
		} detail: {
			StageView()
				.navigationTitle(viewState.sourceTitle)
				.toolbar { stageToolbar }
		}
		.inspector(isPresented: inspectorPresented) {
			InspectorView()
				.inspectorColumnWidth(
					min: Theme.Pane.inspectorMinimumWidth,
					ideal: Theme.Pane.inspectorIdealWidth,
					max: Theme.Pane.inspectorMaximumWidth)
				.toolbar { inspectorToolbar }
		}
	}
	
	@ToolbarContentBuilder private var stageToolbar: some ToolbarContent {
		ToolbarSpacer(.flexible)
		
		ToolbarItemGroup(placement: .automatic) {
			Picker("View", selection: stageViewModeSelection) {
				Label("Grid", systemImage: "square.grid.2x2").tag(ViewMode.grid)
				Label("Loupe", systemImage: "loupe").tag(ViewMode.loupe)
			}.pickerStyle(.segmented)
			
			ControlGroup {
				Toggle(isOn: filterBarPresented) {
					Label("Filters", systemImage: "line.3.horizontal.decrease")
				}
				.toggleStyle(.button)
				Menu {
					// No chords here: chords are declared in MainMenu and
					// nowhere else (keybind round, 2026-09-20).
					Button("Zoom In", systemImage: "plus.magnifyingglass") {
						viewState.setGridColumns(viewState.gridColumns - 1)
					}
					Button("Zoom Out", systemImage: "minus.magnifyingglass") {
						viewState.setGridColumns(viewState.gridColumns + 1)
					}
					
					Divider()
					
					Button(
						"Reverse Sort Direction",
						systemImage: viewState.arrangement.direction == .ascending ? "arrow.up" : "arrow.down",
						action: {
							viewState.setArrangement(viewState.arrangement.reversed)
						}
					)
					Menu("Sort By") {
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
				} label: {
					Image(systemName: "ellipsis")
				}.menuIndicator(.hidden)
			}
		}
	}
	
	@ToolbarContentBuilder private var inspectorToolbar: some ToolbarContent {
		ToolbarItem(placement: .automatic) {
			Button("Toggle Inspector", systemImage: "sidebar.trailing") {
				viewState.setInspectorPresented(!viewState.inspectorPresented)
			}
		}
	}
	
	private var stageViewModeSelection: Binding<ViewMode> {
		Binding (
			get: { viewState.viewMode },
			set: { viewState.setViewMode($0)}
		)
	}
	
	private var filterBarPresented: Binding<Bool> {
		Binding (
			get: { viewState.filterBarPresented },
			set: { viewState.setFilterBarPresented($0) }
		)
	}

	private var inspectorPresented: Binding<Bool> {
		Binding (
			get: { viewState.inspectorPresented },
			set: { viewState.setInspectorPresented($0) }
		)
	}
}
