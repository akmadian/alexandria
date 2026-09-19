//
//  DragPayload.swift
//  Alexandria
//
//  The drag round's payload seam (2026-09-18): what travels on the drag
//  pasteboard, typed and construction-agnostic — every surface, present or
//  future, reads drags through this one vocabulary and never through view
//  types. Plain NSPasteboard types by ruling: the round's spike proved
//  SwiftUI's item-provider bridge drops undeclared custom types silently
//  (both directions — the struck chunk-4 build's true killer), while AppKit
//  pasteboard strings need no declaration anywhere.
//
//  The payload unit is ASSETS (ruled: membership's unit is assets, and both
//  standing verbs take asset ids) — a files-lens drag maps to owning assets
//  at the source, so destinations never learn about lenses. Collection rows
//  drag as the second type; the two never mix in one session.
//

import AppKit

enum DragPayload {

	/// The two intra-app pasteboard types. Plain strings — never UTTypes,
	/// never declared: the whole design is AppKit-source → AppKit-reader.
	static let assetsType = NSPasteboard.PasteboardType("com.alexandria.drag.assets")
	static let collectionType = NSPasteboard.PasteboardType("com.alexandria.drag.collection")

	/// What one drag session carries, normalized on read
	/// (classification-then-dispatch, the NetNewsWire shape).
	enum Payload: Equatable {
		/// Ordinal-sorted at read: the dragged cells in working-set order.
		case assets([Identifier<Asset>])
		case collection(Identifier<Collection>)
	}

	// MARK: Writing (one pasteboard item per dragged thing)

	/// One item per dragged cell — multi-item drag imagery requires one
	/// pasteboard item per dragging item (AppKit contract). The ordinal
	/// pins working-set order: AppKit does not document the order pasteboard
	/// items come back in for a multi-item drag.
	static func assetItem(_ id: Identifier<Asset>, ordinal: Int) -> NSPasteboardItem {
		let item = NSPasteboardItem()
		item.setPropertyList(
			["id": id.rawValue.uuidString, "ordinal": ordinal], forType: assetsType
		)
		return item
	}

	static func collectionItem(_ id: Identifier<Collection>) -> NSPasteboardItem {
		let item = NSPasteboardItem()
		item.setString(id.rawValue.uuidString, forType: collectionType)
		return item
	}

	// MARK: Reading (any NSDraggingInfo, any surface)

	static func read(_ info: NSDraggingInfo) -> Payload? {
		read(info.draggingPasteboard)
	}

	/// Pasteboard-level read, split out so tests exercise the round trip
	/// without synthesizing a dragging session.
	static func read(_ pasteboard: NSPasteboard) -> Payload? {
		guard let items = pasteboard.pasteboardItems, !items.isEmpty else { return nil }
		let assets = items
			.compactMap { item -> (id: Identifier<Asset>, ordinal: Int)? in
				guard let plist = item.propertyList(forType: assetsType) as? [String: Any],
					let text = plist["id"] as? String,
					let uuid = UUID(uuidString: text),
					let ordinal = plist["ordinal"] as? Int
				else { return nil }
				return (Identifier<Asset>(rawValue: uuid), ordinal)
			}
		if !assets.isEmpty {
			// Deduplicated, first occurrence keeping its place (the
			// reorderMembers rule): items are written per CELL, and two
			// file cells of one asset are ONE dragged asset — the badge
			// and zero-add arithmetic count assets, never cells.
			var seen: Set<Identifier<Asset>> = []
			return .assets(
				assets.sorted { $0.ordinal < $1.ordinal }
					.map(\.id)
					.filter { seen.insert($0).inserted }
			)
		}
		if let text = items.first?.string(forType: collectionType),
			let uuid = UUID(uuidString: text) {
			return .collection(Identifier<Collection>(rawValue: uuid))
		}
		return nil
	}
}
