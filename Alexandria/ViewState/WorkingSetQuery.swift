//
//  WorkingSetQuery.swift
//  Alexandria
//
//  The question's vocabulary and its compilation (UI-layer design round,
//  2026-09-11). A query is lens + source + arrangement — FROM, WHERE, and
//  ORDER BY — compiled to one SQL statement over the catalog. The filter's
//  clause joins the WHERE when the filter round mints its type.
//

import Foundation
import GRDB

/// The unit the working set ranges over — the FROM clause. A source or
/// filter clause narrows membership; the lens decides what a member IS.
nonisolated enum Lens: Sendable {
	case assets
	case files
}

/// The sidebar's clause: where the user is looking. A kind of filter by
/// ruling (scope ⊂ filter, 2026-09-11) — it compiles to a WHERE like any
/// other — but held as its own posture field so the browser authors it
/// without read-modify-writing the future filter's tokens, and so it stays
/// separately clearable.
nonisolated enum Source: Hashable, Sendable {
	case library
	/// Subtree by ruling (browser round, 2026-09-11): a folder shows
	/// everything in it AND its subfolders, no direct-only mode. A toggle
	/// is maybe-later — the LrC record shows the buried mode switch is the
	/// pain, not either behavior.
	case folder(Identifier<Folder>)
	/// One specific historical import. No sidebar row yet; a browsing UI
	/// for import history is a future round.
	case `import`(Identifier<Import>)
	/// "Previous Import": the most recent import, resolved at fetch time so
	/// each new import replaces the answer (LrC's settled meaning, ratified
	/// 2026-09-11) — and because the compiled statement reads the imports
	/// table, the observation re-delivers the moment a new import begins.
	case latestImport
	/// An authored collection: the union view by ruling (collections round,
	/// 2026-09-12) — the collection's own members AND everything beneath
	/// its child collections, the folder subtree stance again.
	case collection(Identifier<Collection>)
}

/// The ORDER BY: sort key + direction. Orders the working set, never
/// changes membership. One key today — the sort vocabulary grows in its
/// own round; grouping (sections) is deliberately unsettled.
nonisolated struct Arrangement: Hashable, Sendable {
	enum SortKey: String, Sendable, CaseIterable, Identifiable {
		/// Catalog-entry order at millisecond granularity: ids are UUIDv7
		/// (ms timestamp + random tail), so records minted in the same
		/// millisecond order arbitrarily — but stably, since ids persist.
		case added
		/// Capture time (grid sorting round, 2026-09-15): the shutter moment
		/// (EXIF DateTimeOriginal), read through `files.capture_sort`, which
		/// COALESCEs to disk mtime, so a file with no capture evidence still
		/// sorts by when it was last written and the key is never null. An
		/// asset sorts by its representative file's key (Ari's ruling: the
		/// representative is the sorted file). Consumes direction, unlike
		/// manual.
		case captured
		/// The collection's authored order — the order_key walk (collections
		/// round, 2026-09-12). Meaningful only over a collection source (the
		/// hub's normalize rule keeps it off every other source, visibly),
		/// and it consumes NO direction: authored order has no reverse
		/// (ruled 2026-09-14; LrC's stance too).
		case manual
		
		var id: Self { self }
	}

	enum Direction: Sendable {
		case ascending
		case descending
	}

	var sortKey: SortKey = .captured
	/// Dev default: newest first.
	var direction: Direction = .descending

	var reversed: Arrangement {
		var copy = self
		copy.direction = direction == .ascending ? .descending : .ascending
		return copy
	}

	/// The posture rule (collections round): manual is only meaningful over
	/// a collection — anywhere else it falls back to .added. Direction is
	/// NEVER touched: it stays the user's property for the real sort keys
	/// (manual doesn't consume it — authored order has no reverse), so
	/// entering and leaving manual can never move a direction the user
	/// chose. Ruled 2026-09-14 after the verification review caught the
	/// earlier .ascending pin leaking out of manual: leave a collection and
	/// the library came back oldest-first, a sort nobody picked. Pure, so
	/// the rule pins synchronously; the hub applies it in every
	/// arrangement-authoring intent and owns the logging.
	func normalized(for source: Source) -> Arrangement {
		guard sortKey == .manual else { return self }
		if case .collection = source { return self }
		var fallback = self
		fallback.sortKey = .added
		return fallback
	}
	
	func setSortKey(to key: SortKey) -> Arrangement {
		var fallback = self
		fallback.sortKey = key
		return fallback
	}
}

/// A working-set member's identity, self-describing about its lens — a
/// selection held across a lens flip can never masquerade as the other
/// unit; reconciliation drops it because it matches nothing.
nonisolated enum SubjectID: Hashable, Sendable {
	case asset(Identifier<Asset>)
	case file(Identifier<File>)
}

/// The compiled question. A pure value: testable without the store, and a
/// fresh one is minted per observation swap, so an observation's query
/// never mutates under it.
nonisolated struct WorkingSetQuery: Hashable, Sendable {
	var lens: Lens
	var source: Source
	var arrangement: Arrangement
	/// The filter's clause (filter round, 2026-09-16): nil = no filter, and
	/// every statement below is byte-identical to its unfiltered form
	/// (pinned by test). The struct carries the TREE, never compiled state —
	/// compilation happens per fetch, inside this call, so the query stays
	/// Hashable and a compiled literal can never be cached stale.
	var filter: FilterGroup? = nil

	/// The working set: every id the question yields, in arrangement order.
	/// Ids only — record content is fetched by consumers on demand, so this
	/// stays cheap to re-run on every impactful commit.
	///
	/// Composition is GRDB SQL literals throughout (filter round): every
	/// bound value rides WITH its placeholder, so a spliced clause can never
	/// shift another clause's bindings — the hand-threaded String +
	/// StatementArguments form made binding order positional bookkeeping,
	/// correct only while every `?` happened to precede the filter's.
	func fetchIdentifiers(_ database: Database) throws -> [SubjectID] {
		let direction = arrangement.direction == .ascending ? "ASC" : "DESC"
		// The files lens's ORDER BY. Exhaustive so a new key can't silently
		// keep sorting by id.
		let order: SQL
		switch arrangement.sortKey {
		case .added, .manual:
			// TEXT UUIDv7: lexicographic order is chronological, at ms
			// granularity (see Arrangement.SortKey.added). manual reaches the
			// files lens only as the normalized fallback — its real walk is
			// assets-only and returns earlier — so it mirrors .added.
			order = "ORDER BY id \(sql: direction)"
		case .captured:
			// capture_sort is COALESCE(captured, mtime): total on files (mtime
			// is NOT NULL), so no null bucket here; id breaks capture ties.
			order = "ORDER BY capture_sort \(sql: direction), id \(sql: direction)"
		}

		// The filter's splice for asset memberships: wraps the membership
		// (one asset_id column) BEFORE ordering is applied — never around
		// orderedAssets, where the ORDER BY would sink into a subquery whose
		// order the outer statement doesn't contract to keep (adversarial
		// pass, B1). fm aliases the membership so it can't shadow the
		// election subqueries' rep_a/rep_f or the outer AS m.
		// PERF: the predicate columns (rating/flag/kind) are unindexed, so a
		// whole-library filter is a full column scan — fine at the 40k
		// working target by reasoning, UNTESTED at the 1M ceiling; trigger:
		// filtered library views measurably sluggish. Upgrade: index the
		// judgment columns. Also: on .library the membership IS the assets
		// table, so this wraps a whole-table pass in a PK self-join —
		// accepted for the one uniform seam over every source (the planner
		// flattens it); upgrade if it ever measures: let .library splice
		// the predicate directly.
		func filtered(_ membership: SQL) -> SQL {
			guard let filter else { return membership }
			return "SELECT fm.asset_id FROM (\(membership)) fm JOIN assets ON assets.id = fm.asset_id WHERE \(filter.sqlPredicate())"
		}

		// The asset lens orders by the SAME key, but read through each asset's
		// representative file (Ari's ruling: the representative is the sorted
		// file). Wraps a membership subquery (one asset_id column) in that order.
		func orderedAssets(_ membership: SQL) -> SQL {
			switch arrangement.sortKey {
			case .added, .manual:
				return "SELECT asset_id FROM (\(filtered(membership))) ORDER BY asset_id \(sql: direction)"
			case .captured:
				// Reuses Asset.representativeFileID (the one election) to reach
				// the representative's capture_sort; a representative-less asset
				// (no file) has a NULL key and sorts last in BOTH directions.
				// PERF: not index-served — the final sort is on capture_sort
				// reached through a per-row computed election key, so it's a
				// scan-and-sort over the membership. Fine at 40k as a
				// per-observation cost; trigger: asset-lens capture sort
				// measurably sluggish on a large library. Upgrade path: a stored
				// per-asset representative-capture column maintained on formation
				// and metadata writes.
				return """
					SELECT m.asset_id FROM (\(filtered(membership))) AS m \
					LEFT JOIN files rep ON rep.id = \(sql: Asset.representativeFileID(ofAssetID: "m.asset_id")) \
					ORDER BY rep.capture_sort IS NULL, rep.capture_sort \(sql: direction), m.asset_id \(sql: direction)
					"""
			}
		}

		// The subtree walk (ratified: a folder source reaches everything
		// beneath it): the folder plus every descendant, by parent_id.
		// UNION (not UNION ALL) so a parent_id cycle terminates instead of
		// spinning — the schema doesn't forbid one, only the write path's
		// shape. Seeded from a real record, the one subtree convention
		// (Collection.subtreeCTE is the collections twin): a ghost id
		// walks nothing, never a phantom set holding itself. The trailing
		// space is the separator for concatenation.
		func subtree(_ folder: Identifier<Folder>) -> SQL {
			"WITH RECURSIVE subtree(id) AS (SELECT id FROM folders WHERE id = \(folder) UNION "
				+ "SELECT folders.id FROM folders JOIN subtree ON folders.parent_id = subtree.id) "
		}
		// "Latest" is by started_at (ISO text: lexicographic order is
		// chronological), id as the same-millisecond tiebreak. Outcome is
		// deliberately ignored: a still-running import IS the previous
		// import, growing live.
		let latestImport = "(SELECT id FROM imports ORDER BY started_at DESC, id DESC LIMIT 1)"

		switch lens {
		case .assets:
			// Narrowing sources drive the asset lens from files, not from a
			// per-asset EXISTS over the whole assets table: measured 135x
			// cheaper on a narrowed source at 40k assets (round review,
			// 2026-09-11). `asset_id IS NOT NULL` is the formation-pending
			// exclusion the EXISTS form got implicitly.
			let sql: SQL
			switch source {
			case .library:
				sql = orderedAssets("SELECT id AS asset_id FROM assets")
			case .folder(let folder):
				sql = subtree(folder) + orderedAssets("""
					SELECT DISTINCT asset_id FROM files \
					WHERE folder_id IN subtree AND asset_id IS NOT NULL
					""")
			case .import(let run):
				sql = orderedAssets("""
					SELECT DISTINCT asset_id FROM files \
					WHERE import_id = \(run) AND asset_id IS NOT NULL
					""")
			case .latestImport:
				sql = orderedAssets("""
					SELECT DISTINCT asset_id FROM files \
					WHERE import_id = \(sql: latestImport) AND asset_id IS NOT NULL
					""")
			case .collection(let collection):
				if arrangement.sortKey == .manual {
					// The sectioned union order (ruling 5) — Swift-side,
					// because the Finder comparator can't be computed in
					// SQL. Direction is deliberately not consulted:
					// authored order IS the order (ruled 2026-09-14).
					// The filter intersects (filter round): the clause runs
					// once as its own statement and the walk drops
					// non-members — authored order preserved for what
					// survives, and the filter means the same thing it
					// means everywhere else.
					// PERF: the match set is unscoped — filtering a small
					// manual collection still fetches every matching id in
					// the catalog. Fine at 40k; trigger: filtered manual
					// views sluggish on large catalogs; upgrade: constrain
					// with AND id IN (the subtree's membership).
					var matching: Set<Identifier<Asset>>? = nil
					if let filter {
						let (matchSQL, matchArguments) = try SQL(
							"SELECT id FROM assets WHERE \(filter.sqlPredicate())"
						).build(database)
						matching = try Identifier<Asset>.fetchSet(
							database, sql: matchSQL, arguments: matchArguments
						)
					}
					return try Self.sectionedUnionOrder(of: collection, in: database, matching: matching)
						.map(SubjectID.asset)
				}
				// A regular sort key is one flat order over the union's
				// membership — never sectioned; sorting means sorting. The
				// subtree rides Collection.subtreeCTE, the concept's one
				// implementation (ruling 5's union; ghost-safe seed).
				sql = SQL(sql: Collection.subtreeCTE, arguments: [collection]) + " " + orderedAssets(
					"SELECT DISTINCT asset_id FROM collection_members WHERE collection_id IN subtree"
				)
			}
			let (statement, arguments) = try sql.build(database)
			return try Identifier<Asset>
				.fetchAll(database, sql: statement, arguments: arguments)
				.map(SubjectID.asset)

		case .files:
			// The filter under the files lens (ruled): a file matches if its
			// ASSET matches — asset-unit tokens cross through the relation.
			// Formation-pending files (asset_id NULL) fail the IN and drop
			// out of any filtered files view: intentional, test-pinned. NOTE
			// on the observed region: this subquery adds the whole `assets`
			// table to a files-lens observation's region, so judgment writes
			// re-run the fetch while a filtered files view is open —
			// required (a rating edit can change membership), and
			// removeDuplicates stops the repaint, not the work.
			let assetMatch: SQL? = filter.map {
				"files.asset_id IN (SELECT id FROM assets WHERE \($0.sqlPredicate()))"
			}
			let andAssetMatch: SQL = assetMatch.map { " AND \($0)" } ?? ""
			let sql: SQL
			switch source {
			case .library:
				if let assetMatch {
					sql = "SELECT id FROM files WHERE \(assetMatch) \(order)"
				} else {
					sql = "SELECT id FROM files \(order)"
				}
			case .folder(let folder):
				sql = subtree(folder) + "SELECT id FROM files WHERE folder_id IN subtree\(andAssetMatch) \(order)"
			case .import(let run):
				sql = "SELECT id FROM files WHERE import_id = \(run)\(andAssetMatch) \(order)"
			case .latestImport:
				sql = "SELECT id FROM files WHERE import_id = \(sql: latestImport)\(andAssetMatch) \(order)"
			case .collection:
				// Deliberately unbuilt (ruled 2026-09-14): what the files
				// lens over a collection MEANS — which files, in what order
				// — is not designed yet (_design/collections.md, ruling 10's
				// unsettled marker). Empty, never a guess.
				return []
			}
			let (statement, arguments) = try sql.build(database)
			return try Identifier<File>
				.fetchAll(database, sql: statement, arguments: arguments)
				.map(SubjectID.file)
		}
	}

	// MARK: - The sectioned union order

	/// Ruling 5 (collections round, 2026-09-12): depth-first from the
	/// source — a collection's own members first, in order_key order, then
	/// each child's subtree as a block, children sequenced by FinderOrder
	/// (the sidebar's visible sequence; ruled p0 2026-09-14, a configurable
	/// block sequence is a carried future). An asset reached twice keeps
	/// its first appearance. Stored keys are followed as bytes, not
	/// validated: a corrupt key can misorder a read but never corrupt a
	/// write — refusal lives in the verbs, guarding the mint.
	///
	/// PERF: the observed region is the WHOLE collections and
	/// collection_members tables, so a membership write anywhere in the
	/// catalog re-runs this walk for any open manual view (removeDuplicates
	/// stops the repaint, not the work), and every run rebuilds both
	/// dictionaries from scratch. The named answer is region narrowing plus
	/// incremental assembly; trigger: bulk add-to-collection measurably
	/// sluggish while a manual collection view is open.
	/// `matching` is the filter's verdict (nil = no filter): the walk keeps
	/// authored order and drops non-members. The filter check precedes the
	/// seen-insert; with per-ASSET matching the two orders are
	/// indistinguishable (an excluded asset is excluded at every
	/// appearance), but this is the shape a per-appearance predicate would
	/// need, so it's pinned here rather than rediscovered (round review,
	/// finding 11).
	private static func sectionedUnionOrder(
		of root: Identifier<Collection>, in database: Database,
		matching: Set<Identifier<Asset>>? = nil
	) throws -> [Identifier<Asset>] {
		let subtree = try Collection.fetchAll(
			database,
			sql: Collection.subtreeCTE
				+ " SELECT collections.* FROM collections JOIN subtree ON collections.id = subtree.id",
			arguments: [root]
		)

		// Memberships arrive already in authored order per collection;
		// riding the CTE binds the root id once instead of one argument
		// per subtree collection.
		let memberships = try CollectionMember.fetchAll(
			database,
			sql: Collection.subtreeCTE
				+ " SELECT * FROM collection_members WHERE collection_id IN subtree"
				+ " ORDER BY collection_id, order_key",
			arguments: [root]
		)
		var membersOf: [Identifier<Collection>: [Identifier<Asset>]] = [:]
		for membership in memberships {
			membersOf[membership.collectionId, default: []].append(membership.assetId)
		}

		var grouped: [Identifier<Collection>: [Collection]] = [:]
		for collection in subtree where collection.id != root {
			guard let parent = collection.parentId else { continue }
			grouped[parent, default: []].append(collection)
		}
		let childrenOf = grouped.mapValues { children in
			children.sorted {
				FinderOrder.ascending(
					(name: $0.name, id: $0.id.rawValue),
					(name: $1.name, id: $1.id.rawValue)
				)
			}
		}

		var ordered: [Identifier<Asset>] = []
		var seenAssets: Set<Identifier<Asset>> = []
		var visitedCollections: Set<Identifier<Collection>> = []
		func walk(_ id: Identifier<Collection>) {
			// A parent cycle can't be written (the move verb refuses), but
			// the walk terminates on one anyway — the CTE's own defense.
			guard visitedCollections.insert(id).inserted else { return }
			// Filter before seen-insert — see the doc comment above.
			for asset in membersOf[id] ?? []
			where (matching?.contains(asset) ?? true) && seenAssets.insert(asset).inserted {
				ordered.append(asset)
			}
			for child in childrenOf[id] ?? [] {
				walk(child.id)
			}
		}
		walk(root)
		return ordered
	}
}
