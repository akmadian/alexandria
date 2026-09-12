//
//  GridItem.swift
//  Alexandria
//
//  The deliberately dumb cell (grid round, 2026-09-12): an aspect-fit
//  thumbnail slot and the native selection ring, nothing else. Image
//  loading is deliberately absent — the engine is prescribed by
//  _design/technical/grid.md — so the slot renders the quiet placeholder
//  ground. Cell design (badges, labels, layout) is its own future round;
//  that round replaces this item's CONTENT, while the
//  represent/prepareForReuse contract with the coordinator is the slot's
//  fixed edge.
//

import AppKit

@MainActor final class GridItem: NSCollectionViewItem {

	static let identifier = NSUserInterfaceItemIdentifier("GridItem")

	/// Fires on double-click; the coordinator wires it to loupe activation.
	var onDoubleClick: (() -> Void)?

	private let thumbnailView = NSImageView()
	private(set) var representedID: SubjectID?

	override func loadView() {
		let container = GridItemInteractionView()
		container.wantsLayer = true
		container.onDoubleClick = { [weak self] in self?.onDoubleClick?() }
		thumbnailView.imageScaling = .scaleProportionallyUpOrDown
		thumbnailView.translatesAutoresizingMaskIntoConstraints = false
		container.addSubview(thumbnailView)
		NSLayoutConstraint.activate([
			thumbnailView.leadingAnchor.constraint(equalTo: container.leadingAnchor),
			thumbnailView.trailingAnchor.constraint(equalTo: container.trailingAnchor),
			thumbnailView.topAnchor.constraint(equalTo: container.topAnchor),
			thumbnailView.bottomAnchor.constraint(equalTo: container.bottomAnchor),
		])
		view = container
	}

	func represent(_ id: SubjectID) {
		representedID = id
	}

	override func prepareForReuse() {
		super.prepareForReuse()
		representedID = nil
		thumbnailView.image = nil
	}

	// Selection ring: driven by the native selection state, drawn as a
	// layer border. Styling is deliberately minimal this round.
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

/// The item's root view: passes clicks up to NSCollectionView's own
/// tracking (selection stays native machinery) and surfaces double-clicks.
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
