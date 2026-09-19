//
//  Catalog+Collections.swift
//  Alexandria
//
//  The collections round's verbs (ratified 2026-09-12, _design/
//  collections.md). This file owns BOTH collection tables — `collections`
//  and `collection_members` — because membership is a collection verb from
//  every caller's viewpoint and the child table cannot exist without its
//  parent; the by-table findability rule holds with one file naming the
//  primary table (the Catalog+Assets precedent: a file names the primary
//  table, not the only table touched).
//
//  Every verb is one transaction: one durable gesture, one observation
//  delivery — the judgment durability promise by construction.
//

import Foundation
import GRDB
import Logging

private nonisolated let log = Logger(label: "catalog")

/// The collection verbs' refusals, named so the UI surfaces real messages
/// instead of SQLite codes.
nonisolated enum CollectionError: Error, Equatable {
	/// Create/rename floor: a name must have content (empty or pure
	/// whitespace is refused; anything else is the user's business).
	case emptyName
	/// Move refusal: re-parenting a collection under itself or its own
	/// descendant would orphan the subtree into a cycle.
	case wouldCreateCycle
	/// Reorder refusal: the anchor to drop before isn't a member.
	case anchorNotAMember
	/// Reorder refusal: the anchor is among the moved rows — "before
	/// itself" has no meaning.
	case anchorAmongMoved
	/// A stored order key failed validation: catalog corruption (an
	/// external writer, a damaged file), never a caller mistake. The verb
	/// refuses the one write instead of crashing on a precondition or
	/// corrupting the ordering further.
	// TODO: (catalog-integrity round) refusal is the floor, not the story
	// — detect, surface, and repair (re-mint the collection's keys) belong
	// to the integrity round. Don't lose this.
	case corruptOrderKey(String)
	/// setManualOrder refusal: the ordered list must cover the collection's
	/// members exactly — the caller (the drag round's adopt path) derives it
	/// from an unfiltered, non-union view, so a mismatch is a stale answer
	/// or a programming error, and a partial rewrite would half-scramble a
	/// judgment-class ordering.
	case orderedSetMismatch
	/// Membership-verb refusal: the target is a smart collection, whose
	/// membership is computed from its predicate — it takes no manual adds,
	/// removes, or ordering, by ruling (smart-collection round, 2026-09-18).
	/// The UI keeps smart targets dark; this fence is the authority.
	case membershipIsComputed
}

extension Collection {
	/// The one subtree walk — this collection plus everything nested under
	/// it — composed by every consumer (the verbs, the union's flat sort,
	/// the manual walk), so the concept has ONE implementation. Seeded from
	/// a real record: a nonexistent id walks NOTHING, never a phantom set
	/// holding itself. UNION (not UNION ALL) so a parent cycle terminates
	/// instead of spinning. Callers append their SELECT over `subtree`;
	/// exactly one `?` argument, the root id.
	static let subtreeCTE = """
	WITH RECURSIVE subtree(id) AS (
	    SELECT id FROM collections WHERE id = ?
	    UNION
	    SELECT collections.id FROM collections JOIN subtree ON collections.parent_id = subtree.id)
	"""
}

extension Catalog {

	// MARK: - The collections table

	/// Mints a collection. The name is stored as typed (names are free-form
	/// and may duplicate, by ruling — identity is the id); only emptiness
	/// is refused. A non-nil `predicate` mints a SMART collection: the verb
	/// takes the tree, not a string, so serialization has one fence — an
	/// empty-normalizing or invalid predicate is refused here and a
	/// hand-encoded blob can never enter through the front door.
	@discardableResult
	func createCollection(
		named name: String, under parentId: Identifier<Collection>? = nil,
		predicate: FilterGroup? = nil
	) async throws -> Collection {
		guard !name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
			throw CollectionError.emptyName
		}
		var serialized: String?
		if let predicate {
			// The same posture setFilter enforces: normalized, validated —
			// an empty root is nil everywhere, so a stored empty is refused
			// as the corruption it would later read as.
			guard let normalized = predicate.normalized() else {
				throw FilterError.emptyFilter
			}
			try normalized.validate()
			serialized = try normalized.serialized()
		}
		let collection = Collection(
			id: .mint(), parentId: parentId, name: name, predicate: serialized
		)
		try await databaseWriter.write { try collection.insert($0) }
		return collection
	}

	func renameCollection(_ id: Identifier<Collection>, to name: String) async throws {
		guard !name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
			throw CollectionError.emptyName
		}
		try await databaseWriter.write { database in
			try Collection
				.filter(Collection.Columns.id == id)
				.updateAll(database, Collection.Columns.name.set(to: name))
		}
	}

	/// Re-parents a collection (nil = make it a root). Refuses a
	/// destination inside the moved subtree — the one write that could
	/// mint a parent cycle, checked inside the same transaction so no
	/// concurrent write can slip between check and update.
	func moveCollection(
		_ id: Identifier<Collection>, under destination: Identifier<Collection>?
	) async throws {
		try await databaseWriter.write { database in
			if let destination {
				// The subtree includes its root, so destination == id is
				// refused by the same check.
				let subtree = try Self.subtreeIds(of: id, in: database)
				guard !subtree.contains(destination) else {
					throw CollectionError.wouldCreateCycle
				}
			}
			try Collection
				.filter(Collection.Columns.id == id)
				.updateAll(database, Collection.Columns.parentId.set(to: destination))
		}
	}

	/// The delete confirm's numbers: how many collections the subtree
	/// holds (the root included) and how many memberships go with them.
	/// A nonexistent id honestly reads (0, 0), never a phantom (1, 0).
	func subtreeSummary(
		of id: Identifier<Collection>
	) async throws -> (collections: Int, memberships: Int) {
		try await reader.read { database in
			let ids = try Self.subtreeIds(of: id, in: database)
			let memberships = try CollectionMember
				.filter(ids.contains(CollectionMember.Columns.collectionId))
				.fetchCount(database)
			return (ids.count, memberships)
		}
	}

	/// Deletes a collection and its whole subtree in one transaction.
	/// Memberships go by CASCADE; assets are never touched. Deletion runs
	/// depth-grouped bottom-up — each DELETE statement removes only rows
	/// that are leaves by then — so the parent RESTRICT fence stays
	/// satisfied. (Considered and rejected: `PRAGMA defer_foreign_keys`
	/// would allow one DELETE for the whole subtree, but it relaxes EVERY
	/// foreign-key check in the transaction, not just this one.)
	/// Returns the deleted ids so the caller can retarget a view standing
	/// on any of them (ruling 14) without re-deriving the subtree.
	@discardableResult
	func deleteCollection(_ id: Identifier<Collection>) async throws -> Set<Identifier<Collection>> {
		try await databaseWriter.write { database in
			let ids = try Self.subtreeIds(of: id, in: database)
			let memberships = try CollectionMember
				.filter(ids.contains(CollectionMember.Columns.collectionId))
				.fetchCount(database)
			let records = try Collection
				.filter(ids.contains(Collection.Columns.id))
				.fetchAll(database)
			var childrenOf: [Identifier<Collection>: [Identifier<Collection>]] = [:]
			for record in records where record.id != id {
				if let parent = record.parentId {
					childrenOf[parent, default: []].append(record.id)
				}
			}
			var levels: [[Identifier<Collection>]] = [[id]]
			while let deepest = levels.last {
				let next = deepest.flatMap { childrenOf[$0] ?? [] }
				guard !next.isEmpty else { break }
				levels.append(next)
			}
			for level in levels.reversed() {
				try Collection.deleteAll(database, keys: level)
			}
			// The one verb that destroys judgment work at scale leaves a
			// trace for a human reading a real run.
			log.debug("collection subtree deleted", metadata: [
				"collection": "\(id.rawValue.uuidString)",
				"collections": "\(records.count)",
				"memberships": "\(memberships)",
			])
			return ids
		}
	}

	/// Every collection in the subtree rooted at `id`, the root included —
	/// empty when `id` doesn't exist (Collection.subtreeCTE seeds from a
	/// real record, so a stale id never reads as a phantom one-collection
	/// subtree).
	private nonisolated static func subtreeIds(
		of id: Identifier<Collection>, in database: Database
	) throws -> Set<Identifier<Collection>> {
		try Set(Identifier<Collection>.fetchAll(
			database,
			sql: Collection.subtreeCTE + " SELECT id FROM subtree",
			arguments: [id]
		))
	}

	// MARK: - The collection_members table

	/// The one spelling of "smart = predicate non-NULL" at the SQL level.
	/// `String.fetchOne` reads nil for both "no row" and "NULL predicate" —
	/// intentional: a ghost id is not smart, and each verb keeps its own
	/// ghost behavior (a no-op read, or the membership FK's refusal).
	private nonisolated static func isSmart(
		_ id: Identifier<Collection>, in database: Database
	) throws -> Bool {
		try String.fetchOne(
			database,
			sql: "SELECT predicate FROM collections WHERE id = ?",
			arguments: [id]
		) != nil
	}

	/// The smart fence, checked inside every membership verb's transaction:
	/// a smart collection's membership is computed, never written.
	private nonisolated static func requireManual(
		_ id: Identifier<Collection>, in database: Database
	) throws {
		guard try !isSmart(id, in: database) else {
			throw CollectionError.membershipIsComputed
		}
	}

	/// Whether this collection is smart (predicate non-NULL). The drag
	/// round's hover snapshot reads it; the fence above stays the authority.
	func collectionIsSmart(_ id: Identifier<Collection>) async throws -> Bool {
		try await reader.read { try Self.isSmart(id, in: $0) }
	}

	/// Adds assets to a collection, in the given order, at the end of its
	/// manual order — so "order added" IS the manual order until the first
	/// drag. Idempotent by the composite key: assets already members are
	/// skipped and KEEP their place (re-adding never reorders). Returns how
	/// many were actually new.
	@discardableResult
	func addMembers(
		_ assetIds: [Identifier<Asset>], to collectionId: Identifier<Collection>
	) async throws -> Int {
		guard !assetIds.isEmpty else { return 0 }
		return try await databaseWriter.write { database in
			try Self.requireManual(collectionId, in: database)
			// MAX on TEXT is the BINARY-collation tail — the same order the
			// keys are minted in.
			var tail = try String.fetchOne(
				database,
				sql: "SELECT MAX(order_key) FROM collection_members WHERE collection_id = ?",
				arguments: [collectionId]
			)
			if let tail, !OrderKey.isWellFormed(tail) {
				throw CollectionError.corruptOrderKey(tail)
			}
			var inserted = 0
			for assetId in assetIds {
				let key = OrderKey.between(tail, nil)
				// The conflict target is the composite PK ONLY — never a
				// bare OR IGNORE, which would also swallow an order-key
				// collision and mute the UNIQUE fence's loudness.
				try database.execute(
					sql: """
					INSERT INTO collection_members (collection_id, asset_id, order_key) \
					VALUES (?, ?, ?) ON CONFLICT (collection_id, asset_id) DO NOTHING
					""",
					arguments: [collectionId, assetId, key]
				)
				if database.changesCount > 0 {
					inserted += 1
					tail = key
				}
			}
			return inserted
		}
	}

	/// Removes the given assets' memberships. Assets themselves are never
	/// touched. Returns how many memberships existed to remove.
	@discardableResult
	func removeMembers(
		_ assetIds: [Identifier<Asset>], from collectionId: Identifier<Collection>
	) async throws -> Int {
		guard !assetIds.isEmpty else { return 0 }
		return try await databaseWriter.write { database in
			try Self.requireManual(collectionId, in: database)
			return try CollectionMember
				.filter(CollectionMember.Columns.collectionId == collectionId)
				.filter(assetIds.contains(CollectionMember.Columns.assetId))
				.deleteAll(database)
		}
	}

	/// The drag verb: moves the given members, in the given order, to sit
	/// immediately before `anchor` (nil = to the end). Touches exactly the
	/// moved rows. Reorder never adds: ids that aren't members are skipped
	/// (and repeats keep their first occurrence — a multi-select payload
	/// can legitimately arrive with duplicates). Returns how many rows
	/// actually moved, so a drag that accomplished nothing is visible.
	@discardableResult
	func reorderMembers(
		_ assetIds: [Identifier<Asset>],
		before anchor: Identifier<Asset>?,
		in collectionId: Identifier<Collection>
	) async throws -> Int {
		guard !assetIds.isEmpty else { return 0 }
		if let anchor, assetIds.contains(anchor) {
			throw CollectionError.anchorAmongMoved
		}
		return try await databaseWriter.write { database in
			try Self.requireManual(collectionId, in: database)
			let upper: String?
			if let anchor {
				guard let anchorKey = try String.fetchOne(
					database,
					sql: "SELECT order_key FROM collection_members WHERE collection_id = ? AND asset_id = ?",
					arguments: [collectionId, anchor]
				) else {
					throw CollectionError.anchorNotAMember
				}
				guard OrderKey.isWellFormed(anchorKey) else {
					throw CollectionError.corruptOrderKey(anchorKey)
				}
				upper = anchorKey
			} else {
				upper = nil
			}

			let memberRows = try CollectionMember
				.filter(CollectionMember.Columns.collectionId == collectionId)
				.filter(assetIds.contains(CollectionMember.Columns.assetId))
				.fetchAll(database)
			let members = Set(memberRows.map(\.assetId))
			var seen: Set<Identifier<Asset>> = []
			let moved = assetIds.filter { members.contains($0) && seen.insert($0).inserted }
			guard !moved.isEmpty else { return 0 }

			// Vacate the moved rows' slots first, so a re-minted key can
			// never transiently collide with a moved row's OLD key under
			// the UNIQUE order fence. Delete + reinsert carries the whole
			// record (all three columns are known), so nothing is lost.
			try CollectionMember
				.filter(CollectionMember.Columns.collectionId == collectionId)
				.filter(moved.contains(CollectionMember.Columns.assetId))
				.deleteAll(database)

			// The slot's lower edge: the greatest remaining key below the
			// anchor (nil = dropping at the very front or into empty
			// space). Two SQL forms because the parameterized
			// `(? IS NULL OR …)` disjunct blocks SQLite's covering-index
			// MAX fast path (round review, finding 7).
			var previous: String?
			if let upper {
				previous = try String.fetchOne(
					database,
					sql: "SELECT MAX(order_key) FROM collection_members WHERE collection_id = ? AND order_key < ?",
					arguments: [collectionId, upper]
				)
			} else {
				previous = try String.fetchOne(
					database,
					sql: "SELECT MAX(order_key) FROM collection_members WHERE collection_id = ?",
					arguments: [collectionId]
				)
			}
			if let previous, !OrderKey.isWellFormed(previous) {
				throw CollectionError.corruptOrderKey(previous)
			}
			for assetId in moved {
				let key = OrderKey.between(previous, upper)
				try CollectionMember(
					collectionId: collectionId, assetId: assetId, orderKey: key
				).insert(database)
				previous = key
			}
			return moved.count
		}
	}

	/// The reverse verb: every collection holding this asset, ordered by
	/// id for determinism. Rides idx_collection_members_asset.
	/// Membership rows only, so smart collections never appear — "holds"
	/// here means MANUAL holdings, which is what its one consumer (the drag
	/// badge math) wants. A future "in collections" surface that should
	/// include smart holdings needs its own answer (evaluate each stored
	/// predicate against the one asset), not a widening of this read.
	func collections(
		containing assetId: Identifier<Asset>
	) async throws -> [Identifier<Collection>] {
		try await reader.read { database in
			try Identifier<Collection>.fetchAll(
				database,
				sql: "SELECT collection_id FROM collection_members WHERE asset_id = ? ORDER BY collection_id",
				arguments: [assetId]
			)
		}
	}

	// MARK: - Drag-round reads and the adoption verb (2026-09-18)

	/// The subtree read, public: the drag round's hover snapshot mirrors the
	/// cycle fence with it (the verb's transaction stays the authority).
	func collectionSubtreeIds(
		of id: Identifier<Collection>
	) async throws -> Set<Identifier<Collection>> {
		try await reader.read { database in
			try Self.subtreeIds(of: id, in: database)
		}
	}

	/// For a dragged asset set: collection → how many of these assets it
	/// already holds. Drives the count badge and the zero-add refusal.
	/// Rides idx_collection_members_asset. Chunked, because a select-all
	/// drag can exceed SQLite's bound-parameter ceiling
	/// (SQLITE_MAX_VARIABLE_NUMBER) — and a failed prepare here would be
	/// swallowed as "snapshot failed", leaving every hover verdict
	/// optimistic for the whole drag (round review, finding 7).
	func membershipCounts(
		of assetIds: [Identifier<Asset>]
	) async throws -> [Identifier<Collection>: Int] {
		guard !assetIds.isEmpty else { return [:] }
		return try await reader.read { database in
			var counts: [Identifier<Collection>: Int] = [:]
			let chunkSize = 500
			for start in stride(from: 0, to: assetIds.count, by: chunkSize) {
				let chunk = Array(assetIds[start..<min(start + chunkSize, assetIds.count)])
				let rows = try Row.fetchAll(
					database,
					CollectionMember
						.filter(chunk.contains(CollectionMember.Columns.assetId))
						.select(CollectionMember.Columns.collectionId, count(CollectionMember.Columns.assetId))
						.group(CollectionMember.Columns.collectionId)
				)
				for row in rows {
					counts[row[0] as Identifier<Collection>, default: 0] += row[1] as Int
				}
			}
			return counts
		}
	}

	/// Whether anything nested below this collection contributes members —
	/// the display-faithful union gate (ruled: no reorder in a union view).
	/// A childless or member-less subtree answers false and reorder is live.
	/// A smart DESCENDANT counts as contributing without evaluating its
	/// predicate (smart-collection round): its computed members are on
	/// screen, so adopting the on-screen order would feed setManualOrder
	/// non-members — conservative on purpose, a zero-match predicate still
	/// gates.
	func descendantsContributeMembers(
		of id: Identifier<Collection>
	) async throws -> Bool {
		try await reader.read { database in
			let descendants = try Self.subtreeIds(of: id, in: database).subtracting([id])
			guard !descendants.isEmpty else { return false }
			let smartDescendants = try Collection
				.filter(descendants.contains(Collection.Columns.id))
				.filter(Collection.Columns.predicate != nil)
				.fetchCount(database)
			if smartDescendants > 0 { return true }
			return try CollectionMember
				.filter(descendants.contains(CollectionMember.Columns.collectionId))
				.fetchCount(database) > 0
		}
	}

	/// The adoption verb (drag round, ruled 2026-09-18: a judgment is never
	/// silently overwritten — this runs only behind the explicit switch
	/// confirmation): replaces the collection's ENTIRE manual order with
	/// `ordered`, re-minting every key in one transaction. The list must
	/// cover the members exactly — the caller derives it from an unfiltered,
	/// non-union view, so anything else is refused whole rather than
	/// half-scrambling an ordering.
	func setManualOrder(
		_ ordered: [Identifier<Asset>], in collectionId: Identifier<Collection>
	) async throws {
		try await databaseWriter.write { database in
			try Self.requireManual(collectionId, in: database)
			let members = try Set(Identifier<Asset>.fetchAll(
				database,
				sql: "SELECT asset_id FROM collection_members WHERE collection_id = ?",
				arguments: [collectionId]
			))
			guard members == Set(ordered), members.count == ordered.count else {
				throw CollectionError.orderedSetMismatch
			}
			// Vacate every slot first so fresh keys can never transiently
			// collide with standing ones under the UNIQUE order fence (the
			// reorderMembers precedent).
			try CollectionMember
				.filter(CollectionMember.Columns.collectionId == collectionId)
				.deleteAll(database)
			var tail: String?
			for assetId in ordered {
				let key = OrderKey.between(tail, nil)
				try CollectionMember(
					collectionId: collectionId, assetId: assetId, orderKey: key
				).insert(database)
				tail = key
			}
			log.debug("manual order adopted", metadata: [
				"collection": "\(collectionId.rawValue.uuidString)",
				"members": "\(ordered.count)",
			])
		}
	}
}
