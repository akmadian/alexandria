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
}
