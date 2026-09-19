//
//  SmartCollectionTests.swift
//  AlexandriaTests
//
//  The smart-collection round (2026-09-18): a smart collection is the same
//  noun with a stored predicate — membership computed, never written. Pinned
//  here: the create fence (tree in, normalized envelope stored), the four
//  membership verbs' refusals, the query semantics (predicate as the
//  source's clause, live filter ANDing on top, smart children feeding both
//  union orders), the corrupt-predicate posture (one row disabled; the
//  VIEWED root surfaces the unreadable flag), and the union gate counting
//  smart descendants.
//

import Foundation
import GRDB
import Testing
@testable import Alexandria

struct SmartCollectionTests {

	private func ratingAtLeast(_ n: Int) -> FilterGroup {
		FilterGroup(combine: .and, children: [
			.token(FilterToken(field: .rating, op: .gte, value: .int(n))),
		])
	}

	private func fetch(
		_ query: WorkingSetQuery, in catalog: Catalog
	) async throws -> WorkingSetQuery.Answer {
		try await catalog.reader.read { try query.fetchAnswer($0) }
	}

	private func corrupt(
		_ id: Identifier<Collection>, to blob: String, in catalog: Catalog
	) async throws {
		// No verb mints a bad predicate; corruption enters sideways (an
		// external writer, disk damage) — modeled here as a raw write.
		try await catalog.databaseWriter.write {
			try $0.execute(
				sql: "UPDATE collections SET predicate = ? WHERE id = ?",
				arguments: [blob, id]
			)
		}
	}

	/// Three formed assets keyed by name — the filter tests' fixture shape.
	private struct Fixture {
		let context: ImportContext
		var catalog: Catalog { context.catalog }
		let asset: [String: Identifier<Asset>]
	}

	private func fixture() async throws -> Fixture {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/a.jpg"),
			context.prepared("/Volumes/Test/Shoot/b.jpg"),
			context.prepared("/Volumes/Test/Shoot/c.jpg"),
		])
		try await context.form()
		let files = try await context.catalog.files(inImport: context.importId)
		var byName: [String: Identifier<Asset>] = [:]
		for file in files {
			if let assetId = file.assetId { byName[String(file.name.prefix(1))] = assetId }
		}
		return Fixture(context: context, asset: byName)
	}

	// MARK: - The create fence

	@Test func createStoresTheNormalizedEnvelopeAndReadsBack() async throws {
		let catalog = try Catalog(DatabaseQueue(path: ":memory:"))
		let group = ratingAtLeast(3)
		let created = try await catalog.createCollection(named: "Smart", predicate: group)
		#expect(created.isSmart)
		// Ruled: saves land at top-level root, and the envelope carries the
		// predicate ONLY — no source/scope component exists to store.
		#expect(created.parentId == nil)
		let stored = try #require(created.predicate)
		// The stored blob round-trips through the one persistence fence to
		// the same tree the caller handed in.
		#expect(try FilterGroup(serialized: stored) == group)
	}

	@Test func createRefusesAnEmptyNormalizingPredicate() async throws {
		let catalog = try Catalog(DatabaseQueue(path: ":memory:"))
		await #expect(throws: FilterError.emptyFilter) {
			try await catalog.createCollection(
				named: "Hollow", predicate: FilterGroup(combine: .and, children: [])
			)
		}
	}

	// MARK: - The membership fences

	@Test func everyMembershipVerbRefusesASmartCollection() async throws {
		let catalog = try Catalog(DatabaseQueue(path: ":memory:"))
		let smart = try await catalog.createCollection(named: "Smart", predicate: ratingAtLeast(1))
		let ghost = Identifier<Asset>.mint()
		await #expect(throws: CollectionError.membershipIsComputed) {
			_ = try await catalog.addMembers([ghost], to: smart.id)
		}
		await #expect(throws: CollectionError.membershipIsComputed) {
			_ = try await catalog.removeMembers([ghost], from: smart.id)
		}
		await #expect(throws: CollectionError.membershipIsComputed) {
			_ = try await catalog.reorderMembers([ghost], before: nil, in: smart.id)
		}
		await #expect(throws: CollectionError.membershipIsComputed) {
			try await catalog.setManualOrder([], in: smart.id)
		}
	}

	@Test func collectionIsSmartAnswersByPredicatePresence() async throws {
		let catalog = try Catalog(DatabaseQueue(path: ":memory:"))
		let manual = try await catalog.createCollection(named: "Manual")
		let smart = try await catalog.createCollection(named: "Smart", predicate: ratingAtLeast(1))
		#expect(try await catalog.collectionIsSmart(manual.id) == false)
		#expect(try await catalog.collectionIsSmart(smart.id) == true)
	}

	// MARK: - The query

	@Test func aSmartSourceComputesMembershipFromItsPredicate() async throws {
		let fx = try await fixture()
		let a = try #require(fx.asset["a"]), c = try #require(fx.asset["c"])
		_ = try await fx.catalog.setRating([a], to: 5)
		_ = try await fx.catalog.setRating([c], to: 3)
		let smart = try await fx.catalog.createCollection(named: "Best", predicate: ratingAtLeast(4))

		let query = WorkingSetQuery(
			lens: .assets, source: .collection(smart.id), arrangement: Arrangement()
		)
		let answer = try await fetch(query, in: fx.catalog)
		#expect(answer.ids == [.asset(a)])
		#expect(!answer.predicateUnreadable)
	}

	@Test func theLiveFilterNarrowsASmartSource() async throws {
		// Ruled precedence: the predicate is the SOURCE's clause, the live
		// filter ANDs on top — a filter over a smart collection means what
		// it means everywhere else.
		let fx = try await fixture()
		let a = try #require(fx.asset["a"]), c = try #require(fx.asset["c"])
		_ = try await fx.catalog.setRating([a], to: 5)
		_ = try await fx.catalog.setRating([c], to: 3)
		let smart = try await fx.catalog.createCollection(named: "Rated", predicate: ratingAtLeast(3))

		let query = WorkingSetQuery(
			lens: .assets, source: .collection(smart.id), arrangement: Arrangement(),
			filter: ratingAtLeast(4)
		)
		#expect(try await fetch(query, in: fx.catalog).ids == [.asset(a)])
	}

	@Test func aSmartChildContributesToTheParentsFlatUnion() async throws {
		let fx = try await fixture()
		let a = try #require(fx.asset["a"]), b = try #require(fx.asset["b"])
		_ = try await fx.catalog.setRating([a], to: 5)
		let parent = try await fx.catalog.createCollection(named: "Parent")
		_ = try await fx.catalog.addMembers([b], to: parent.id)
		_ = try await fx.catalog.createCollection(
			named: "Smart Child", under: parent.id, predicate: ratingAtLeast(4)
		)

		let query = WorkingSetQuery(
			lens: .assets, source: .collection(parent.id), arrangement: Arrangement()
		)
		#expect(Set(try await fetch(query, in: fx.catalog).ids) == [.asset(a), .asset(b)])
	}

	@Test func theSectionedWalkPlacesASmartBlockAfterAuthoredMembers() async throws {
		// Ruling 5 extended: a smart child's block is its computed members
		// in added order; the dedupe keeps first appearance — an asset both
		// authored on the parent and matched by the child appears once, in
		// its authored slot.
		let fx = try await fixture()
		let a = try #require(fx.asset["a"]), c = try #require(fx.asset["c"])
		_ = try await fx.catalog.setRating([a, c], to: 5)
		let parent = try await fx.catalog.createCollection(named: "Parent")
		_ = try await fx.catalog.addMembers([c], to: parent.id)
		_ = try await fx.catalog.createCollection(
			named: "Smart Child", under: parent.id, predicate: ratingAtLeast(4)
		)

		var arrangement = Arrangement()
		arrangement.sortKey = .manual
		let query = WorkingSetQuery(
			lens: .assets, source: .collection(parent.id), arrangement: arrangement
		)
		// c authored first; a arrives through the smart block; c is NOT
		// repeated by the block it also matches.
		#expect(try await fetch(query, in: fx.catalog).ids == [.asset(c), .asset(a)])
	}

	// MARK: - The corrupt-predicate posture

	@Test func aCorruptViewedPredicateAnswersEmptyAndUnreadable() async throws {
		let fx = try await fixture()
		let smart = try await fx.catalog.createCollection(named: "Rotten", predicate: ratingAtLeast(1))
		try await corrupt(smart.id, to: "not json", in: fx.catalog)

		let query = WorkingSetQuery(
			lens: .assets, source: .collection(smart.id), arrangement: Arrangement()
		)
		let answer = try await fetch(query, in: fx.catalog)
		#expect(answer.ids.isEmpty)
		#expect(answer.predicateUnreadable)

		// A newer vocabulary generation is the same posture: refused whole
		// (skipping unknown tokens would widen), surfaced, never a crash.
		try await corrupt(
			smart.id,
			to: #"{"version":99,"root":{"combine":"and","children":[]}}"#,
			in: fx.catalog
		)
		let skewed = try await fetch(query, in: fx.catalog)
		#expect(skewed.ids.isEmpty)
		#expect(skewed.predicateUnreadable)
	}

	@Test func aCorruptRootSuppressesItsDescendantsBehindTheNotice() async throws {
		// Ruled (review finding 1, option (a)): an unreadable VIEWED
		// predicate answers EMPTY even when manual descendants hold
		// members — a partial set behind the notice would count in the
		// status bar and put the cursor on assets the grid never draws.
		let fx = try await fixture()
		let b = try #require(fx.asset["b"])
		let smart = try await fx.catalog.createCollection(named: "Rotten", predicate: ratingAtLeast(1))
		let child = try await fx.catalog.createCollection(named: "Child", under: smart.id)
		_ = try await fx.catalog.addMembers([b], to: child.id)
		try await corrupt(smart.id, to: "not json", in: fx.catalog)

		let flat = WorkingSetQuery(
			lens: .assets, source: .collection(smart.id), arrangement: Arrangement()
		)
		let flatAnswer = try await fetch(flat, in: fx.catalog)
		#expect(flatAnswer.ids.isEmpty)
		#expect(flatAnswer.predicateUnreadable)

		var manual = Arrangement()
		manual.sortKey = .manual
		let sectioned = WorkingSetQuery(
			lens: .assets, source: .collection(smart.id), arrangement: manual
		)
		let sectionedAnswer = try await fetch(sectioned, in: fx.catalog)
		#expect(sectionedAnswer.ids.isEmpty)
		#expect(sectionedAnswer.predicateUnreadable)
	}

	@Test func manualOverASmartRootActsAsAddedOrder() async throws {
		// Ruled loose edge, pinned: manual is offerable over a smart root
		// (posture can't see smartness) and its block reads back in added
		// order — exactly the .added ascending answer.
		let fx = try await fixture()
		let a = try #require(fx.asset["a"]), b = try #require(fx.asset["b"]), c = try #require(fx.asset["c"])
		_ = try await fx.catalog.setRating([a, b, c], to: 5)
		let smart = try await fx.catalog.createCollection(named: "All", predicate: ratingAtLeast(4))

		var manual = Arrangement()
		manual.sortKey = .manual
		var added = Arrangement()
		added.sortKey = .added
		added.direction = .ascending
		let manualAnswer = try await fetch(
			WorkingSetQuery(lens: .assets, source: .collection(smart.id), arrangement: manual),
			in: fx.catalog
		)
		let addedAnswer = try await fetch(
			WorkingSetQuery(lens: .assets, source: .collection(smart.id), arrangement: added),
			in: fx.catalog
		)
		#expect(manualAnswer.ids.count == 3)
		#expect(manualAnswer.ids == addedAnswer.ids)
	}

	@Test func theLiveFilterNarrowsASmartBlockInTheSectionedWalk() async throws {
		// The manual path applies the filter through a different mechanism
		// than the flat splice (the matching set intersected in the walk);
		// the two must agree about smart blocks.
		let fx = try await fixture()
		let a = try #require(fx.asset["a"]), b = try #require(fx.asset["b"]), c = try #require(fx.asset["c"])
		_ = try await fx.catalog.setRating([a, c], to: 5)
		_ = try await fx.catalog.setRating([b], to: 3)
		let parent = try await fx.catalog.createCollection(named: "Parent")
		_ = try await fx.catalog.addMembers([c], to: parent.id)
		_ = try await fx.catalog.createCollection(
			named: "Smart Child", under: parent.id, predicate: ratingAtLeast(3)
		)

		var manual = Arrangement()
		manual.sortKey = .manual
		let query = WorkingSetQuery(
			lens: .assets, source: .collection(parent.id), arrangement: manual,
			filter: ratingAtLeast(4)
		)
		// c authored (survives), the block matches {a, b, c} but the filter
		// keeps a only (c deduped to its authored slot, b dropped).
		#expect(try await fetch(query, in: fx.catalog).ids == [.asset(c), .asset(a)])
	}

	@Test func aCorruptSmartChildDisablesOnlyItsOwnContribution() async throws {
		let fx = try await fixture()
		let b = try #require(fx.asset["b"])
		let parent = try await fx.catalog.createCollection(named: "Parent")
		_ = try await fx.catalog.addMembers([b], to: parent.id)
		let child = try await fx.catalog.createCollection(
			named: "Rotten Child", under: parent.id, predicate: ratingAtLeast(1)
		)
		try await corrupt(child.id, to: "not json", in: fx.catalog)

		let query = WorkingSetQuery(
			lens: .assets, source: .collection(parent.id), arrangement: Arrangement()
		)
		let answer = try await fetch(query, in: fx.catalog)
		// The parent's own answer stands; the corrupt row contributes
		// nothing and the notice is NOT the parent's to show.
		#expect(answer.ids == [.asset(b)])
		#expect(!answer.predicateUnreadable)
	}

	// MARK: - The union gate

	@Test func aSmartDescendantGatesReorderWithoutEvaluation() async throws {
		// The drag round's display-faithful gate: a smart child's computed
		// members are on screen, so the on-screen order can't be adopted as
		// the parent's manual order — even when the predicate matches
		// nothing (conservative by design).
		let catalog = try Catalog(DatabaseQueue(path: ":memory:"))
		let parent = try await catalog.createCollection(named: "Parent")
		#expect(try await catalog.descendantsContributeMembers(of: parent.id) == false)
		_ = try await catalog.createCollection(
			named: "Smart Child", under: parent.id, predicate: ratingAtLeast(1)
		)
		#expect(try await catalog.descendantsContributeMembers(of: parent.id) == true)
	}
}
