//
//  GridItem.swift
//  Alexandria
//
//  NSCollectionView's adapter, and nothing else (value architecture,
//  2026-09-20). The item owes the host exactly four things: mount SwiftUI
//  in the AppKit slot, receive the model (one write point — reuse means
//  "assign the empty value"), translate AppKit's selection pushes into
//  the model (isSelected/highlightState arrive HERE by protocol), and
//  anchor staleness (representedID — async deliveries land after reuse
//  re-targets a cell, so the coordinator checks it before writing).
//
//  Pixels ride the model as data (NSImage, the decoded-pixel container);
//  display is SwiftUI's. The old imperative paint seam (ThumbnailLeafView,
//  CATransaction swaps) was measured against this shape and retired —
//  spike 2026-09-20: no frame cost at fast-scroll scale.
//

import AppKit
import SwiftUI

@MainActor final class GridItem: NSCollectionViewItem {

	static let identifier = NSUserInterfaceItemIdentifier("GridItem")

	private(set) var representedID: SubjectID?

	/// The one write point: assign (or mutate a field) and the cell
	/// re-renders. Equality-guarded so no-op writes cost nothing.
	var model = CellModel.empty {
		didSet {
			guard model != oldValue else { return }
			hosting.rootView = GridCell(model: model)
		}
	}

	private lazy var hosting = CellHostingView(rootView: GridCell(model: model))

	/// Set by the coordinator's mirror — the item can't know cursor identity
	/// on its own (AppKit has no cursor concept, only selection).
	var isCursor = false {
		didSet { if oldValue != isCursor { updateProminence() } }
	}

	override func loadView() {
		// Plain root: an unhandled mouseDown walks the responder chain to
		// the collection view, so selection stays native machinery.
		let container = NSView()
		hosting.sizingOptions = []
		// Fill-parent via autoresizing, not Auto Layout — no constraint
		// solving per cell in a surface with hundreds of live items.
		hosting.frame = container.bounds
		hosting.autoresizingMask = [.width, .height]
		container.addSubview(hosting)
		view = container
	}

	func represent(_ id: SubjectID) {
		representedID = id
	}

	override func prepareForReuse() {
		super.prepareForReuse()
		representedID = nil
		// The WHOLE display resets — including the native selection flags,
		// which AppKit only reliably pushes when they change, so a recycled
		// cell could otherwise carry prominence onto an unselected id.
		isCursor = false
		isSelected = false
		highlightState = .none
		model = .empty
	}

	override var isSelected: Bool {
		didSet { updateProminence() }
	}
	override var highlightState: NSCollectionViewItem.HighlightState {
		didSet { updateProminence() }
	}

	private func updateProminence() {
		model.prominence = CellProminence.resolve(
			isCursor: isCursor, isSelected: isSelected, highlightState: highlightState
		)
	}
}

/// Hit-test transparent: every click falls through to the item's root (and
/// on up to the collection view) so selection stays native machinery.
/// (Spike finding, 2026-09-18: NSHostingView otherwise captures its WHOLE
/// frame, even over regions with no interactive content. When a cell
/// control needs clicks, this becomes a registered-rects carve, proven at
/// the same spike; until then, blanket transparency IS the policy.)
private final class CellHostingView<Content: View>: NSHostingView<Content> {
	override func hitTest(_ point: NSPoint) -> NSView? { nil }
}
