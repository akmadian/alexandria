//
//  StarRatingTests.swift
//  AlexandriaTests
//
//  The stepping rule (keybind round, 2026-09-20): relative edits clamp to
//  1...5 and unrate below 1. The rule is a static so it pins without a view;
//  the control's arrow keys and accessibility increment both go through it.
//

import Testing
@testable import Alexandria

struct StarRatingTests {

	@Test func steppingClampsAndUnrates() {
		// Up from unrated lands on 1; up from the top stays at the top.
		#expect(StarRating.stepped(from: 0, by: 1) == 1)
		#expect(StarRating.stepped(from: 3, by: 1) == 4)
		#expect(StarRating.stepped(from: 5, by: 1) == 5)
		// Down from 1 unrates; down from unrated stays unrated.
		#expect(StarRating.stepped(from: 1, by: -1) == nil)
		#expect(StarRating.stepped(from: 0, by: -1) == nil)
		#expect(StarRating.stepped(from: 4, by: -1) == 3)
	}

	/// The gate itself (ruled 2026-09-20): with stepping off, an arrow is
	/// ignored — it falls through to the chain, never writes.
	@Test func arrowsAreGatedByStepping() {
		#expect(StarRating.arrowOutcome(from: 3, by: 1, stepping: true) == .set(4))
		#expect(StarRating.arrowOutcome(from: 3, by: 1, stepping: false) == .ignored)
		#expect(StarRating.arrowOutcome(from: 1, by: -1, stepping: true) == .set(nil))
	}

	/// Digits are absolute and never gated: 0 unrates, out-of-range and
	/// non-digits fall through.
	@Test func digitsAreAbsolute() {
		#expect(StarRating.digitOutcome("4") == .set(4))
		#expect(StarRating.digitOutcome("0") == .set(nil))
		#expect(StarRating.digitOutcome("7") == .ignored)
		#expect(StarRating.digitOutcome("p") == .ignored)
		#expect(StarRating.digitOutcome(nil) == .ignored)
	}
}
