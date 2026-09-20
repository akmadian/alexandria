//
//  GridItem.swift
//  Alexandria
//
//  The deliberately dumb cell (grid round 2026-09-12; recomposed for the
//  cell round 2026-09-18): it asks for nothing and holds no request; whether
//  the pixels it's handed are still the right ones for the id on screen is
//  the coordinator's call, made against the one id↔position table before it
//  ever calls `show`.
//
//  Composition, top to bottom: a plain root view (clicks fall up the
//  responder chain to NSCollectionView's native tracking; double-click
//  lives on GridCollectionView), a hit-test-transparent hosting view,
//  and the SwiftUI `GridCell` (zones + content slot). The pixels stay on
//  `ThumbnailLeafView` — the image is the leaf layer's `contents`, swapped
//  inside an actions-disabled CATransaction: an atomic compositor swap that
//  never erases to the ground and never runs an implicit fade. That is the
//  structural fix for the swap-flash — there is no drawRect pass to catch
//  the gray ground mid-swap, because there is no drawRect. The represent/
//  reuse contract with the coordinator is unchanged; only the decoration
//  moved into SwiftUI (proven at the slot-shape spike, 2026-09-18: zero
//  body evaluations per imperative paint, one hosting view per item, ever).
//

import AppKit
import SwiftUI

/// The pixel leaf: layer contents, painted imperatively by the coordinator
/// through the item. Never a click target — decoration and selection route
/// around it, not through it.
@MainActor final class ThumbnailLeafView: NSView {

	init() {
		super.init(frame: .zero)
		wantsLayer = true
		guard let layer else { return }
		// The quiet ground shows through until (and in the letterbox bars of)
		// an aspect-fit thumbnail — fixed geometry from first paint.
		// layer.contentsGravity = .resizeAspect
	}

	required init?(coder: NSCoder) { fatalError("ThumbnailLeafView is code-only") }

	/// Display pixels, as an atomic layer-contents swap — implicit animations
	/// off so nothing fades and the ground is never revealed between old and
	/// new pixels.
	func show(_ image: NSImage) {
		guard let layer else { return }
		let cgImage = image.cgImage(forProposedRect: nil, context: nil, hints: nil)
		CATransaction.begin()
		CATransaction.setDisableActions(true)
		layer.contentsScale = window?.backingScaleFactor ?? 2
		layer.contents = cgImage
		CATransaction.commit()
	}

	/// Fall back to the quiet ground — no thumbnail yet, or the slot was
	/// recycled off its pixels. Also atomic, so a clear never flashes.
	func showPlaceholder() {
		guard let layer else { return }
		CATransaction.begin()
		CATransaction.setDisableActions(true)
		layer.contents = nil
		CATransaction.commit()
	}

	override func hitTest(_ point: NSPoint) -> NSView? { nil }
}

@MainActor final class GridItem: NSCollectionViewItem {

	static let identifier = NSUserInterfaceItemIdentifier("GridItem")

	private(set) var representedID: SubjectID?

	private let thumbnail = ThumbnailLeafView()
	/// Internal (not private) so tests can pin the reuse-reset contract;
	/// production writers go through `display`/the prominence didSets.
	let state = CellState()

	/// Set by the coordinator's mirror — the item can't know cursor identity
	/// on its own (AppKit has no cursor concept, only selection).
	var isCursor = false {
		didSet { if oldValue != isCursor { updateProminence() } }
	}

	override func loadView() {
		// Plain root: an unhandled mouseDown walks the responder chain to
		// the collection view, so selection stays native machinery.
		let container = NSView()
		let hosting = CellHostingView(rootView: GridCell(state: state, content: thumbnail))
		hosting.sizingOptions = []
		// Fill-parent via autoresizing, not Auto Layout — no constraint
		// solving per cell in a surface with hundreds of live items (the
		// grid-resize round's carried "hosting autoresizing lever").
		hosting.frame = container.bounds
		hosting.autoresizingMask = [.width, .height]
		container.addSubview(hosting)
		view = container
	}

	// Pixels pass straight through to the leaf; the coordinator's contract
	// is with the item and doesn't know the composition changed.
	func show(_ image: NSImage) { thumbnail.show(image) }
	func showPlaceholder() { thumbnail.showPlaceholder() }

	// Decoration data, coordinator-filled: position at configure and after
	// deliveries (a diff shifts positions without reconfigure), records at
	// configure and when resolution lands.
	func display(position: Int) { state.position = position }
	func display(asset: Asset?, file: File?) {
		state.asset = asset
		state.file = file
	}

	func represent(_ id: SubjectID) {
		representedID = id
	}

	override func prepareForReuse() {
		super.prepareForReuse()
		representedID = nil
		// The WHOLE display state resets — including the native selection
		// flags, which AppKit only reliably pushes when they change, so a
		// recycled cell could otherwise carry a ring onto an unselected id.
		isCursor = false
		isSelected = false
		highlightState = .none
		state.clear()
		showPlaceholder()
	}

	override var isSelected: Bool {
		didSet { updateProminence() }
	}
	override var highlightState: NSCollectionViewItem.HighlightState {
		didSet { updateProminence() }
	}

	private func updateProminence() {
		state.prominence = CellProminence.resolve(
			isCursor: isCursor, isSelected: isSelected, highlightState: highlightState
		)
	}
}

/// Hit-test transparent: every click falls through to the interaction root
/// so selection stays native machinery. (Spike finding, 2026-09-18:
/// NSHostingView otherwise captures its WHOLE frame, even over regions with
/// no interactive content. When a cell control needs clicks — the metadata
/// mode's star — this becomes a registered-rects carve, proven at the same
/// spike; until then, blanket transparency IS the policy.)
private final class CellHostingView<Content: View>: NSHostingView<Content> {
	override func hitTest(_ point: NSPoint) -> NSView? { nil }
}

