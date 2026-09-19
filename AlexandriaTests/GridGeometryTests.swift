//
//  GridGeometryTests.swift
//  AlexandriaTests
//
//  The grid's frame math, pinned pure (resize fix, 2026-09-19): column
//  count constant at every width, gaps exactly the spacing, edges on the
//  device-pixel grid, the division remainder absorbed by cell widths. The
//  flow layout this replaced failed the first two by construction — count
//  drifted on resize (stale cached sizes) and gaps carried the remainder.
//

import CoreGraphics
import Testing
@testable import Alexandria

struct GridGeometryTests {

	private func geometry(
		columns: Int = 5, width: CGFloat, scale: CGFloat = 2
	) -> GridGeometry {
		GridGeometry(columns: columns, width: width, scale: scale, spacing: 2, inset: 2)
	}

	@Test(arguments: [320.0, 400.0, 401.5, 517.0, 640.0, 799.0, 1200.0])
	func columnCountHoldsAtEveryWidth(width: CGFloat) {
		let geometry = geometry(width: width)
		// The first row is items 0..<5: same top edge; item 5 starts row two.
		let topEdges = (0..<5).map { geometry.frame(ofItem: $0).minY }
		#expect(Set(topEdges).count == 1)
		#expect(geometry.frame(ofItem: 5).minY > geometry.frame(ofItem: 0).minY)
	}

	@Test(arguments: [320.0, 400.0, 401.5, 517.0, 640.0, 799.0, 1200.0])
	func gapsAreExactlyTheSpacing(width: CGFloat) {
		let geometry = geometry(width: width)
		for column in 1..<5 {
			let gap = geometry.frame(ofItem: column).minX - geometry.frame(ofItem: column - 1).maxX
			#expect(gap == 2)
		}
	}

	@Test(arguments: [1.0, 2.0])
	func edgesLandOnTheDevicePixelGrid(scale: CGFloat) {
		let geometry = geometry(width: 517, scale: scale)
		for item in 0..<10 {
			let frame = geometry.frame(ofItem: item)
			for edge in [frame.minX, frame.maxX, frame.minY, frame.maxY] {
				#expect((edge * scale).truncatingRemainder(dividingBy: 1) == 0)
			}
		}
	}

	@Test func remainderGoesIntoCellWidthsNotGaps() {
		// 518pt at 2×: 1036px − 8 inset − 16 spacing = 1012px over 5 → 202 r2:
		// two cells one pixel wider, and the row spans inset edge to inset edge.
		let geometry = geometry(width: 518)
		let widths = (0..<5).map { geometry.frame(ofItem: $0).width * 2 }
		#expect(widths.max()! - widths.min()! == 1)
		#expect(widths.reduce(0, +) == 1012)
		#expect(geometry.frame(ofItem: 0).minX == 2)
		#expect(geometry.frame(ofItem: 4).maxX == 516)
	}

	@Test func contentHeightCountsRows() {
		let geometry = geometry(width: 400)
		let side = geometry.cellSide
		#expect(geometry.contentHeight(itemCount: 0) == 4)
		#expect(geometry.contentHeight(itemCount: 5) == 4 + side)
		#expect(geometry.contentHeight(itemCount: 6) == 4 + 2 * side + 2)
	}

	@Test func itemsInRectIsTheVisibleRowWindow() {
		let geometry = geometry(width: 400)
		let pitch = geometry.cellSide + 2
		let all = CGRect(x: 0, y: 0, width: 400, height: 10_000)
		#expect(geometry.items(in: all, itemCount: 23) == 0..<23)
		#expect(geometry.items(in: all, itemCount: 0).isEmpty)
		// A viewport over rows 2 and 3 only.
		let window = CGRect(x: 0, y: 2 + 2 * pitch, width: 400, height: pitch)
		let range = geometry.items(in: window, itemCount: 100)
		#expect(range.lowerBound == 10)
		#expect(range.upperBound >= 20)
		// A rect past the content clamps to the last row (overscroll asks
		// for real attributes, never out-of-range items).
		let below = CGRect(x: 0, y: 2 + 5 * pitch, width: 400, height: 100)
		#expect(geometry.items(in: below, itemCount: 10) == 5..<10)
	}

	@Test func gapFramesSitBesideTheirItems() {
		let geometry = geometry(width: 400)
		// Mid-row: fills the spacing between neighbors exactly.
		let before2 = geometry.gapFrame(before: 2, itemCount: 23)
		#expect(before2.minX == geometry.frame(ofItem: 1).maxX)
		#expect(before2.maxX == geometry.frame(ofItem: 2).minX)
		#expect(before2.minY == geometry.frame(ofItem: 2).minY)
		// Row start: hugs the item's left edge on the right row.
		let rowStart = geometry.gapFrame(before: 5, itemCount: 23)
		#expect(rowStart.maxX == geometry.frame(ofItem: 5).minX)
		#expect(rowStart.minY == geometry.frame(ofItem: 5).minY)
		// After the last item.
		let end = geometry.gapFrame(before: 23, itemCount: 23)
		#expect(end.minX == geometry.frame(ofItem: 22).maxX)
		// Empty grid: defined, not a crash.
		#expect(geometry.gapFrame(before: 0, itemCount: 0).width == 2)
	}

	@Test func degenerateWidthsStayDefined() {
		for width: CGFloat in [0, 3, 10] {
			let geometry = geometry(width: width)
			let frame = geometry.frame(ofItem: 4)
			#expect(frame.width > 0)
			#expect(geometry.contentHeight(itemCount: 10) > 0)
		}
	}
}
