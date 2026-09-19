//
//  SidebarRowShim.swift
//  Alexandria
//
//  The sidebar's drag adapter (drag round, 2026-09-18): a single AppKit
//  NSView overlaid on each collection row, playing both parts —
//
//  - drop TARGET: registered for the drag types, verdicts asked of
//    DragRules, row highlight via callback, count badge, spring-loaded
//    disclosure on hover-hold, verbs dispatched on accept;
//  - drag SOURCE: the threshold dance — swallow mouse-down, a real
//    dragging session past 4pt (synchronous pasteboard write), a clean
//    click selects via callback.
//
//  Why a shim at all: the round's spike proved SwiftUI's drop routing and
//  its item-provider bridge both drop undeclared custom types silently (in
//  BOTH directions), while AppKit's pasteboard path just works. The shim is
//  deliberately the THIN part: payload, rules, and verbs live outside it,
//  so a rebuilt sidebar (OutlineGroup keeps these shims as-is; NSOutlineView
//  deletes them and calls the same seams from its delegate) inherits
//  everything but this file.
//
//  Autoscroll is timer-driven by spike finding: hover callbacks are
//  movement-driven, so content scrolling out from under a STATIONARY cursor
//  goes silent — the driver polls the mouse while the button is down, the
//  same shape NSOutlineView implements internally.
//

import AppKit
import Logging
import SwiftUI

private nonisolated let log = Logger(label: "drag")

// MARK: - Autoscroll

/// One per sidebar (owned by the hosting view, never a singleton): any shim
/// pokes it awake during a drag; it scrolls while the cursor sits in the
/// edge bands and stops itself when the mouse button releases.
final class AutoscrollDriver {
	private var timer: Timer?
	private weak var scrollView: NSScrollView?

	func ensureRunning(for scroll: NSScrollView) {
		scrollView = scroll
		guard timer == nil else { return }
		// .common mode, explicitly: a run loop in event tracking starves
		// .default-mode timers, and this timer exists precisely to run
		// while a drag holds the loop. (The async NSDraggingSession ran a
		// default-mode timer in the round's spike, but .common is the mode
		// the fact actually depends on — round review, finding 2.) The
		// closure captures weakly so driver ⇄ timer never forms a cycle a
		// missed stop() would leak.
		let timer = Timer(timeInterval: 0.05, repeats: true) { [weak self] _ in
			MainActor.assumeIsolated { self?.tick() }
		}
		RunLoop.main.add(timer, forMode: .common)
		self.timer = timer
	}

	private func stop() {
		timer?.invalidate()
		timer = nil
	}

	private func tick() {
		guard NSEvent.pressedMouseButtons & 1 == 1 else { return stop() }
		guard let scroll = scrollView, let window = scroll.window else { return }
		let clip = scroll.contentView
		let point = clip.convert(window.mouseLocationOutsideOfEventStream, from: nil)
		guard point.x >= clip.bounds.minX, point.x <= clip.bounds.maxX,
			let document = scroll.documentView else { return }
		let margin: CGFloat = 44
		let step: CGFloat = 6
		var origin = clip.bounds.origin
		if point.y < clip.bounds.minY + margin, point.y >= clip.bounds.minY - 4 {
			origin.y -= step
		} else if point.y > clip.bounds.maxY - margin, point.y <= clip.bounds.maxY + 4 {
			origin.y += step
		} else {
			return
		}
		let limit = max(0, document.frame.height - clip.bounds.height)
		origin.y = min(max(origin.y, 0), limit)
		guard origin.y != clip.bounds.origin.y else { return }
		clip.scroll(to: origin)
		scroll.reflectScrolledClipView(clip)
	}
}

// MARK: - The row shim

final class SidebarRowShimView: NSView, NSDraggingSource {

	struct Configuration {
		var collection: Identifier<Collection>
		var parent: Identifier<Collection>?
		var title: String
		/// From the tree's row (predicate presence, never decoded): rides
		/// the DropTarget so a smart row is dark from the first hover.
		var isSmart: Bool
		/// Whether hover-hold should spring the row's disclosure open
		/// (rows with children only).
		var canSpring: Bool
		var catalog: Catalog?
		var autoscroll: AutoscrollDriver?
		var onSelect: () -> Void = {}
		var onSpring: () -> Void = {}
		var onTargeted: (Bool) -> Void = { _ in }
	}

	var configuration: Configuration

	init(configuration: Configuration) {
		self.configuration = configuration
		super.init(frame: .zero)
		registerForDraggedTypes([DragPayload.assetsType, DragPayload.collectionType])
	}

	required init?(coder: NSCoder) { fatalError("SidebarRowShimView is code-only") }

	// MARK: Destination

	private var springWork: DispatchWorkItem?

	private var verdict: DropVerdict? {
		guard let context = DragContext.current else { return nil }
		return DragRules.verdict(
			over: .collectionRow(configuration.collection, smart: configuration.isSmart),
			context: context
		)
	}

	override func draggingEntered(_ sender: NSDraggingInfo) -> NSDragOperation {
		if let scroll = enclosingScrollView {
			configuration.autoscroll?.ensureRunning(for: scroll)
		}
		// Spring regardless of this row's own verdict: a refused parent can
		// still shelter an accepting child.
		if configuration.canSpring {
			let work = DispatchWorkItem { [weak self] in self?.configuration.onSpring() }
			springWork?.cancel()
			springWork = work
			DispatchQueue.main.asyncAfter(deadline: .now() + 0.7, execute: work)
		}
		return refreshTargeting(sender)
	}

	override func draggingUpdated(_ sender: NSDraggingInfo) -> NSDragOperation {
		refreshTargeting(sender)
	}

	/// Verdict → operation, highlight, and count badge, re-derived on every
	/// hover event so a snapshot landing mid-hover upgrades the answer.
	private func refreshTargeting(_ sender: NSDraggingInfo) -> NSDragOperation {
		switch verdict {
		case .add(let new):
			configuration.onTargeted(true)
			if let new { sender.numberOfValidItemsForDrop = new }
			return .copy
		case .reparent:
			configuration.onTargeted(true)
			return .move
		case .reorderLive, .reorderAdopt, .refused, nil:
			configuration.onTargeted(false)
			return []
		}
	}

	override func draggingExited(_ sender: NSDraggingInfo?) {
		springWork?.cancel()
		configuration.onTargeted(false)
	}

	override func draggingEnded(_ sender: NSDraggingInfo) {
		springWork?.cancel()
		configuration.onTargeted(false)
	}

	override func performDragOperation(_ sender: NSDraggingInfo) -> Bool {
		guard let catalog = configuration.catalog,
			let payload = DragPayload.read(sender), let verdict else { return false }
		return DragVerbs.perform(
			payload, verdict: verdict,
			over: .collectionRow(configuration.collection, smart: configuration.isSmart),
			catalog: catalog
		)
	}

	/// The SwiftUI context menu lives above this overlay in the responder
	/// chain; forward explicitly rather than leaning on NSView's default
	/// (round review, finding 3 — the row's verbs must survive the shim).
	override func rightMouseDown(with event: NSEvent) {
		nextResponder?.rightMouseDown(with: event)
	}

	// MARK: Source (the threshold dance)

	private var downEvent: NSEvent?

	override func mouseDown(with event: NSEvent) {
		downEvent = event
	}

	override func mouseDragged(with event: NSEvent) {
		guard let down = downEvent else { return }
		let dx = abs(event.locationInWindow.x - down.locationInWindow.x)
		let dy = abs(event.locationInWindow.y - down.locationInWindow.y)
		guard max(dx, dy) > 4 else { return }
		downEvent = nil
		guard let catalog = configuration.catalog else { return }
		DragContext.beginCollection(
			configuration.collection, parent: configuration.parent, catalog: catalog
		)
		let item = NSDraggingItem(
			pasteboardWriter: DragPayload.collectionItem(configuration.collection)
		)
		item.setDraggingFrame(bounds, contents: Self.dragImage(title: configuration.title, size: bounds.size))
		beginDraggingSession(with: [item], event: down, source: self)
	}

	override func mouseUp(with event: NSEvent) {
		guard downEvent != nil else { return }
		downEvent = nil
		configuration.onSelect()
	}

	func draggingSession(
		_ session: NSDraggingSession, sourceOperationMaskFor context: NSDraggingContext
	) -> NSDragOperation {
		// Intra-app by ruling; outside the app a collection means nothing.
		context == .withinApplication ? .move : []
	}

	func draggingSession(
		_ session: NSDraggingSession, endedAt screenPoint: NSPoint, operation: NSDragOperation
	) {
		DragContext.end()
	}

	/// The row's stand-in drag image: name in a rounded pill. (The row's
	/// live pixels aren't cheaply reachable from an overlay; a labeled pill
	/// reads better than a blank snapshot would.)
	private static func dragImage(title: String, size: NSSize) -> NSImage {
		let text = NSAttributedString(
			string: title,
			attributes: [
				.font: NSFont.systemFont(ofSize: NSFont.systemFontSize),
				.foregroundColor: NSColor.labelColor,
			]
		)
		let padded = NSSize(
			width: max(size.width, text.size().width + 24), height: max(size.height, 22)
		)
		return NSImage(size: padded, flipped: false) { rect in
			NSColor.windowBackgroundColor.withAlphaComponent(0.9).setFill()
			NSBezierPath(roundedRect: rect, xRadius: 5, yRadius: 5).fill()
			text.draw(at: NSPoint(x: 12, y: (rect.height - text.size().height) / 2))
			return true
		}
	}
}

// MARK: - The header target (move to root)

final class CollectionsHeaderShimView: NSView {
	var catalog: Catalog?
	var onTargeted: (Bool) -> Void = { _ in }

	override init(frame: NSRect) {
		super.init(frame: frame)
		registerForDraggedTypes([DragPayload.assetsType, DragPayload.collectionType])
	}
	required init?(coder: NSCoder) { fatalError("CollectionsHeaderShimView is code-only") }

	/// Click-transparent: this shim has no mouse role, and it overlays the
	/// header's "+" button (round review, finding 3). Drag destination
	/// discovery walks registered views, not hitTest, so the drop role
	/// survives — verified by hand-test.
	override func hitTest(_ point: NSPoint) -> NSView? { nil }

	private var verdict: DropVerdict? {
		guard let context = DragContext.current else { return nil }
		return DragRules.verdict(over: .collectionsHeader, context: context)
	}

	override func draggingEntered(_ sender: NSDraggingInfo) -> NSDragOperation {
		guard case .reparent = verdict else { return [] }
		onTargeted(true)
		return .move
	}
	override func draggingExited(_ sender: NSDraggingInfo?) { onTargeted(false) }
	override func draggingEnded(_ sender: NSDraggingInfo) { onTargeted(false) }

	override func performDragOperation(_ sender: NSDraggingInfo) -> Bool {
		guard let catalog, let payload = DragPayload.read(sender), let verdict
		else { return false }
		return DragVerbs.perform(payload, verdict: verdict, over: .collectionsHeader, catalog: catalog)
	}
}

// MARK: - SwiftUI faces

struct SidebarRowShim: NSViewRepresentable {
	var configuration: SidebarRowShimView.Configuration

	func makeNSView(context: Context) -> SidebarRowShimView {
		SidebarRowShimView(configuration: configuration)
	}
	func updateNSView(_ view: SidebarRowShimView, context: Context) {
		view.configuration = configuration
	}
}

struct CollectionsHeaderShim: NSViewRepresentable {
	var catalog: Catalog
	var onTargeted: (Bool) -> Void

	func makeNSView(context: Context) -> CollectionsHeaderShimView {
		let view = CollectionsHeaderShimView()
		view.catalog = catalog
		view.onTargeted = onTargeted
		return view
	}
	func updateNSView(_ view: CollectionsHeaderShimView, context: Context) {
		view.catalog = catalog
		view.onTargeted = onTargeted
	}
}
