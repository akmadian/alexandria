//
//  GridLayout.swift
//  Alexandria
//
//  The grid's layout (resize fix, 2026-09-19): a thin adapter over
//  GridGeometry with the column count as the one input. It replaces
//  NSCollectionViewFlowLayout, whose model is the inversion (item size in,
//  column count out) — flow re-flowed stale cached sizes on a width change
//  (its bounds-change invalidation never re-asks the size delegate;
//  measured by probe 2026-09-19), stretching gaps until an extra column
//  popped in. Here a width invalidation just re-prepares, and re-prepare
//  IS the recompute — there is no delegate round trip to go stale.
//
//  Attributes are O(1) arithmetic per item with no per-item cache (flow's
//  prepare is O(n) over the whole set) — the 1M-scale posture grid.md
//  already commits to.
//

import AppKit

final class GridLayout: NSCollectionViewLayout {

	/// The one input. The coordinator's `apply(columns:)` writes it on a
	/// zoom; a width change re-prepares against the same count.
	var columns = CatalogViewState.gridColumnRange.lowerBound {
		didSet {
			guard columns != oldValue else { return }
			invalidateLayout()
		}
	}

	private var geometry: GridGeometry?
	private var itemCount = 0

	/// The scroll origin that keeps the pre-reflow topmost item in place,
	/// minted by prepare() when ANY geometry change swaps the frames
	/// (width, column count, or backing scale — one anchor for one
	/// concept), applied (and cleared) by GridCollectionView.layout()
	/// BEFORE super runs — so the pass materializes cells for the anchored
	/// viewport, never one jump behind it. A point offset doesn't survive
	/// a reflow — every row above the viewport changes height — so the
	/// ITEM holds still instead. Anchoring by item index is deliberate: a
	/// pure reflow moves no items (unlike a delivery, whose id-keyed
	/// anchor lives in the coordinator). It lives HERE, not on a callback
	/// from shouldInvalidateLayout: any stray layout pass can consume the
	/// new width before that comparison runs (measured 2026-09-19, the
	/// full-suite anchor test), but the first prepare against the new
	/// geometry always still holds the old one.
	private var pendingReflowOrigin: CGFloat?

	/// The re-bucket signal, deliberately separate from the scroll origin:
	/// a geometry change with no anchor to place (an empty grid) must
	/// still never strand cells on a stale decode tier (round review,
	/// finding 7).
	private var geometryChanged = false

	/// The clip view's width — the one width that doesn't derive from this
	/// layout's own contentSize answer (the collection view sizes its frame
	/// FROM the content size, so reading it back would be circular).
	private var viewportWidth: CGFloat {
		collectionView?.superview?.bounds.width ?? 0
	}

	override func prepare() {
		let outgoing = geometry
		let outgoingCount = itemCount
		geometry = GridGeometry(
			columns: columns, width: viewportWidth,
			scale: collectionView?.window?.backingScaleFactor ?? 2
		)
		itemCount =
			(collectionView?.numberOfSections ?? 0) > 0
			? collectionView?.numberOfItems(inSection: 0) ?? 0 : 0
		if let outgoing, let incoming = geometry, outgoing != incoming {
			geometryChanged = true
			pendingReflowOrigin = reflowOrigin(
				from: outgoing, outgoingCount: outgoingCount, to: incoming)
		}
	}

	/// Where the viewport must scroll so the topmost visible item keeps its
	/// offset from the viewport top across the geometry swap. Nil when
	/// there's nothing to hold (empty grid, or nothing above the viewport
	/// worth compensating).
	private func reflowOrigin(
		from outgoing: GridGeometry, outgoingCount: Int, to incoming: GridGeometry
	) -> CGFloat? {
		guard let collectionView, outgoingCount > 0, itemCount > 0 else { return nil }
		let visible = collectionView.visibleRect
		// An unconsumed origin IS the viewport top in the outgoing
		// geometry's coordinates: its scroll hasn't landed, so the live
		// rect still reads the geometry BEFORE that one, and anchoring
		// from it picks the wrong item — two prepares before one layout
		// pass then drift the viewport permanently (round review,
		// finding 2).
		let top = pendingReflowOrigin ?? visible.minY
		let window = CGRect(x: visible.minX, y: top, width: visible.width, height: visible.height)
		let range = outgoing.items(in: window, itemCount: outgoingCount)
		guard let topmost = range.first else { return nil }
		let offset = outgoing.frame(ofItem: topmost).minY - top
		let anchored = min(topmost, itemCount - 1)
		return max(0, incoming.frame(ofItem: anchored).minY - offset)
	}

	/// Consumed by GridCollectionView.layout() — read once, then cleared.
	func takePendingReflowOrigin() -> CGFloat? {
		defer { pendingReflowOrigin = nil }
		return pendingReflowOrigin
	}

	/// Consumed by GridCollectionView.layout() after every pass; true when
	/// prepare() swapped the geometry since the last take.
	func takeGeometryChanged() -> Bool {
		defer { geometryChanged = false }
		return geometryChanged
	}

	/// A question replace makes an unconsumed origin meaningless — it
	/// anchored content that no longer exists, and applying it would open
	/// the new answer at an arbitrary depth (round review, finding 4).
	func cancelPendingReflow() {
		pendingReflowOrigin = nil
	}

	/// The cell side the CURRENT viewport implies, computed fresh from the
	/// live clip width — never racing prepare timing (master: GridGeometry;
	/// the decode-bucket choice keys on this). Zero when detached from a
	/// scroll view: the smallest rung, harmless for a grid with no cells
	/// on screen.
	var liveCellSide: CGFloat {
		guard collectionView != nil else { return 0 }
		return GridGeometry(
			columns: columns, width: viewportWidth,
			scale: collectionView?.window?.backingScaleFactor ?? 2
		).cellSide
	}

	override var collectionViewContentSize: NSSize {
		guard let geometry else { return .zero }
		return NSSize(width: geometry.width, height: geometry.contentHeight(itemCount: itemCount))
	}

	override func layoutAttributesForElements(in rect: NSRect) -> [NSCollectionViewLayoutAttributes] {
		guard let geometry else { return [] }
		return geometry.items(in: rect, itemCount: itemCount).map { item in
			let attributes = NSCollectionViewLayoutAttributes(
				forItemWith: IndexPath(item: item, section: 0))
			attributes.frame = geometry.frame(ofItem: item)
			return attributes
		}
	}

	override func layoutAttributesForItem(at indexPath: IndexPath) -> NSCollectionViewLayoutAttributes? {
		guard let geometry, indexPath.item >= 0 else { return nil }
		let attributes = NSCollectionViewLayoutAttributes(forItemWith: indexPath)
		attributes.frame = geometry.frame(ofItem: indexPath.item)
		return attributes
	}

	override func shouldInvalidateLayout(forBoundsChange newBounds: NSRect) -> Bool {
		guard let geometry else { return false }
		return newBounds.width != geometry.width
	}

	override func layoutAttributesForInterItemGap(before indexPath: IndexPath) -> NSCollectionViewLayoutAttributes? {
		guard let geometry else { return nil }
		let attributes = NSCollectionViewLayoutAttributes(forInterItemGapBefore: indexPath)
		attributes.frame = geometry.gapFrame(before: indexPath.item, itemCount: itemCount)
		return attributes
	}
}
