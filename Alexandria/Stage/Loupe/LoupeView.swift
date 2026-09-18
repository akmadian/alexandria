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

// MARK: - Preview

// LoupeView's body is entirely @Query-driven, so a live catalog is needed to
// reach the offline/loading branches. What paints without a database is the
// content the loupe actually shows — Quick Look over a real file.
#Preview("Fallback — image") {
	LoupeFallbackView(url: fixture("real-jpg_6150009.JPG"))
		.frame(width: 480, height: 360)
}

#Preview("Fallback — video") {
	LoupeFallbackView(url: fixture("video-422-10bit.mov"))
		.frame(width: 480, height: 360)
}

/// repo-root/TestData, resolved from this source file's location — same trick
/// the test suites use. Dev-machine paths are fine here: previews never ship.
private func fixture(_ name: String) -> URL {
	URL(fileURLWithPath: #filePath)
		.deletingLastPathComponent()   // Loupe/
		.deletingLastPathComponent()   // Stage/
		.deletingLastPathComponent()   // Alexandria/
		.deletingLastPathComponent()   // repo root
		.appending(path: "TestData")
		.appending(path: name)
}
