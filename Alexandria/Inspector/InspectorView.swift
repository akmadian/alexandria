//
//  InspectorView.swift
//  Alexandria
//
//  Created by ari on 9/11/26.
//

import SwiftUI

struct InspectorView: View {
	@Environment(CatalogViewState.self) private var viewState
	
	var body: some View {
		switch viewState.cursor {
		case .asset(let id):
			AssetInspector(id: id)
		case .file:
			ContentUnavailableView("No file inspector yet", systemImage: "doc")
		case nil:
			ContentUnavailableView("Nothing Selected", systemImage: "info")
		}
	}
}
