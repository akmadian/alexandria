//
//  LoupeView.swift
//  Alexandria
//
//  Created by ari on 9/16/26.
//

import SwiftUI
import GRDBQuery
import Logging

private nonisolated let log = Logger(label: "loupe")

struct LoupeView: View {
	@Query<RepresentativeFileLocationRequest> private var repFileLocation: Location?

	init(id: Identifier<Asset>) {
		_repFileLocation = Query(constant: RepresentativeFileLocationRequest(assetId: id, log: log))
	}

	var body: some View {
		if let repFileLocation {
			if let url = repFileLocation.fileUrl {
				// Every kind renders through Quick Look for now; bespoke per-kind
				// views (and the switch that routes to them) come back when they exist.
				LoupeFallbackView(url: url)
			} else {
				// Catalog identity is known but the bytes are offline — the volume
				// carrying the representative file isn't mounted.
				ContentUnavailableView(
					"Offline",
					systemImage: "externaldrive.badge.xmark",
					description: Text("“\(repFileLocation.volume.name)” isn’t mounted.")
				)
			}
		} else {
			ProgressView()
		}
	}
}
