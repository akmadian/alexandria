//
//  GridLayoutTests.swift
//  AlexandriaTests
//
//  The resize regression (2026-09-19) — the test that would have caught
//  the flow-layout bug: resizing the window stretched gaps until an extra
//  column popped in, because flow's bounds-change invalidation reflowed
//  cached item sizes without re-asking the size delegate. Under GridLayout
//  a width change re-prepares the geometry, so the column count and gap
//  hold and the cells take the new width. The second test pins the anchor:
//  a deep-scrolled viewport keeps its topmost item through a reflow
//  instead of drifting under a fixed point offset.
//

import AppKit
import Testing
@testable import Alexandria

/// Serialized: each test drains the run loop to settle AppKit's deferred
/// invalidation cycle, and a drain inside one test's body would interleave
/// the other's — two live window fixtures nesting run loops is a harness
/// hazard, not a product configuration.
@Suite(.serialized)
@MainActor
struct GridLayoutTests {

	private final class Items: NSObject, NSCollectionViewDataSource {
		var count = 60
		func collectionView(_ cv: NSCollectionView, numberOfItemsInSection s: Int) -> Int { count }
		func collectionView(_ cv: NSCollectionView, itemForRepresentedObjectAt ip: IndexPath) -> NSCollectionViewItem {
			cv.makeItem(withIdentifier: GridItem.identifier, for: ip)
		}
	}

	private let dataSource = Items()

	private func makeGrid(
		columns: Int, width: CGFloat, dataSource: any NSCollectionViewDataSource
	) -> (NSWindow, NSScrollView, GridCollectionView) {
		let layout = GridLayout()
		layout.columns = columns
		let grid = GridCollectionView(frame: .zero)
		grid.collectionViewLayout = layout
		grid.register(GridItem.self, forItemWithIdentifier: GridItem.identifier)
		grid.dataSource = dataSource
		let scroll = NSScrollView(frame: NSRect(x: 0, y: 0, width: width, height: 400))
		scroll.documentView = grid
		let window = NSWindow(
			contentRect: NSRect(x: 0, y: 0, width: width, height: 400),
			styleMask: [.titled, .resizable], backing: .buffered, defer: false
		)
		window.contentView = scroll
		grid.reloadData()
		settle(window)
		return (window, scroll, grid)
	}

	/// Drain the deferred invalidation → prepare → layout cycle the way the
	/// probe proved it runs (2026-09-19): a bare layoutSubtreeIfNeeded is
	/// not enough after a live bounds change, and a fixed-length drain is
	/// not enough when other main-actor suites interleave inside it — so
	/// drain until the window's layout passes have actually run (the anchor
	/// restore rides the collection view's layout()), with a deadline that
	/// lets the final assertions report a genuine hang.
	private func settle(_ window: NSWindow) {
		let deadline = Date().addingTimeInterval(5)
		repeat {
			window.contentView?.layoutSubtreeIfNeeded()
			RunLoop.current.run(until: Date().addingTimeInterval(0.02))
		} while windowNeedsLayout(window) && Date() < deadline
	}

	private func windowNeedsLayout(_ window: NSWindow) -> Bool {
		func needs(_ view: NSView) -> Bool {
			view.needsLayout || view.subviews.contains(where: needs)
		}
		return window.contentView.map(needs) ?? false
	}

	private func firstRowFrames(_ grid: NSCollectionView, columns: Int) -> [NSRect] {
		(0..<(columns + 2)).compactMap {
			grid.layoutAttributesForItem(at: IndexPath(item: $0, section: 0))?.frame
		}
	}

	@Test func resizeKeepsColumnsAndGapsWhileCellsTakeTheWidth() {
		let (window, _, grid) = makeGrid(columns: 4, width: 400, dataSource: dataSource)

		let before = firstRowFrames(grid, columns: 4)
		let widthBefore = before[0].width
		#expect(before.filter { $0.minY == before[0].minY }.count == 4)

		window.setContentSize(NSSize(width: 640, height: 400))
		settle(window)

		let after = firstRowFrames(grid, columns: 4)
		// Still exactly four columns in the first row — no popped-in fifth.
		#expect(after.filter { $0.minY == after[0].minY }.count == 4)
		// The cells absorbed the width…
		#expect(after[0].width > widthBefore)
		// …and every gap is still exactly the spacing.
		for column in 1..<4 {
			#expect(after[column].minX - after[column - 1].maxX == Theme.Grid.spacing)
		}
	}

	@Test func deepScrolledViewportKeepsItsTopmostItemThroughAReflow() {
		// The production wiring, minus catalog and pixels: the coordinator
		// is data source AND delegate, and its id table answers the
		// anchor's id(at:) reads.
		let coordinator = GridRepresentable.Coordinator()
		let (window, scroll, grid) = makeGrid(columns: 4, width: 400, dataSource: coordinator)
		grid.delegate = coordinator
		coordinator.attach(grid)
		let ids = (0..<60).map { _ in SubjectID.asset(Identifier(rawValue: UUID())) }
		coordinator.apply(workingSet: ids, answering: nil)
		settle(window)

		// Deep enough that rows above the viewport dominate the offset.
		scroll.contentView.scroll(to: NSPoint(x: 0, y: 800))
		scroll.reflectScrolledClipView(scroll.contentView)
		settle(window)

		// The anchor by geometry truth (frames against the visible rect) —
		// the same source the reflow reads. indexPathsForVisibleItems is
		// materialization bookkeeping and lags in a never-displayed window,
		// so it can't carry the assertion.
		let anchorPath = topmostByGeometry(grid)
		let offsetBefore =
			grid.layoutAttributesForItem(at: anchorPath)!.frame.minY
			- grid.visibleRect.minY

		window.setContentSize(NSSize(width: 640, height: 400))
		settle(window)

		// Same topmost item, same offset from the viewport top — the item
		// held still, not the point offset. (Without the anchor, 800pt of
		// shrunken rows put a LATER item at the top here: this offset would
		// read ~480, not ~zero.)
		let offsetAfter =
			grid.layoutAttributesForItem(at: anchorPath)!.frame.minY
			- grid.visibleRect.minY
		#expect(topmostByGeometry(grid) == anchorPath)
		#expect(abs(offsetAfter - offsetBefore) <= 1)
	}

	/// The deep-scroll flicker regression (2026-09-19): the anchor jump must
	/// land BEFORE the pass builds cells, so the same pass materializes the
	/// anchored viewport. When the scroll came after the build, the visible
	/// region was one jump behind — at depth the jump exceeds the viewport,
	/// and the grid showed blank for the whole drag.
	@Test func reflowMaterializesTheAnchoredViewportInTheSamePass() {
		let coordinator = GridRepresentable.Coordinator()
		let (window, scroll, grid) = makeGrid(columns: 5, width: 800, dataSource: coordinator)
		grid.delegate = coordinator
		coordinator.attach(grid)
		let ids = (0..<2600).map { _ in SubjectID.asset(Identifier(rawValue: UUID())) }
		coordinator.apply(workingSet: ids, answering: nil)
		settle(window)

		// Mid-library: hundreds of rows above, so one tick's anchor jump is
		// large — the regression's trigger.
		scroll.contentView.scroll(to: NSPoint(x: 0, y: 40_000))
		scroll.reflectScrolledClipView(scroll.contentView)
		settle(window)

		// Exactly ONE layout pass, no run-loop drain: a drain would let
		// repair passes run, and the OLD (scroll-after-build) order passed
		// this test through exactly that repair (round review, finding 1 —
		// red-verified against the old order 2026-09-19). The origin check
		// proves the tick actually processed in that one pass.
		window.setContentSize(NSSize(width: 830, height: 400))
		window.contentView?.layoutSubtreeIfNeeded()
		#expect(scroll.contentView.bounds.origin.y != 40_000)

		// Every row of the visible rect is covered by a MATERIALIZED item —
		// not just correct geometry, actual live cells.
		let visible = grid.visibleRect
		let spans = grid.visibleItems()
			.map { $0.view.frame.intersection(visible) }
			.filter { !$0.isNull && $0.height > 0 }
			.map { ($0.minY, $0.maxY) }
			.sorted { $0.0 < $1.0 }
		var covered: CGFloat = 0
		var cursor = visible.minY
		for (low, high) in spans where high > max(low, cursor) {
			covered += high - max(low, cursor)
			cursor = high
		}
		// Full cover minus the inter-row gaps (2pt per visible row seam).
		#expect(covered >= visible.height * 0.95)
	}

	/// The zoom path through the production seam (round review, finding 9):
	/// apply(columns:) writes the layout's one column store, and the anchor
	/// rides the geometry-change mint exactly like a width reflow — the
	/// topmost item holds its viewport offset through the re-rowing.
	@Test func zoomThroughTheCoordinatorHoldsColumnsAndPlace() {
		let coordinator = GridRepresentable.Coordinator()
		let (window, scroll, grid) = makeGrid(columns: 4, width: 800, dataSource: coordinator)
		grid.delegate = coordinator
		coordinator.attach(grid)
		let ids = (0..<600).map { _ in SubjectID.asset(Identifier(rawValue: UUID())) }
		coordinator.apply(workingSet: ids, answering: nil)
		settle(window)

		scroll.contentView.scroll(to: NSPoint(x: 0, y: 8_000))
		scroll.reflectScrolledClipView(scroll.contentView)
		settle(window)

		let anchorPath = topmostByGeometry(grid)
		let offsetBefore =
			grid.layoutAttributesForItem(at: anchorPath)!.frame.minY
			- grid.visibleRect.minY

		coordinator.apply(columns: 6)
		settle(window)

		// Six columns in the anchored row…
		let anchorTop = grid.layoutAttributesForItem(at: anchorPath)!.frame.minY
		let rowMates = (0..<600).filter {
			grid.layoutAttributesForItem(at: IndexPath(item: $0, section: 0))!.frame.minY == anchorTop
		}
		#expect(rowMates.count == 6)
		// …and the anchored item kept its offset from the viewport top.
		let offsetAfter = anchorTop - grid.visibleRect.minY
		#expect(abs(offsetAfter - offsetBefore) <= 1)
	}

	/// First item whose frame reaches below the viewport's top edge.
	private func topmostByGeometry(_ grid: NSCollectionView) -> IndexPath {
		let top = grid.visibleRect.minY
		for item in 0..<grid.numberOfItems(inSection: 0) {
			let path = IndexPath(item: item, section: 0)
			if let frame = grid.layoutAttributesForItem(at: path)?.frame, frame.maxY > top {
				return path
			}
		}
		return IndexPath(item: 0, section: 0)
	}
}
