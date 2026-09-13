//
//  GridImaging.swift
//  Alexandria
//
//  The pure half of the grid's image machinery (grid round, 2026-09-12,
//  rebuilt clean): the grid-specific decisions the coordinator acts on, with
//  no Nuke and no AppKit — same discipline as GridDiff. The coordinator
//  reads live geometry (cell size, backing scale) and the store's ladder and
//  passes them in; what comes back is a decision it hands to the engine.
//
//  There is ONE size per cell (the settled size for its geometry). The
//  "small while moving, sharp on settle" tiering was removed this round: no
//  battle-tested grid does it (Apple Photos, Nuke, Kingfisher, SDWebImage,
//  Lightroom, Capture One), and it was the source of the whole-viewport
//  settle flash. Scroll cost is handled by request priority + prefetch, not
//  by resolution — which is where the ecosystem actually puts it.
//

import Foundation

/// Why an image is being loaded — maps to engine priority so speculation
/// cannot outrank demand. Two intents only: a cell the user can see, and one
/// ahead of the viewport.
nonisolated enum Urgency: Sendable {
	case content
	case preheat
}

nonisolated enum GridImaging {

	/// The tier for a cell: the smallest rung on the store's ladder that
	/// still covers the cell's longest edge in PHYSICAL pixels (points ×
	/// backing scale), clamped to the largest rung the store holds. A 228pt
	/// cell on a 2× display needs 456px → the 512 rung. The grid does not
	/// invent sizes; it picks one the store can decode cleanly.
	static func bucket(forCellSide side: CGFloat, scale: CGFloat, ladder: [DecodeBucket]) -> DecodeBucket {
		let physical = side * max(scale, 1)
		if let covering = ladder.first(where: { CGFloat($0.pixels) >= physical }) {
			return covering
		}
		// Past the store's ceiling (a near-loupe column count): the largest
		// rung, upscaled into the cell. A bigger stored tier is the loupe
		// round's answer, not the grid's.
		return ladder.last ?? DecodeBucket(pixels: max(1, Int(physical.rounded(.up))))
	}

	/// Whether freshly decoded pixels should replace what a cell already
	/// shows: only larger-or-equal wins, so an instant paint from a smaller
	/// cached size is never overwritten by nothing, and a late arrival can't
	/// downgrade a sharper image already on screen.
	static func shouldPaint(incoming: DecodeBucket, over shown: DecodeBucket?) -> Bool {
		guard let shown else { return true }
		return incoming >= shown
	}

	/// Whether the stamp-watch heal should re-request a cell. The stamp event
	/// is valueless, so a heal sweeps every visible cell — but it must only
	/// spend (and burn its one-shot on) a cell whose bytes exist NOW. Offering
	/// a not-yet-written thumbnail would fail its decode AND consume the
	/// one-shot, so the cell could never heal when its own bytes finally land
	/// (the invariant-4 bug this guards). The one-shot still stops a genuinely
	/// corrupt (present-but-undecodable) thumbnail from looping, because that
	/// cell's file exists and its offer is spent on the real failure.
	static func shouldOfferHeal(
		placeholder: Bool, inFlight: Bool, alreadyOffered: Bool, fileExists: Bool
	) -> Bool {
		placeholder && !inFlight && !alreadyOffered && fileExists
	}
}
