//
//  GridItem.swift
//  Alexandria
//
//  The deliberately dumb cell (grid round, 2026-09-12; rebuilt clean): a
//  quiet placeholder ground, a thumbnail, and the native selection ring —
//  nothing else. It asks for nothing and holds no request; whether the
//  pixels it's handed are still the right ones for the id on screen is the
//  coordinator's call, made against the one id↔position table before it ever
//  calls `show`.
//
//  The image is the cell layer's `contents`, swapped inside an
//  actions-disabled CATransaction — an atomic compositor swap that never
//  erases to the ground and never runs an implicit fade. That is the
//  structural fix for the swap-flash: there is no drawRect pass to catch the
//  gray ground mid-swap, because there is no drawRect. Cell design (badges,
//  labels) is its own future round; it replaces this content while the
//  represent/reuse contract with the coordinator stays fixed.
//

import AppKit

@MainActor final class GridItem: NSCollectionViewItem {

	static let identifier = NSUserInterfaceItemIdentifier("GridItem")

	/// Fires on double-click; the coordinator wires it to loupe activation.
	var onDoubleClick: (() -> Void)?

	private(set) var representedID: SubjectID?

	override func loadView() {
		let container = GridItemInteractionView()
		container.wantsLayer = true
		container.onDoubleClick = { [weak self] in self?.onDoubleClick?() }
		guard let layer = container.layer else { view = container; return }
		// The quiet ground shows through until (and in the letterbox bars of)
		// an aspect-fit thumbnail — fixed geometry from first paint.
		layer.backgroundColor = Theme.Grid.placeholder.cgColor
		layer.contentsGravity = .resizeAspect
		view = container
	}

	/// Display pixels the coordinator resolved for this slot, as an atomic
	/// layer-contents swap — implicit animations off so nothing fades and the
	/// ground is never revealed between old and new pixels.
	func show(_ image: NSImage) {
		guard let layer = view.layer else { return }
		let cgImage = image.cgImage(forProposedRect: nil, context: nil, hints: nil)
		CATransaction.begin()
		CATransaction.setDisableActions(true)
		layer.contentsScale = view.window?.backingScaleFactor ?? 2
		layer.contents = cgImage
		CATransaction.commit()
	}

	/// Fall back to the quiet ground — no thumbnail yet, or the slot was
	/// recycled off its pixels. Also atomic, so a clear never flashes.
	func showPlaceholder() {
		guard let layer = view.layer else { return }
		CATransaction.begin()
		CATransaction.setDisableActions(true)
		layer.contents = nil
		CATransaction.commit()
	}

	func represent(_ id: SubjectID) {
		representedID = id
	}

	override func prepareForReuse() {
		super.prepareForReuse()
		representedID = nil
		showPlaceholder()
	}

	// Selection ring: driven by the native selection state, drawn as a layer
	// border. Styling is deliberately minimal this round.
	override var isSelected: Bool {
		didSet { updateSelectionRing() }
	}
	override var highlightState: NSCollectionViewItem.HighlightState {
		didSet { updateSelectionRing() }
	}

	private func updateSelectionRing() {
		let highlighted = isSelected
			|| highlightState == .forSelection
			|| highlightState == .asDropTarget
		view.layer?.borderWidth = highlighted ? Theme.Grid.selectionRingWidth : 0
		view.layer?.borderColor = NSColor.controlAccentColor.cgColor
	}
}

/// The item's root view: passes clicks up to NSCollectionView's own tracking
/// (selection stays native machinery) and surfaces double-clicks.
@MainActor private final class GridItemInteractionView: NSView {
	var onDoubleClick: (() -> Void)?

	override func mouseDown(with event: NSEvent) {
		// Super first: the collection view's selection handling runs before
		// activation, so a double-click activates the item it just selected.
		super.mouseDown(with: event)
		if event.clickCount == 2 {
			onDoubleClick?()
		}
	}
}
