//
//  DragRules.swift
//  Alexandria
//
//  The drag round's ratified combinatorics (2026-09-18): one pure decision
//  table over (payload, target, context). Every surface asks it and none
//  reimplements a cell; unlisted combinations refuse by returning nil, so a
//  future payload or target is safe-by-default until its own round fills
//  its cells. Semantic on both axes — no view type appears here — which is
//  what lets a rebuilt sidebar (OutlineGroup, NSOutlineView) inherit the
//  table unchanged.
//
//  Refusals come in two ranks by ruling: nil = the target never lights up
//  (degrade by not allowing, never a landing no-op), and .refused(message)
//  = the ruled visible refusal (union views, filtered adoption) carried on
//  the drag-image label channel.
//
//  The context is advisory, the verbs are authoritative: hover verdicts
//  ride the drag-start snapshot, and every accept re-checks inside its
//  verb's transaction (the cycle fence lives in moveCollection, not here).
//

import AppKit
import Logging

private nonisolated let dragLog = Logger(label: "drag")

// MARK: - Targets and verdicts

/// Where a drag is hovering, in the design's vocabulary. Adapters translate
/// their geometry into these; the table never learns what a row is made of.
enum DropTarget: Equatable {
	/// `smart` rides the target because the row's builder knows it
	/// synchronously (the tree carries it) and a smart row must be dark
	/// from the drag's FIRST hover — the async membership snapshot's
	/// optimistic window must never light one up.
	case collectionRow(Identifier<Collection>, smart: Bool)
	/// The "Collections" section header — the move-to-root target.
	case collectionsHeader
	/// The gap between two grid cells (reorder's only grid meaning; there
	/// is deliberately no drop-ON-cell in the grid).
	case gridGap
}

enum DropVerdict: Equatable {
	/// Add to a collection. `new` is how many would actually be added
	/// (drives the count badge); nil while the membership snapshot is still
	/// landing — optimistic, per ruling.
	case add(new: Int?)
	/// Re-parent the dragged collection (nil = to the root).
	case reparent(under: Identifier<Collection>?)
	/// Reorder within the viewed collection's manual order.
	case reorderLive
	/// Reorder that first adopts the on-screen order as the manual order —
	/// completes only through the explicit confirmation (judgment is never
	/// silently overwritten, ruled 2026-09-18).
	case reorderAdopt
	/// The ruled visible refusal: no drop, but the user is told why.
	case refused(message: String)
}

// MARK: - The session context

/// Everything a destination may ask about the one live drag. Set by the
/// source at session start, cleared at session end. A per-drag global is
/// honest modeling, not a shortcut: macOS runs exactly one drag at a time,
/// and the drag pasteboard itself is the same kind of global (UIKit's
/// UIDragSession is this object with a platform badge).
final class DragContext {

	private(set) static var current: DragContext?

	let payload: DragPayload.Payload

	/// Collection drags: the dragged collection's parent at drag start
	/// (nil = a root), for the current-parent no-op refusal. Known
	/// synchronously — the source row knows its own place.
	let parentOfDraggedCollection: Identifier<Collection>?

	/// Collection drags: the dragged subtree (root included), the cycle
	/// refusal's hover mirror. Seeded with the dragged id synchronously so
	/// self-drops refuse before the read lands; the full subtree follows.
	private(set) var draggedSubtree: Set<Identifier<Collection>> = []

	/// Asset drags: collection → how many of the dragged assets it already
	/// holds. nil until the snapshot read lands (verdicts stay optimistic).
	private(set) var alreadyHeld: [Identifier<Collection>: Int]?

	/// Grid reorder facts, present only when the drag began over a
	/// collection source in the assets lens.
	struct ReorderFacts {
		var viewedCollection: Identifier<Collection>
		var isManual: Bool
		var filterActive: Bool
		/// Whether any DISPLAYED member comes from a nested collection
		/// (display-faithful union gate, ruled). nil until the read lands.
		var unionContributes: Bool?
		/// Whether the viewed collection is SMART — reorder refuses whole
		/// (no manual order exists to write, and adopt would hit the verb's
		/// fence). nil until the read lands; the query's Source carries only
		/// the id, so smartness arrives with the snapshot like the union
		/// fact.
		var sourceIsSmart: Bool?
	}
	private(set) var reorderFacts: ReorderFacts?

	/// Internal so DragRulesTests can pin table cells against hand-built
	/// contexts; live code enters only through the begin* constructors.
	init(
		payload: DragPayload.Payload,
		parentOfDraggedCollection: Identifier<Collection>? = nil,
		reorderFacts: ReorderFacts? = nil,
		draggedSubtree: Set<Identifier<Collection>> = [],
		alreadyHeld: [Identifier<Collection>: Int]? = nil
	) {
		self.payload = payload
		self.parentOfDraggedCollection = parentOfDraggedCollection
		self.reorderFacts = reorderFacts
		self.draggedSubtree = draggedSubtree
		self.alreadyHeld = alreadyHeld
	}

	// MARK: Lifecycle

	/// Posted by end() so surfaces holding drag-scoped UI state (the
	/// sidebar's targeted highlight) can reset unconditionally — a shim
	/// recycled mid-drag can no longer clear a highlight it inherited
	/// (round review, finding 10).
	static let didEnd = Notification.Name("alexandria.drag.didEnd")

	/// An asset drag (the grid's). Kicks the membership snapshot; hover
	/// verdicts are optimistic until it lands.
	static func beginAssets(
		_ ids: [Identifier<Asset>], catalog: Catalog, reorderFacts: ReorderFacts?
	) {
		let context = DragContext(
			payload: .assets(ids), parentOfDraggedCollection: nil, reorderFacts: reorderFacts
		)
		current = context
		dragLog.debug("drag began", metadata: [
			"payload": "assets",
			"count": "\(ids.count)",
			"reorderable": "\(reorderFacts != nil)",
			"manual": "\(reorderFacts?.isManual ?? false)",
			"filter": "\(reorderFacts?.filterActive ?? false)",
		])
		Task {
			do {
				let held = try await catalog.membershipCounts(of: ids)
				guard Self.current === context else { return }
				context.alreadyHeld = held
			} catch {
				// The snapshot is hover decoration; the verbs stay
				// authoritative. The consequence is user-visible though:
				// every row keeps an optimistic badge-less verdict, and the
				// zero-add refusal never engages this session's drag.
				dragLog.error("membership snapshot failed; verdicts stay optimistic", metadata: [
					"error": "\(error)",
				])
			}
			if let facts = context.reorderFacts {
				do {
					let smart = try await catalog.collectionIsSmart(facts.viewedCollection)
					let union = try await catalog.descendantsContributeMembers(of: facts.viewedCollection)
					guard Self.current === context else { return }
					context.reorderFacts?.sourceIsSmart = smart
					context.reorderFacts?.unionContributes = union
				} catch {
					dragLog.error("reorder-facts snapshot failed (smart/union stay unknown; gap stays dark)", metadata: [
						"error": "\(error)",
					])
				}
			}
		}
	}

	/// A collection drag (a sidebar row's). Kicks the subtree snapshot.
	static func beginCollection(
		_ id: Identifier<Collection>, parent: Identifier<Collection>?, catalog: Catalog
	) {
		let context = DragContext(
			payload: .collection(id), parentOfDraggedCollection: parent, reorderFacts: nil
		)
		context.draggedSubtree = [id]
		current = context
		dragLog.debug("drag began", metadata: [
			"payload": "collection",
			"collection": "\(id.rawValue.uuidString)",
		])
		Task {
			do {
				let subtree = try await catalog.collectionSubtreeIds(of: id)
				guard Self.current === context else { return }
				context.draggedSubtree = subtree
			} catch {
				dragLog.error("subtree snapshot failed", metadata: ["error": "\(error)"])
			}
		}
	}

	static func end() {
		guard current != nil else { return }
		current = nil
		dragLog.trace("drag ended")
		NotificationCenter.default.post(name: didEnd, object: nil)
	}

	// MARK: Reorder posture (derived, per hover)

	/// The grid gap's verdict for an asset payload, from the ruled gates.
	/// Precedence, in order: filtered adoption refuses instantly (no
	/// snapshot needed); the union gate refuses BOTH paths — manual
	/// included, per ruling 6/10, the round-review's finding 1 — with the
	/// ruled visible message; a pending union snapshot keeps the gap dark
	/// for both paths (milliseconds); then manual is live and any other
	/// sort offers the confirmed switch. A non-collection source stays
	/// dark — the grid simply isn't a target.
	fileprivate var reorderVerdict: DropVerdict? {
		guard let facts = reorderFacts else { return nil }
		// KNOWN smart refuses first — it is the real reason regardless of
		// filter state (review finding 2: refusing on the filter first told
		// the user to clear it, only to be refused again). The filter check
		// stays ahead of the PENDING smart window so an ordinary filtered
		// collection keeps its instant, snapshot-free refusal.
		if facts.sourceIsSmart == true {
			return .refused(message: "Can't reorder a smart collection")
		}
		if !facts.isManual, facts.filterActive {
			return .refused(message: "Can't reorder while a filter is active")
		}
		// Pending smartness keeps the gap dark, the union snapshot's stance.
		if facts.sourceIsSmart == nil { return nil }
		switch facts.unionContributes {
		case .some(true):
			return .refused(message: "Can't reorder a view that includes nested collections")
		case .none:
			return nil  // snapshot still landing; the gap stays dark for now
		case .some(false):
			return facts.isManual ? .reorderLive : .reorderAdopt
		}
	}

	// MARK: The drag-image label channel (hints and visible refusals)

	/// The original image components, captured once so the label APPENDS to
	/// the drag image instead of replacing it (the spike's orange-swallows-
	/// purple defect), and restore is exact.
	private var originalComponents: [NSDraggingImageComponent]?
	private var attachedLabel: String?

	/// Attach `text` under the cursor for the rest of this hover. Idempotent
	/// per text; destinations call `restoreLabel` on exit so the channel is
	/// symmetric and a hint never outlives the zone that showed it.
	func attachLabel(_ text: String, to info: NSDraggingInfo, in view: NSView) {
		guard attachedLabel != text else { return }
		attachedLabel = text
		// Once per attach (the guard above), never per hover event: the
		// message otherwise reaches the drag image and nothing else.
		dragLog.debug("drag label attached", metadata: ["text": "\(text)"])
		info.enumerateDraggingItems(
			options: [], for: view, classes: [NSPasteboardItem.self], searchOptions: [:]
		) { [weak self] draggingItem, index, stop in
			guard let self, index == 0 else { stop.pointee = true; return }
			if self.originalComponents == nil {
				self.originalComponents = draggingItem.imageComponents
			}
			let originals = self.originalComponents ?? []
			let height = draggingItem.draggingFrame.height
			draggingItem.imageComponentsProvider = {
				originals + [Self.labelComponent(text, above: height)]
			}
		}
	}

	func restoreLabel(_ info: NSDraggingInfo, in view: NSView) {
		guard attachedLabel != nil else { return }
		attachedLabel = nil
		guard let originals = originalComponents else { return }
		info.enumerateDraggingItems(
			options: [], for: view, classes: [NSPasteboardItem.self], searchOptions: [:]
		) { draggingItem, index, stop in
			guard index == 0 else { stop.pointee = true; return }
			draggingItem.imageComponentsProvider = { originals }
		}
	}

	private static func labelComponent(
		_ text: String, above height: CGFloat
	) -> NSDraggingImageComponent {
		let attributed = NSAttributedString(
			string: " \(text) ",
			attributes: [
				.font: NSFont.systemFont(ofSize: NSFont.smallSystemFontSize, weight: .semibold),
				.foregroundColor: NSColor.white,
				.backgroundColor: NSColor.controlAccentColor,
			]
		)
		let size = attributed.size()
		let image = NSImage(size: size, flipped: false) { rect in
			attributed.draw(in: rect)
			return true
		}
		let component = NSDraggingImageComponent(key: .label)
		component.contents = image
		// Component frames are bottom-left origin: sit the label just above
		// the drag image.
		component.frame = NSRect(x: 0, y: height + 2, width: size.width, height: size.height)
		return component
	}
}

// MARK: - The table

enum DragRules {

	/// The ratified table. nil = the target never lights up. Every cell is
	/// pinned by DragRulesTests; a combination not written here refuses by
	/// construction.
	static func verdict(over target: DropTarget, context: DragContext) -> DropVerdict? {
		switch (context.payload, target) {

		case (.assets(let ids), .collectionRow(let collection, let smart)):
			// Smart takes no manual adds, by ruling (smart-collection
			// round): dark, the same rank as the zero-add refusal. The
			// verb's fence stays the authority.
			guard !smart else { return nil }
			guard let held = context.alreadyHeld else { return .add(new: nil) }
			let new = ids.count - (held[collection] ?? 0)
			// Zero new adds = the ruled dark refusal, never a landing no-op.
			return new > 0 ? .add(new: new) : nil

		case (.assets, .gridGap):
			return context.reorderVerdict

		case (.assets, .collectionsHeader):
			return nil

		case (.collection, .collectionRow(let target, _)):
			// Own subtree (cycle, self included) and the current parent
			// (no-op) stay dark; the verb's transaction re-checks the cycle
			// authoritatively. Smartness is no bar here: re-parenting UNDER
			// a smart collection is tree surgery, and children stay legal
			// on every kind (ruled 2026-09-18).
			guard !context.draggedSubtree.contains(target),
				target != context.parentOfDraggedCollection
			else { return nil }
			return .reparent(under: target)

		case (.collection, .collectionsHeader):
			// Already a root: moving to the root is a no-op, stays dark.
			return context.parentOfDraggedCollection == nil ? nil : .reparent(under: nil)

		case (.collection, .gridGap):
			return nil
		}
	}

	// MARK: The reorder math (pure, pinned)

	/// The one place drop geometry becomes order — shared by the live and
	/// adopt paths so they cannot disagree, extracted pure so tests pin it
	/// (round review, finding 5: this is where a future edit could scramble
	/// a judgment-class ordering).
	struct ReorderPlan: Equatable {
		/// The member the dragged items land BEFORE: the first non-dragged
		/// asset at or after the gap ("before a dragged item" degenerates to
		/// its own successor); nil = append (the verb's contract).
		var anchor: Identifier<Asset>?
		/// The on-screen order with the drop applied — what adoption makes
		/// the manual order, and what the live path's write produces.
		var adopted: [Identifier<Asset>]
	}

	static func reorderPlan(
		ids: [SubjectID], dragged: [Identifier<Asset>], dropIndex: Int
	) -> ReorderPlan {
		let draggedSet = Set(dragged)
		let gap = min(max(dropIndex, 0), ids.count)
		func asset(_ subject: SubjectID) -> Identifier<Asset>? {
			guard case .asset(let id) = subject, !draggedSet.contains(id) else { return nil }
			return id
		}
		let anchor = ids[gap...].lazy.compactMap(asset).first
		var adopted = ids.compactMap(asset)
		let insertAt = anchor.flatMap { adopted.firstIndex(of: $0) } ?? adopted.count
		adopted.insert(contentsOf: dragged, at: insertAt)
		return ReorderPlan(anchor: anchor, adopted: adopted)
	}
}

// MARK: - Verb dispatch (one door for every surface)

/// (payload, verdict) → the standing catalog verb, written once so accept
/// mirrors validate in exactly one place — for every surface, present or
/// future (round review, finding 8: this must not live in a deletable
/// adapter file, and the grid enters through the same door). Adoption is
/// deliberately absent: it writes only through GridView's confirmation.
/// A verb refusal answers audibly — the hover snapshot is advisory, the
/// transaction is the authority.
enum DragVerbs {
	static func perform(
		_ payload: DragPayload.Payload, verdict: DropVerdict, over target: DropTarget,
		reorderingIn reorderCollection: Identifier<Collection>? = nil,
		before anchor: Identifier<Asset>? = nil,
		catalog: Catalog
	) -> Bool {
		switch (payload, verdict, target) {

		case (.assets(let ids), .add, .collectionRow(let collection, _)):
			Task {
				do {
					let added = try await catalog.addMembers(ids, to: collection)
					dragLog.info("drag added members", metadata: [
						"collection": "\(collection.rawValue.uuidString)",
						"dragged": "\(ids.count)", "added": "\(added)",
					])
				} catch {
					NSSound.beep()
					dragLog.error("drag add failed", metadata: ["error": "\(error)"])
				}
			}
			return true

		case (.collection(let dragged), .reparent(let destination), _):
			Task {
				do {
					try await catalog.moveCollection(dragged, under: destination)
					dragLog.info("drag re-parented", metadata: [
						"collection": "\(dragged.rawValue.uuidString)",
						"under": "\(destination.map(\.rawValue.uuidString) ?? "root")",
					])
				} catch CollectionError.wouldCreateCycle {
					// The snapshot mirror missed (a re-parent raced the drag);
					// the verb's fence answers audibly, matching the menu verb.
					NSSound.beep()
					dragLog.info("drag re-parent refused: would create a cycle")
				} catch {
					NSSound.beep()
					dragLog.error("drag re-parent failed", metadata: ["error": "\(error)"])
				}
			}
			return true

		case (.assets(let ids), .reorderLive, .gridGap):
			guard let reorderCollection else { return false }
			Task {
				do {
					let moved = try await catalog.reorderMembers(ids, before: anchor, in: reorderCollection)
					dragLog.info("drag reorder", metadata: [
						"collection": "\(reorderCollection.rawValue.uuidString)",
						"moved": "\(moved)",
					])
				} catch {
					NSSound.beep()
					dragLog.error("drag reorder failed", metadata: ["error": "\(error)"])
				}
			}
			return true

		default:
			return false
		}
	}
}
