//
//  Shell.swift
//  Alexandria
//
//  Created by ari on 9/11/26.
//

import SwiftUI

struct ShellView: View {
	@Environment(CatalogViewState.self) private var viewState
	@State private var inspectorPresented = true
	
	var body: some View {
		NavigationSplitView {
			BrowserView()
				.navigationSplitViewColumnWidth(
					min: Theme.Pane.browserMinimumWidth,
					ideal: Theme.Pane.browserIdealWidth,
					max: Theme.Pane.browserMaximumWidth)
		} detail: {
			StageView()
				.toolbar(removing: .title)
				.toolbar { stageToolbar }
		}
		.inspector(isPresented: $inspectorPresented) {
			InspectorView()
				.inspectorColumnWidth(
					min: Theme.Pane.inspectorMinimumWidth,
					ideal: Theme.Pane.inspectorIdealWidth,
					max: Theme.Pane.inspectorMaximumWidth)
				.toolbar { inspectorToolbar }
		}
	}
	
	@ToolbarContentBuilder private var stageToolbar: some ToolbarContent {
		ToolbarItem(placement: .navigation) {
			Text("Alexandria")
		}.sharedBackgroundVisibility(.hidden)

		ToolbarSpacer(.flexible)
		
		ToolbarItemGroup(placement: .automatic) {
			Button("Zoom Out", systemImage: "minus.magnifyingglass") {viewState.setGridColumns(viewState.gridColumns + 1)}
			Button("Zoom In", systemImage: "plus.magnifyingglass") {viewState.setGridColumns(viewState.gridColumns - 1)}
			Toggle(isOn: filterBarPresented) {
				Label("Filters", systemImage: "line.3.horizontal.decrease")
			}
			.toggleStyle(.button)
			Picker("View", selection: stageViewModeSelection) {
				Label("Grid", systemImage: "square.grid.2x2").tag(ViewMode.grid)
				Label("Loupe", systemImage: "loupe").tag(ViewMode.loupe)
			}.pickerStyle(.segmented)
		}
	}
	
	@ToolbarContentBuilder private var inspectorToolbar: some ToolbarContent {
		ToolbarItem(placement: .automatic) {
			Button("Toggle Inspector", systemImage: "sidebar.trailing") {
				inspectorPresented.toggle()
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
}
