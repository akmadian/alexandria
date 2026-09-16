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
