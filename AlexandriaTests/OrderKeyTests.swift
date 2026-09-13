//
//  OrderKeyTests.swift
//  AlexandriaTests
//
//  The manual-order key math, pinned (collections round, 2026-09-12;
//  vectors re-pinned for the full reference algorithm after round
//  review): ordering properties under every insertion pattern — the two
//  shipped failures the design rejected (LrC's float mantissa exhaustion
//  and renumber storms) AND the reviewed one (append growing keys
//  linearly) — plus the load-bearing claim that Swift's String order and
//  SQLite's BINARY collation agree on the keys.
//

import Foundation
import GRDB
import Testing
@testable import Alexandria

/// Deterministic RNG (SplitMix64) so the randomized ordering tests are
/// fixed vectors, not flakes.
private struct SeededGenerator: RandomNumberGenerator {
	var state: UInt64
	mutating func next() -> UInt64 {
		state &+= 0x9E3779B97F4A7C15
		var z = state
		z = (z ^ (z >> 30)) &* 0xBF58476D1CE4E5B9
		z = (z ^ (z >> 27)) &* 0x94D049BB133111EB
		return z ^ (z >> 31)
	}
}

struct OrderKeyTests {

	private func assertCanonical(_ keys: [String]) {
		#expect(keys == keys.sorted())
		#expect(Set(keys).count == keys.count)
		#expect(keys.allSatisfy(OrderKey.isWellFormed))
	}

	@Test func referenceVectors() {
		// Concrete mints, pinned so the algorithm can't drift between
		// builds (stored keys outlive the code that minted them).
		#expect(OrderKey.between(nil, nil) == "a0")       // first key ever
		#expect(OrderKey.between("a0", nil) == "a1")      // append = increment
		#expect(OrderKey.between("a9", nil) == "aA")      // digit order: 0-9 < A-Z < a-z
		#expect(OrderKey.between("az", nil) == "b00")     // level carry
		#expect(OrderKey.between(nil, "a0") == "Zz")      // prepend steps down
		#expect(OrderKey.between("a0", "a1") == "a0V")    // adjacent integers: fraction opens
		#expect(OrderKey.between("a0", "a0V") == "a0G")   // fraction midpoint
		#expect(OrderKey.between("Zz", "a0") == "ZzV")    // across the sign boundary
		#expect(OrderKey.between("a0V", "a1") == "a0l")   // fraction toward the top
		#expect(OrderKey.between("a1", "b00") == "a2")    // integer step wins when it fits
	}

	@Test func wellFormednessRejectsTheInvalidShapes() {
		#expect(OrderKey.isWellFormed("a0"))
		#expect(OrderKey.isWellFormed("Zz"))
		#expect(OrderKey.isWellFormed("b00"))
		#expect(OrderKey.isWellFormed("a0V"))
		#expect(!OrderKey.isWellFormed(""))                // no head
		#expect(!OrderKey.isWellFormed("0"))               // head must be a letter
		#expect(!OrderKey.isWellFormed("b0"))              // integer part short of its declared length
		#expect(!OrderKey.isWellFormed("a0V0"))            // fraction with trailing minimum digit
		#expect(!OrderKey.isWellFormed("a0!"))             // character outside the alphabet
		#expect(!OrderKey.isWellFormed("A" + String(repeating: "0", count: 26)))  // reserved smallest integer
	}

	/// The review's finding, pinned: append is an integer increment, so
	/// 10,000 appends stay at FOUR characters — not the 2,000-character
	/// keys the midpoint-only scheme minted. Prepend is symmetric.
	@Test func appendAndPrependStayLogarithmicallyShort() {
		var keys = [OrderKey.between(nil, nil)]
		for _ in 0..<10_000 { keys.append(OrderKey.between(keys.last, nil)) }
		for _ in 0..<10_000 { keys.insert(OrderKey.between(nil, keys.first), at: 0) }
		assertCanonical(keys)
		#expect(keys.map(\.count).max() == 4)
	}

	/// The LrC killer, pinned: inserting into the SAME gap hundreds of
	/// times (their float scheme silently collided after ~52). The
	/// fraction grows — the accepted, PERF-marked cost of the one pattern
	/// that must pay it — but order never breaks.
	@Test func repeatedSameGapInsertionStaysTotallyOrdered() {
		var keys = ["a0", "a1"]
		for _ in 0..<300 {
			keys.insert(OrderKey.between(keys[0], keys[1]), at: 1)
		}
		assertCanonical(keys)
	}

	@Test func randomizedInsertionsProduceATotalOrder() {
		var generator = SeededGenerator(state: 0xA1EC5)
		var keys = [OrderKey.between(nil, nil)]
		for _ in 0..<1000 {
			let slot = Int.random(in: 0...keys.count, using: &generator)
			let lower = slot > 0 ? keys[slot - 1] : nil
			let upper = slot < keys.count ? keys[slot] : nil
			keys.insert(OrderKey.between(lower, upper), at: slot)
		}
		assertCanonical(keys)
	}

	/// The collation claim, proven rather than assumed: SQLite's default
	/// BINARY TEXT ordering returns the keys in exactly Swift's sorted
	/// order — the property every ORDER BY order_key read stands on.
	@Test func sqliteBinaryCollationAgreesWithSwiftOrdering() throws {
		var generator = SeededGenerator(state: 0x08D3)
		var keys = [OrderKey.between(nil, nil)]
		for _ in 0..<200 {
			let slot = Int.random(in: 0...keys.count, using: &generator)
			keys.insert(
				OrderKey.between(slot > 0 ? keys[slot - 1] : nil, slot < keys.count ? keys[slot] : nil),
				at: slot
			)
		}
		let queue = try DatabaseQueue()
		let fromSQLite = try queue.write { database in
			try database.execute(sql: "CREATE TABLE keys (key TEXT NOT NULL)")
			for key in keys.shuffled(using: &generator) {
				try database.execute(sql: "INSERT INTO keys VALUES (?)", arguments: [key])
			}
			return try String.fetchAll(database, sql: "SELECT key FROM keys ORDER BY key")
		}
		#expect(fromSQLite == keys.sorted())
	}
}
