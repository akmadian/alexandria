//
//  GridCellTests.swift
//  AlexandriaTests
//
//  The cell's four-state resolution, pinned without a window: cursor
//  outranks selection, AppKit's transient highlight states read as
//  selected, and nothing else lights the ring.
//

import AppKit
import Testing
@testable import Alexandria

struct GridCellTests {

	@Test func idleWhenNothingApplies() {
		#expect(
			CellProminence.resolve(isCursor: false, isSelected: false, highlightState: .none)
				== .idle
		)
	}

	@Test func selectionReadsSelected() {
		#expect(
			CellProminence.resolve(isCursor: false, isSelected: true, highlightState: .none)
				== .selected
		)
	}

	@Test func transientHighlightsReadSelected() {
		#expect(
			CellProminence.resolve(isCursor: false, isSelected: false, highlightState: .forSelection)
				== .selected
		)
		#expect(
			CellProminence.resolve(isCursor: false, isSelected: false, highlightState: .asDropTarget)
				== .selected
		)
	}

	@Test func deselectionHighlightStaysIdle() {
		#expect(
			CellProminence.resolve(
				isCursor: false, isSelected: false, highlightState: .forDeselection
			) == .idle
		)
	}

	/// The reuse-reset contract: recycling wipes the WHOLE doorway — records,
	/// position, prominence — and the native selection flags, so no state
	/// from id A can decorate id B.
	@Test @MainActor func prepareForReuseResetsTheWholeDoorway() {
		let item = GridItem()
		_ = item.view  // loadView: hosting + chrome mounted, off-window
		item.represent(.asset(.mint()))
		item.display(position: 41)
		item.display(
			asset: Asset(id: .mint(), kind: .image, rating: 3, flag: .pick, representativeFileId: nil),
			file: nil
		)
		item.isSelected = true
		item.isCursor = true
		#expect(item.state.prominence == .cursor)

		item.prepareForReuse()
		#expect(item.representedID == nil)
		#expect(item.state.position == nil)
		#expect(item.state.asset == nil)
		#expect(item.state.file == nil)
		#expect(item.state.prominence == .idle)
		#expect(item.isSelected == false)
		#expect(item.isCursor == false)
	}

	@Test func cursorOutranksSelection() {
		#expect(
			CellProminence.resolve(isCursor: true, isSelected: true, highlightState: .none)
				== .cursor
		)
		#expect(
			CellProminence.resolve(isCursor: true, isSelected: false, highlightState: .none)
				== .cursor
		)
	}
}
