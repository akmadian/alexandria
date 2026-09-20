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

// MARK: - Keyboard

/// The loupe's key surface (keybind round, 2026-09-20): the stage-surface
/// contract's minimal claimant until the loupe round builds the real viewer.
/// Claims focus from limbo, walks the cursor on arrows, and lets every other
/// key bubble to the menu. Attached by StageView behind the whole loupe
/// branch, so arrows work in the offline/empty postures too.
struct LoupeKeyHost: NSViewRepresentable {
	var onStep: (Int) -> Void

	func makeNSView(context: Context) -> LoupeKeyView {
		let view = LoupeKeyView()
		view.onStep = onStep
		return view
	}

	func updateNSView(_ view: LoupeKeyView, context: Context) {
		view.onStep = onStep
	}
}

final class LoupeKeyView: NSView {
	var onStep: (Int) -> Void = { _ in }

	override var acceptsFirstResponder: Bool { true }

	override func viewDidMoveToWindow() {
		super.viewDidMoveToWindow()
		if window != nil { claimKeyFocusFromLimbo() }
	}

	/// Navigation keys are interpreted here (a plain NSView doesn't route
	/// them to the move* selectors on its own); everything else takes
	/// super's DEFAULT path, which already forwards up the responder chain —
	/// unlike the grid, this view has no key-binding machinery to dodge, so
	/// there is nothing to hand-roll (review finding, 2026-09-20).
	override func keyDown(with event: NSEvent) {
		if event.isNavigationKey {
			interpretKeyEvents([event])
		} else {
			super.keyDown(with: event)
		}
	}

	override func moveLeft(_ sender: Any?) { onStep(-1) }
	override func moveRight(_ sender: Any?) { onStep(1) }
	override func moveUp(_ sender: Any?) { onStep(-1) }
	override func moveDown(_ sender: Any?) { onStep(1) }
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
