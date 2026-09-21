//
//  GridGeometry.swift
//  Alexandria
//
//  The pure half of the grid's layout (resize fix, 2026-09-19): every frame
//  is arithmetic on (viewport width, column count, spacing, inset, backing
//  scale) — same discipline as GridDiff and GridImaging, no AppKit in the
//  loop. Column count is the INPUT: the flow layout this replaced derived
//  count from item size, and a width change reflowed stale cached sizes —
//  gaps stretched until an extra column popped in (the 2026-09-19 probe:
//  flow's bounds-change invalidation never re-asks the size delegate).
//
//  All internal math is in device pixels: cell edges land on the pixel
//  grid, the division remainder goes into cell widths (the first `extra`
//  columns are one pixel wider), and gaps are EXACTLY the spacing —
//  flow justified its rows by stretching gaps instead, so even its correct
//  state missed the spec.
//

// CoreGraphics only for CGRect/CGFloat — no view types; the logic stays
// pure and main-actor-free.
internal import CoreGraphics

nonisolated struct GridGeometry: Equatable {
	let columns: Int
	/// The viewport width this geometry answers for, in points.
	let width: CGFloat
	let scale: CGFloat

	// Device pixels throughout.
	private let insetPx: Int
	private let spacingPx: Int
	/// The narrow column's width; the first `extra` columns get one more px.
	private let basePx: Int
	private let extra: Int
	/// One height for every cell (nominally square; the remainder pixels
	/// live only in widths so rows stay aligned and row math stays O(1)).
	private let heightPx: Int

	init(
		columns: Int, width: CGFloat, scale rawScale: CGFloat,
		spacing: CGFloat = Theme.Grid.interCellSpacing, inset: CGFloat = Theme.Grid.inset
	) {
		let scale = max(rawScale, 1)
		self.columns = max(columns, 1)
		self.width = width
		self.scale = scale
		insetPx = Int((inset * scale).rounded())
		spacingPx = Int((spacing * scale).rounded())
		let widthPx = Int((width * scale).rounded())
		let available = widthPx - 2 * insetPx - (self.columns - 1) * spacingPx
		// A degenerate viewport (narrower than one px per column) clamps to
		// 1px cells rather than minting empty or negative frames.
		basePx = max(1, available / self.columns)
		extra = available >= self.columns ? available % self.columns : 0
		heightPx = basePx + (2 * extra >= self.columns ? 1 : 0)
	}

	/// The nominal cell side in points — what the decode-bucket choice keys on.
	var cellSide: CGFloat { CGFloat(heightPx) / scale }

	private var rowPitchPx: Int { heightPx + spacingPx }

	func frame(ofItem index: Int) -> CGRect {
		let row = index / columns
		let column = index % columns
		let x = insetPx + column * (basePx + spacingPx) + min(column, extra)
		let y = insetPx + row * rowPitchPx
		let cellWidth = basePx + (column < extra ? 1 : 0)
		return CGRect(
			x: CGFloat(x) / scale, y: CGFloat(y) / scale,
			width: CGFloat(cellWidth) / scale, height: CGFloat(heightPx) / scale
		)
	}

	func contentHeight(itemCount: Int) -> CGFloat {
		guard itemCount > 0 else { return CGFloat(2 * insetPx) / scale }
		let rows = (itemCount + columns - 1) / columns
		return CGFloat(2 * insetPx + rows * heightPx + (rows - 1) * spacingPx) / scale
	}

	/// The items whose frames can intersect `rect` — a row-range read, so a
	/// viewport query costs the visible rows, never the item count.
	/// Deliberately over-inclusive: rows clamp to the content's first/last
	/// row, so a rect past either end answers the nearest row rather than
	/// nothing (overscroll asks for real attributes), and a rect edge
	/// exactly on a row seam includes that row. Never empty for a
	/// non-empty grid, except a degenerate inverted rect.
	func items(in rect: CGRect, itemCount: Int) -> Range<Int> {
		guard itemCount > 0 else { return 0..<0 }
		let totalRows = (itemCount + columns - 1) / columns
		let lastIndex = CGFloat(totalRows - 1)
		let minYPx = rect.minY * scale - CGFloat(insetPx)
		let maxYPx = rect.maxY * scale - CGFloat(insetPx)
		let pitch = CGFloat(rowPitchPx)
		// Clamp in CGFloat BEFORE converting: an infinite or null rect must
		// answer the clamped range, never trap the Int conversion.
		let firstRow = Int(min(lastIndex, max(0, (minYPx / pitch).rounded(.down))))
		let lastRow = Int(min(lastIndex, max(0, (maxYPx / pitch).rounded(.down))))
		guard lastRow >= firstRow else { return 0..<0 }
		return (firstRow * columns)..<min(itemCount, (lastRow + 1) * columns)
	}

	/// The drop-indicator slot before `index` (== `itemCount` means after the
	/// last item): the spacing-wide strip beside the item, full cell height —
	/// what the reorder drag draws its insertion line in. The base
	/// NSCollectionViewLayout returns nothing for inter-item gaps; supplying
	/// this is what keeps the drag round's one grid affordance alive.
	func gapFrame(before index: Int, itemCount: Int) -> CGRect {
		let spacingPoints = CGFloat(spacingPx) / scale
		guard itemCount > 0 else {
			let origin = CGFloat(insetPx) / scale
			return CGRect(x: origin, y: origin, width: spacingPoints, height: CGFloat(heightPx) / scale)
		}
		if index >= itemCount {
			let anchor = frame(ofItem: itemCount - 1)
			return CGRect(x: anchor.maxX, y: anchor.minY, width: spacingPoints, height: anchor.height)
		}
		let anchor = frame(ofItem: max(0, index))
		return CGRect(
			x: anchor.minX - spacingPoints, y: anchor.minY,
			width: spacingPoints, height: anchor.height
		)
	}
}
