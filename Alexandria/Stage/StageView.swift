//
//  StageView.swift
//  Alexandria
//
//  Created by ari on 9/11/26.
//

import SwiftUI

struct StageView: View {
	@Environment(CatalogViewState.self) private var viewState

	var body: some View {
		switch viewState.viewMode {
		case .grid:
			GridView()
		case .loupe:
			Text("Loupe")
		}
	}
}
