//
//  CatalogCollectionsTests.swift
//  AlexandriaTests
//
//  The collections round's ratified behavior, pinned against the real
//  verbs (2026-09-12, _design/collections.md): name floor, free-form
//  duplicate names, cycle refusal, subtree delete, membership idempotence,
//  added-order-is-manual-order, reorder semantics, the reverse lookup, and
//  the schema fences chunk 1 deferred to real write paths (UNIQUE order
//  key, CASCADE on both membership parents).
//

import Foundation
import GRDB
import Testing
@testable import Alexandria

struct CatalogCollectionsTests {

	private func makeCatalog() throws -> Catalog {
		try Catalog(DatabaseQueue(path: ":memory:"))
	}

	/// Member asset ids in manual order — the read every ordered surface
	/// will make.
	private func memberOrder(
		_ catalog: Catalog, in collection: Identifier<Collection>
	) async throws -> [Identifier<Asset>] {
		try await catalog.reader.read { database in
			try Identifier<Asset>.fetchAll(
				database,
				sql: "SELECT asset_id FROM collection_members WHERE collection_id = ? ORDER BY order_key",
				arguments: [collection]
			)
		}
	}

	private func orderKeys(
		_ catalog: Catalog, in collection: Identifier<Collection>
	) async throws -> [Identifier<Asset>: String] {
		try await catalog.reader.read { database in
			var keys: [Identifier<Asset>: String] = [:]
			for member in try CollectionMember
				.filter(CollectionMember.Columns.collectionId == collection)
				.fetchAll(database) {
				keys[member.assetId] = member.orderKey
			}
			return keys
		}
	}

	// MARK: - The collections table

	@Test func nameFloorRejectsEmptyAndWhitespaceOnly() async throws {
		let catalog = try makeCatalog()
		await #expect(throws: CollectionError.emptyName) {
			try await catalog.createCollection(named: "")
		}
		await #expect(throws: CollectionError.emptyName) {
			try await catalog.createCollection(named: "   \n")
		}
		let collection = try await catalog.createCollection(named: "Trips")
		await #expect(throws: CollectionError.emptyName) {
			try await catalog.renameCollection(collection.id, to: " ")
		}
		try await catalog.renameCollection(collection.id, to: "Voyages")
		let renamed = try await catalog.reader.read { database in
			try Collection.fetchOne(database, key: collection.id)
		}
		#expect(renamed?.name == "Voyages")
	}

	/// Pins the no-uniqueness ruling: names are free-form, siblings and
	/// roots included — identity is the id.
	@Test func duplicateNamesAreAllowedEverywhere() async throws {
		let catalog = try makeCatalog()
		let first = try await catalog.createCollection(named: "Trips")
		let second = try await catalog.createCollection(named: "Trips")
		try await catalog.createCollection(named: "Trips", under: first.id)
		try await catalog.createCollection(named: "Trips", under: first.id)
		#expect(first.id != second.id)
		let total = try await catalog.reader.read { try Collection.fetchCount($0) }
		#expect(total == 4)
	}

	@Test func moveReparentsButRefusesSelfAndDescendants() async throws {
		let catalog = try makeCatalog()
		let a = try await catalog.createCollection(named: "A")
		let b = try await catalog.createCollection(named: "B", under: a.id)
		let c = try await catalog.createCollection(named: "C", under: b.id)

		await #expect(throws: CollectionError.wouldCreateCycle) {
			try await catalog.moveCollection(a.id, under: a.id)
		}
		await #expect(throws: CollectionError.wouldCreateCycle) {
			try await catalog.moveCollection(a.id, under: c.id)
		}

		// Legal moves: to a root, and back under a non-descendant.
		try await catalog.moveCollection(c.id, under: nil)
		var parent = try await catalog.reader.read { try Collection.fetchOne($0, key: c.id)?.parentId }
		#expect(parent == nil)
		try await catalog.moveCollection(c.id, under: a.id)
		parent = try await catalog.reader.read { try Collection.fetchOne($0, key: c.id)?.parentId }
		#expect(parent == a.id)
	}

	@Test func deleteRemovesTheSubtreeAndMembershipsButNeverAssets() async throws {
		let catalog = try makeCatalog()
		let a = try await catalog.createCollection(named: "A")
		let b = try await catalog.createCollection(named: "B", under: a.id)
		let c = try await catalog.createCollection(named: "C", under: b.id)
		let keeper = try await catalog.createCollection(named: "Keeper")

		let asset1 = try await seedAsset(catalog, at: 1_000)
		let asset2 = try await seedAsset(catalog, at: 1_001)
		try await catalog.addMembers([asset1], to: a.id)
		try await catalog.addMembers([asset1, asset2], to: c.id)
		try await catalog.addMembers([asset2], to: keeper.id)

		let summary = try await catalog.subtreeSummary(of: a.id)
		#expect(summary.collections == 3)
		#expect(summary.memberships == 3)

		try await catalog.deleteCollection(a.id)

		let (collections, memberships, assets) = try await catalog.reader.read { database in
			(try Collection.fetchCount(database),
			 try CollectionMember.fetchCount(database),
			 try Asset.fetchCount(database))
		}
		#expect(collections == 1)   // Keeper survives
		#expect(memberships == 1)   // Keeper's membership survives
		#expect(assets == 2)        // assets are never touched
	}

	// MARK: - The collection_members table

	@Test func addIsIdempotentCountsNewRowsAndKeepsAddedOrder() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let x = try await seedAsset(catalog, at: 1_000)
		let y = try await seedAsset(catalog, at: 1_001)
		let z = try await seedAsset(catalog, at: 1_002)
		let w = try await seedAsset(catalog, at: 1_003)

		let first = try await catalog.addMembers([x, y, z], to: collection.id)
		#expect(first == 3)
		#expect(try await memberOrder(catalog, in: collection.id) == [x, y, z])

		let keysBefore = try await orderKeys(catalog, in: collection.id)
		// Re-adding y is skipped AND leaves y's place untouched; w appends.
		let second = try await catalog.addMembers([y, w], to: collection.id)
		#expect(second == 1)
		#expect(try await memberOrder(catalog, in: collection.id) == [x, y, z, w])
		let keysAfter = try await orderKeys(catalog, in: collection.id)
		#expect(keysAfter[y] == keysBefore[y])
	}

	@Test func removeTakesOnlyTheGivenMemberships() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let other = try await catalog.createCollection(named: "Other")
		let x = try await seedAsset(catalog, at: 1_000)
		let y = try await seedAsset(catalog, at: 1_001)
		try await catalog.addMembers([x, y], to: collection.id)
		try await catalog.addMembers([x], to: other.id)

		let removed = try await catalog.removeMembers([x, y], from: collection.id)
		#expect(removed == 2)
		#expect(try await memberOrder(catalog, in: collection.id).isEmpty)
		// x's membership elsewhere is untouched, and x itself survives.
		#expect(try await memberOrder(catalog, in: other.id) == [x])
	}

	@Test func reorderMovesBeforeTheAnchorTouchingOnlyMovedRows() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 1_001)
		let c = try await seedAsset(catalog, at: 1_002)
		let d = try await seedAsset(catalog, at: 1_003)
		try await catalog.addMembers([a, b, c, d], to: collection.id)
		let keysBefore = try await orderKeys(catalog, in: collection.id)

		// Drag d and b (in that grab order) to sit before a.
		try await catalog.reorderMembers([d, b], before: a, in: collection.id)
		#expect(try await memberOrder(catalog, in: collection.id) == [d, b, a, c])

		// The unmoved rows' keys are untouched — a reorder writes exactly
		// the moved rows.
		let keysAfter = try await orderKeys(catalog, in: collection.id)
		#expect(keysAfter[a] == keysBefore[a])
		#expect(keysAfter[c] == keysBefore[c])
	}

	@Test func reorderWithNilAnchorMovesToTheEnd() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 1_001)
		let c = try await seedAsset(catalog, at: 1_002)
		try await catalog.addMembers([a, b, c], to: collection.id)

		try await catalog.reorderMembers([a], before: nil, in: collection.id)
		#expect(try await memberOrder(catalog, in: collection.id) == [b, c, a])
	}

	@Test func reorderRefusesBadAnchorsAndNeverAdds() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 1_001)
		let stranger = try await seedAsset(catalog, at: 1_002)
		try await catalog.addMembers([a, b], to: collection.id)

		await #expect(throws: CollectionError.anchorNotAMember) {
			try await catalog.reorderMembers([b], before: stranger, in: collection.id)
		}
		await #expect(throws: CollectionError.anchorAmongMoved) {
			try await catalog.reorderMembers([a, b], before: b, in: collection.id)
		}
		// A non-member in the moved set is skipped — reorder never adds.
		try await catalog.reorderMembers([stranger, b], before: a, in: collection.id)
		#expect(try await memberOrder(catalog, in: collection.id) == [b, a])
	}

	@Test func reverseLookupListsCollectionsHoldingAnAssetInIdOrder() async throws {
		let catalog = try makeCatalog()
		let first = try await catalog.createCollection(named: "Trips")
		let second = try await catalog.createCollection(named: "Selects")
		let third = try await catalog.createCollection(named: "Print Run")
		let asset = try await seedAsset(catalog, at: 1_000)
		let loner = try await seedAsset(catalog, at: 1_001)
		try await catalog.addMembers([asset], to: first.id)
		try await catalog.addMembers([asset, loner], to: second.id)
		try await catalog.addMembers([asset], to: third.id)

		// Deterministic: exactly the holding collections, in id order —
		// asserted against an explicitly sorted expectation, not a
		// sortedness check that could pass by luck.
		let expected = [first.id, second.id, third.id]
			.sorted { $0.rawValue.uuidString < $1.rawValue.uuidString }
		let holding = try await catalog.collections(containing: asset)
		#expect(holding == expected)
	}

	@Test func namesRoundTripAsTyped() async throws {
		let catalog = try makeCatalog()
		// Free-form by ruling: surrounding whitespace is the user's
		// business (only empty/whitespace-ONLY is refused) and survives
		// storage verbatim.
		let created = try await catalog.createCollection(named: "  Trips  ")
		let stored = try await catalog.reader.read { try Collection.fetchOne($0, key: created.id) }
		#expect(stored?.name == "  Trips  ")
	}

	@Test func reorderToleratesDuplicateIdsInTheMovedSet() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 1_001)
		let c = try await seedAsset(catalog, at: 1_002)
		try await catalog.addMembers([a, b, c], to: collection.id)

		// A multi-select drag payload can repeat ids; first occurrence
		// wins, no constraint blowup.
		let movedCount = try await catalog.reorderMembers([c, a, c], before: nil, in: collection.id)
		#expect(movedCount == 2)
		#expect(try await memberOrder(catalog, in: collection.id) == [b, c, a])
	}

	@Test func reorderingEveryMemberIntoEmptySpaceWorks() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 1_001)
		let c = try await seedAsset(catalog, at: 1_002)
		try await catalog.addMembers([a, b, c], to: collection.id)

		// The whole membership moves at once: after vacating, both edges
		// are open (previous nil, anchor nil) and minting starts fresh.
		let movedCount = try await catalog.reorderMembers([c, b, a], before: nil, in: collection.id)
		#expect(movedCount == 3)
		#expect(try await memberOrder(catalog, in: collection.id) == [c, b, a])
	}

	/// Pins the ghost-id contract the doc comments claim: a nonexistent
	/// collection reads as an honest empty subtree — (0, 0) from the
	/// summary, a clean no-op from delete and move (a ghost self-move
	/// no-ops rather than refusing: there is no collection to cycle).
	@Test func ghostCollectionIdsReadEmptyAndWriteNothing() async throws {
		let catalog = try makeCatalog()
		let real = try await catalog.createCollection(named: "Real")
		let ghost = Identifier<Collection>.mint()

		let summary = try await catalog.subtreeSummary(of: ghost)
		#expect(summary.collections == 0)
		#expect(summary.memberships == 0)

		try await catalog.deleteCollection(ghost)
		try await catalog.moveCollection(ghost, under: ghost)
		try await catalog.moveCollection(ghost, under: real.id)

		let (collections, ghostRow) = try await catalog.reader.read { database in
			(try Collection.fetchCount(database),
			 try Collection.fetchOne(database, key: ghost))
		}
		#expect(collections == 1)      // Real survives untouched
		#expect(ghostRow == nil)       // nothing was minted by the no-ops
	}

	@Test func corruptAnchorKeyIsRefusedOnReorder() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 1_001)
		try await catalog.addMembers([a, b], to: collection.id)

		// Sabotage the ANCHOR's stored key: the reorder must refuse with
		// the named error at the anchor-read site, never crash.
		try await catalog.databaseWriter.write { database in
			try database.execute(
				sql: "UPDATE collection_members SET order_key = '~~' WHERE asset_id = ?",
				arguments: [a]
			)
		}
		await #expect(throws: CollectionError.corruptOrderKey("~~")) {
			try await catalog.reorderMembers([b], before: a, in: collection.id)
		}
	}

	@Test func corruptLowerEdgeKeyIsRefusedOnReorder() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 1_001)
		let c = try await seedAsset(catalog, at: 1_002)
		try await catalog.addMembers([a, b, c], to: collection.id)

		// Sabotage a NON-moved, non-anchor row ('|' sorts above 'z', so
		// it becomes the surviving MAX below nil-anchor): the lower-edge
		// read must refuse with the named error.
		try await catalog.databaseWriter.write { database in
			try database.execute(
				sql: "UPDATE collection_members SET order_key = '||' WHERE asset_id = ?",
				arguments: [b]
			)
		}
		await #expect(throws: CollectionError.corruptOrderKey("||")) {
			try await catalog.reorderMembers([a], before: nil, in: collection.id)
		}
	}

	@Test func corruptStoredOrderKeyIsRefusedNotCrashed() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 1_001)
		try await catalog.addMembers([a], to: collection.id)

		// Sabotage the stored key with bytes above the alphabet, so the
		// tail read surfaces it: the verb must refuse with a named error
		// (an I/O fact), never abort on the math's precondition.
		try await catalog.databaseWriter.write { database in
			try database.execute(
				sql: "UPDATE collection_members SET order_key = '~~' WHERE asset_id = ?",
				arguments: [a]
			)
		}
		await #expect(throws: CollectionError.corruptOrderKey("~~")) {
			try await catalog.addMembers([b], to: collection.id)
		}
	}

	// MARK: - Schema fences, proven on real rows

	@Test func duplicateOrderKeysAreRefusedLoudly() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 1_001)
		try await catalog.addMembers([a], to: collection.id)
		let key = try await orderKeys(catalog, in: collection.id)[a]

		// The LrC silent-identical-position rot, made a loud failure.
		await #expect(throws: DatabaseError.self) {
			try await catalog.databaseWriter.write { database in
				try database.execute(
					sql: "INSERT INTO collection_members (collection_id, asset_id, order_key) VALUES (?, ?, ?)",
					arguments: [collection.id, b, key]
				)
			}
		}
	}

	@Test func membershipCascadesWhenItsAssetGoes() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let asset = try await seedAsset(catalog, at: 1_000)
		try await catalog.addMembers([asset], to: collection.id)

		// A file-less asset can be deleted directly; its membership must
		// follow (a membership is meaningless without either parent).
		try await catalog.databaseWriter.write { database in
			_ = try Asset.deleteOne(database, key: asset)
		}
		#expect(try await memberOrder(catalog, in: collection.id).isEmpty)
	}

	/// Chunk 4: the delete verb returns exactly the subtree it removed —
	/// the ruling-14 retarget's input — and a ghost id returns empty.
	@Test func deleteReturnsTheDeletedSubtreeIds() async throws {
		let catalog = try makeCatalog()
		let trips = try await catalog.createCollection(named: "Trips")
		let child = try await catalog.createCollection(named: "Iceland", under: trips.id)
		let grand = try await catalog.createCollection(named: "Raw", under: child.id)
		let bystander = try await catalog.createCollection(named: "Picks")

		let deleted = try await catalog.deleteCollection(trips.id)
		#expect(deleted == Set([trips.id, child.id, grand.id]))
		#expect(try await catalog.subtreeSummary(of: bystander.id).collections == 1)

		let ghost = try await catalog.deleteCollection(Identifier<Collection>(rawValue: .v7()))
		#expect(ghost.isEmpty)
	}

	// MARK: - Drag round: the snapshot reads and the adoption verb (2026-09-18)

	@Test func membershipCountsAnswerPerCollectionForADraggedSet() async throws {
		let catalog = try makeCatalog()
		let selects = try await catalog.createCollection(named: "Selects")
		let picks = try await catalog.createCollection(named: "Picks")
		let x = try await seedAsset(catalog, at: 1_000)
		let y = try await seedAsset(catalog, at: 1_001)
		let z = try await seedAsset(catalog, at: 1_002)
		try await catalog.addMembers([x, y], to: selects.id)
		try await catalog.addMembers([y], to: picks.id)

		let counts = try await catalog.membershipCounts(of: [x, y, z])
		#expect(counts == [selects.id: 2, picks.id: 1])
		#expect(try await catalog.membershipCounts(of: []).isEmpty)
	}

	/// The union gate's read: a leaf answers false, a child WITH members
	/// answers true, and a member-less child keeps reorder live. (It is
	/// subtree-membership truth, conservatively: a child whose members are
	/// a subset of the parent's own displays identically yet still gates —
	/// safe in the refusing direction.)
	@Test func descendantsContributeMembersAnswersSubtreeMembership() async throws {
		let catalog = try makeCatalog()
		let trips = try await catalog.createCollection(named: "Trips")
		let iceland = try await catalog.createCollection(named: "Iceland", under: trips.id)
		let asset = try await seedAsset(catalog, at: 1_000)
		try await catalog.addMembers([asset], to: trips.id)

		#expect(try await catalog.descendantsContributeMembers(of: trips.id) == false)
		#expect(try await catalog.descendantsContributeMembers(of: iceland.id) == false)

		try await catalog.addMembers([asset], to: iceland.id)
		#expect(try await catalog.descendantsContributeMembers(of: trips.id) == true)
	}

	@Test func subtreeReadMirrorsTheVerbFence() async throws {
		let catalog = try makeCatalog()
		let a = try await catalog.createCollection(named: "A")
		let b = try await catalog.createCollection(named: "B", under: a.id)
		#expect(try await catalog.collectionSubtreeIds(of: a.id) == Set([a.id, b.id]))
		#expect(try await catalog.collectionSubtreeIds(of: Identifier<Collection>(rawValue: .v7())).isEmpty)
	}

	@Test func setManualOrderReplacesTheWholeOrderInOneGesture() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let x = try await seedAsset(catalog, at: 1_000)
		let y = try await seedAsset(catalog, at: 1_001)
		let z = try await seedAsset(catalog, at: 1_002)
		try await catalog.addMembers([x, y, z], to: collection.id)

		try await catalog.setManualOrder([z, x, y], in: collection.id)
		#expect(try await memberOrder(catalog, in: collection.id) == [z, x, y])

		// Adoption is repeatable: fresh keys, same fences.
		try await catalog.setManualOrder([y, z, x], in: collection.id)
		#expect(try await memberOrder(catalog, in: collection.id) == [y, z, x])
	}

	/// The exact-cover fence: a subset, a superset, or a duplicate would
	/// half-scramble a judgment-class ordering — refused whole, nothing
	/// written (the drag round's adopt path guarantees the cover by
	/// posture; this is what catches a stale answer).
	@Test func setManualOrderRefusesAnythingButAnExactMemberCover() async throws {
		let catalog = try makeCatalog()
		let collection = try await catalog.createCollection(named: "Selects")
		let x = try await seedAsset(catalog, at: 1_000)
		let y = try await seedAsset(catalog, at: 1_001)
		let stranger = try await seedAsset(catalog, at: 1_002)
		try await catalog.addMembers([x, y], to: collection.id)

		await #expect(throws: CollectionError.orderedSetMismatch) {
			try await catalog.setManualOrder([x], in: collection.id)
		}
		await #expect(throws: CollectionError.orderedSetMismatch) {
			try await catalog.setManualOrder([x, y, stranger], in: collection.id)
		}
		await #expect(throws: CollectionError.orderedSetMismatch) {
			try await catalog.setManualOrder([x, y, y], in: collection.id)
		}
		// Nothing was written by any refusal.
		#expect(try await memberOrder(catalog, in: collection.id) == [x, y])
	}
}
