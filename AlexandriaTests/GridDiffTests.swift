//
//  GridDiffTests.swift
//  AlexandriaTests
//
//  The grid's delivery arithmetic, pinned without AppKit: batch-op
//  coordinates follow AppKit's contract (deletes and move-origins in OLD
//  positions, inserts and move-destinations in NEW positions), and the
//  change budget routes wholesale replacements to reload instead of a
//  quadratic diff and a thousand flying cells.
//

import AppKit
import Foundation
import Testing
@testable import Alexandria

struct GridDiffTests {

	/// Distinct, stable ids to build orderings from.
	private let a = SubjectID.asset(.mint())
	private let b = SubjectID.asset(.mint())
	private let c = SubjectID.asset(.mint())
	private let d = SubjectID.asset(.mint())

	private func path(_ item: Int) -> IndexPath { IndexPath(item: item, section: 0) }

	@Test func identicalListsYieldEmptyOps() throws {
		let ops = try #require(GridDiff.compute(from: [a, b, c], to: [a, b, c]))
		#expect(ops.isEmpty)
	}

	@Test func appendsArriveAsInsertsAtNewPositions() throws {
		// The import-growth case: the common delivery during churn.
		let ops = try #require(GridDiff.compute(from: [a, b], to: [a, b, c, d]))
		#expect(ops.deletes.isEmpty)
		#expect(ops.moves.isEmpty)
		#expect(ops.inserts == [path(2), path(3)])
	}

	@Test func headInsertShiftsNothingElse() throws {
		// Newest-first arrangement: fresh records land at the top.
		let ops = try #require(GridDiff.compute(from: [b, c], to: [a, b, c]))
		#expect(ops.inserts == [path(0)])
		#expect(ops.deletes.isEmpty)
		#expect(ops.moves.isEmpty)
	}

	@Test func removalsArriveAsDeletesAtOldPositions() throws {
		let ops = try #require(GridDiff.compute(from: [a, b, c, d], to: [a, c]))
		#expect(ops.deletes == [path(1), path(3)])
		#expect(ops.inserts.isEmpty)
		#expect(ops.moves.isEmpty)
	}

	@Test func mixedChangeUsesBothCoordinateSpaces() throws {
		// The classic crash-maker: delete coordinates are OLD, insert NEW.
		let ops = try #require(GridDiff.compute(from: [a, b, c], to: [b, c, d]))
		#expect(ops.deletes == [path(0)])   // a left position 0 of the OLD list
		#expect(ops.inserts == [path(2)])   // d entered position 2 of the NEW list
		#expect(ops.moves.isEmpty)
	}

	@Test func reorderBecomesAMoveWithBothEnds() throws {
		let ops = try #require(GridDiff.compute(from: [a, b, c], to: [b, c, a]))
		#expect(ops.deletes.isEmpty)
		#expect(ops.inserts.isEmpty)
		#expect(ops.moves == [GridDiff.Move(from: path(0), to: path(2))])
	}

	@Test func emptyToPopulatedAndBack() throws {
		let filled = try #require(GridDiff.compute(from: [], to: [a, b]))
		#expect(filled.inserts == [path(0), path(1)])
		let emptied = try #require(GridDiff.compute(from: [a, b], to: []))
		#expect(emptied.deletes == [path(0), path(1)])
	}

	@Test func wholesaleReplacementReturnsNilPastTheBudget() {
		// A latestImport takeover: two big, disjoint answers. The budget
		// check must refuse BEFORE diffing — this is the quadratic case.
		let old = (0..<2_000).map { _ in SubjectID.file(.mint()) }
		let new = (0..<2_000).map { _ in SubjectID.file(.mint()) }
		#expect(GridDiff.compute(from: old, to: new) == nil)
	}

	@Test func largeAppendStaysWithinBudget() throws {
		// Growth is not churn: a big list gaining a batch still diffs.
		let old = (0..<5_000).map { _ in SubjectID.file(.mint()) }
		let new = old + (0..<100).map { _ in SubjectID.file(.mint()) }
		let ops = try #require(GridDiff.compute(from: old, to: new))
		#expect(ops.inserts.count == 100)
		#expect(ops.deletes.isEmpty)
	}
}
