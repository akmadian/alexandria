//
//  CatalogViewStateTests.swift
//  AlexandriaTests
//
//  The hub's contract: the working set answers the posture's question and
//  tracks commits, question intents swap the observation (last question
//  wins, and a superseded answer can never land), position reconciles
//  against every delivery, and the previous answer stands until replaced —
//  all against an in-memory catalog through the real observation machinery,
//  so the tests await deliveries rather than calling internals. The query
//  compilation itself is pinned separately over the full (lens, source)
//  matrix, without the observation in the loop.
//

import Foundation
import Testing
import GRDB
@testable import Alexandria

/// Two tracked roots on one volume: the context's root "Shoot" holds two
/// files plus one in a nested "Sub" folder, all under the context's
/// import; "Second" holds one file under a second, later import — so
/// every (lens, source) pair answers with a distinguishable working set,
/// and the subtree ruling has a nested folder to prove itself on.
private struct TwoFolderFixture {
	let context: ImportContext
	let subFolder: Identifier<Folder>
	let folderB: Identifier<Folder>
	let importB: Identifier<Import>
	/// One collection holding Second's formed asset, so the (lens, source)
	/// matrix stays exhaustive over Source.
	let collection: Identifier<Collection>

	static func make() async throws -> TwoFolderFixture {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/a.jpg"),
			context.prepared("/Volumes/Test/Shoot/b.jpg"),
			context.prepared("/Volumes/Test/Shoot/Sub/d.jpg"),
		])
		try await context.form()
		let subFolder = try await context.catalog.databaseWriter.read {
			try Folder.filter(Folder.Columns.nameKey == "Sub").fetchOne($0)!.id
		}

		let volumeId = try await context.catalog.databaseWriter.read {
			try Volume.fetchAll($0).first!.id
		}
		let folderB = try await context.catalog.findOrCreateRootFolder(
			named: "Second", on: volumeId, rootPath: "Second"
		)
		// latestImport resolves by started_at (ms-granular): make sure the
		// second import's timestamp is genuinely later.
		try await Task.sleep(for: .milliseconds(5))
		let importB = Identifier<Import>.mint()
		try await context.catalog.recordImportStarted(id: importB, folderId: folderB)
		try await context.catalog.recordNewFileBatch(
			[context.prepared("/Volumes/Test/Second/c.jpg")],
			importId: importB, rootFolderId: folderB,
			rootUrl: URL(fileURLWithPath: "/Volumes/Test/Second")
		)
		let files = try await context.catalog.files(inImport: importB)
		try await context.catalog.recordFormedAssets(AssetFormation.form(files: files).clusters)
		let memberAsset = try await context.catalog.databaseWriter.read {
			try Identifier<Asset>.fetchAll(
				$0,
				sql: "SELECT DISTINCT asset_id FROM files WHERE import_id = ? AND asset_id IS NOT NULL",
				arguments: [importB]
			).first!
		}
		let picks = try await context.catalog.createCollection(named: "Picks")
		try await context.catalog.addMembers([memberAsset], to: picks.id)
		return TwoFolderFixture(
			context: context, subFolder: subFolder, folderB: folderB, importB: importB,
			collection: picks.id
		)
	}
}

/// A collection tree that makes every ordering claim falsifiable: the
/// parent's authored order DISAGREES with added order; the children are
/// named "Shoot 9"/"Shoot 10" so Finder order (9 before 10) disagrees with
/// both lexicographic AND creation order (Shoot 10 is created first); a
/// grandchild proves depth-first (its block lands before the sibling's);
/// and one asset is a member of the parent AND a child.
private struct CollectionFixture {
	let catalog: Catalog
	let trips: Identifier<Collection>
	let loose1: Identifier<Asset>   // minted t=1000
	let loose2: Identifier<Asset>   // t=2000
	let dup: Identifier<Asset>      // t=3000; in Trips AND Shoot 9
	let nine: Identifier<Asset>     // t=4000
	let ten: Identifier<Asset>      // t=5000
	let deep: Identifier<Asset>     // t=6000; in the grandchild

	/// Ruling 5's walk over this tree, by hand: Trips' own members in
	/// authored order, then Shoot 9's block (dup already seen, then its
	/// grandchild's), then Shoot 10's.
	var sectioned: [SubjectID] {
		[loose2, dup, loose1, nine, deep, ten].map(SubjectID.asset)
	}

	static func make() async throws -> CollectionFixture {
		let catalog = try Catalog(DatabaseQueue())
		let loose1 = try await seedAsset(catalog, at: 1_000)
		let loose2 = try await seedAsset(catalog, at: 2_000)
		let dup = try await seedAsset(catalog, at: 3_000)
		let nine = try await seedAsset(catalog, at: 4_000)
		let ten = try await seedAsset(catalog, at: 5_000)
		let deep = try await seedAsset(catalog, at: 6_000)

		let trips = try await catalog.createCollection(named: "Trips")
		let shoot10 = try await catalog.createCollection(named: "Shoot 10", under: trips.id)
		let shoot9 = try await catalog.createCollection(named: "Shoot 9", under: trips.id)
		let grand = try await catalog.createCollection(named: "Grand", under: shoot9.id)
		try await catalog.addMembers([loose2, dup, loose1], to: trips.id)
		try await catalog.addMembers([dup, nine], to: shoot9.id)
		try await catalog.addMembers([deep], to: grand.id)
		try await catalog.addMembers([ten], to: shoot10.id)
		return CollectionFixture(
			catalog: catalog, trips: trips.id,
			loose1: loose1, loose2: loose2, dup: dup,
			nine: nine, ten: ten, deep: deep
		)
	}
}

@MainActor
struct CatalogViewStateTests {

	private struct PollTimeout: Error {}

	/// Polls until the hub settles into `condition` — deliveries are async
	/// main-actor hops away, so assertions on them must wait. On timeout it
	/// records the label and THROWS, stopping the test at the real failure
	/// instead of cascading into secondary ones.
	private func eventually(
		_ label: String,
		timeout: Duration = .seconds(2),
		_ condition: () -> Bool
	) async throws {
		let clock = ContinuousClock()
		let deadline = clock.now.advanced(by: timeout)
		while clock.now < deadline {
			if condition() { return }
			try await Task.sleep(for: .milliseconds(10))
		}
		Issue.record("timed out waiting for: \(label)")
		throw PollTimeout()
	}

	private var libraryQuestion: WorkingSetQuery {
		WorkingSetQuery(lens: .assets, source: .library, arrangement: Arrangement())
	}

	// MARK: The answer side

	@Test func initialDeliveryAnswersTheLibraryQuestion() async throws {
		let catalog = try Catalog(DatabaseQueue())
		let older = try await seedAsset(catalog, at: 1_000)
		let newer = try await seedAsset(catalog, at: 2_000)

		let hub = CatalogViewState(catalog: catalog)
		// Unanswered ≠ empty: no delivery can land before this main-actor
		// turn suspends, so the unanswered state is deterministic here.
		#expect(hub.answeredQuery == nil)
		#expect(hub.workingSet.isEmpty)

		try await eventually("initial delivery") {
			hub.workingSet == [.asset(newer), .asset(older)]
		}
		#expect(hub.answeredQuery == libraryQuestion)
		// The cursor exists whenever the working set is non-empty.
		#expect(hub.cursor == .asset(newer))
		#expect(hub.selection.isEmpty)
	}

	@Test func commitArrivesThroughTheObservation() async throws {
		let catalog = try Catalog(DatabaseQueue())
		_ = try await seedAsset(catalog, at: 1_000)
		let hub = CatalogViewState(catalog: catalog)
		try await eventually("initial delivery") { hub.workingSet.count == 1 }

		// The judgment loop's shape: a write lands, the answer follows —
		// the hub was never told.
		let newest = try await seedAsset(catalog, at: 2_000)
		try await eventually("commit delivery") {
			hub.workingSet.first == .asset(newest) && hub.workingSet.count == 2
		}
	}

	@Test func arrangementReversalReversesTheOrderAndTheCursorSurvives() async throws {
		let catalog = try Catalog(DatabaseQueue())
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 2_000)
		let c = try await seedAsset(catalog, at: 3_000)

		let hub = CatalogViewState(catalog: catalog)
		try await eventually("initial delivery") {
			hub.workingSet == [.asset(c), .asset(b), .asset(a)]
		}
		#expect(hub.cursor == .asset(c))

		hub.setArrangement(hub.arrangement.reversed)
		try await eventually("reversed delivery") {
			hub.workingSet == [.asset(a), .asset(b), .asset(c)]
		}
		// The cursor's subject is still a member: it survives the
		// replacement rather than falling to the new first.
		#expect(hub.cursor == .asset(c))
	}

	@Test func theAnswerStandsUntilReplaced() async throws {
		let catalog = try Catalog(DatabaseQueue())
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 2_000)
		let hub = CatalogViewState(catalog: catalog)
		try await eventually("initial delivery") {
			hub.workingSet == [.asset(b), .asset(a)]
		}

		// Between two non-empty answers the set must never be observed
		// empty — never clear-then-load. Sample every poll iteration.
		hub.setArrangement(hub.arrangement.reversed)
		let clock = ContinuousClock()
		let deadline = clock.now.advanced(by: .seconds(2))
		while clock.now < deadline {
			if hub.workingSet.isEmpty {
				Issue.record("working set observed empty mid-swap")
				throw PollTimeout()
			}
			if hub.workingSet == [.asset(a), .asset(b)] { return }
			try await Task.sleep(for: .milliseconds(1))
		}
		Issue.record("timed out waiting for: reversed delivery")
		throw PollTimeout()
	}

	@Test func refreshReasksTheCurrentQuestion() async throws {
		let catalog = try Catalog(DatabaseQueue())
		_ = try await seedAsset(catalog, at: 1_000)
		let hub = CatalogViewState(catalog: catalog)
		try await eventually("initial delivery") { hub.workingSet.count == 1 }

		// Same posture, so the question intents would no-op; refresh swaps
		// anyway and the new observation re-answers.
		hub.refresh()
		try await Task.sleep(for: .milliseconds(100))
		#expect(hub.workingSet.count == 1)
		#expect(hub.answeredQuery == libraryQuestion)
	}

	// MARK: The question — compilation over the full (lens, source) matrix

	@Test func queryCompilationAnswersEveryLensSourcePair() async throws {
		let fixture = try await TwoFolderFixture.make()
		let folderA = fixture.context.rootFolderId
		let importA = fixture.context.importId

		let cases: [(Lens, Source, Int)] = [
			(.assets, .library, 4),
			(.files, .library, 4),
			// folder(A) reaches Sub's file: the subtree ruling.
			(.assets, .folder(folderA), 3),
			(.files, .folder(folderA), 3),
			(.assets, .folder(fixture.subFolder), 1),
			(.files, .folder(fixture.subFolder), 1),
			(.assets, .folder(fixture.folderB), 1),
			(.files, .folder(fixture.folderB), 1),
			(.assets, .import(importA), 3),
			(.files, .import(importA), 3),
			(.assets, .import(fixture.importB), 1),
			(.files, .import(fixture.importB), 1),
			// importB started later, so it is the previous import.
			(.assets, .latestImport, 1),
			(.files, .latestImport, 1),
			(.assets, .collection(fixture.collection), 1),
			// Files over a collection: deliberately empty (ruled 2026-09-14;
			// pinned in depth by filesLensOverACollectionIsDeliberatelyEmpty).
			(.files, .collection(fixture.collection), 0),
		]
		for (lens, source, expected) in cases {
			let query = WorkingSetQuery(lens: lens, source: source, arrangement: Arrangement())
			let ids = try await fixture.context.catalog.databaseWriter.read {
				try query.fetchIdentifiers($0)
			}
			#expect(ids.count == expected, "\(lens)/\(source) answered \(ids.count), expected \(expected)")
			let shapeMatches = ids.allSatisfy { id in
				switch (lens, id) {
				case (.assets, .asset), (.files, .file): true
				default: false
				}
			}
			#expect(shapeMatches, "\(lens)/\(source) answered with the wrong unit")
		}
	}

	// MARK: The lens and the source, through the observation

	@Test func aNewImportBecomesThePreviousImportLive() async throws {
		let fixture = try await TwoFolderFixture.make()
		let hub = CatalogViewState(catalog: fixture.context.catalog)
		hub.setLens(.files)
		hub.setSource(.latestImport)
		try await eventually("previous import answer") { hub.workingSet.count == 1 }

		// A third import BEGINS: the observation re-delivers (the compiled
		// statement reads the imports table) and the previous import is now
		// the new, still-empty one — the hub was never told.
		try await Task.sleep(for: .milliseconds(5))
		let importC = Identifier<Import>.mint()
		try await fixture.context.catalog.recordImportStarted(
			id: importC, folderId: fixture.folderB
		)
		try await eventually("new import takes over") { hub.workingSet.isEmpty }

		// And it grows live as the import commits.
		try await fixture.context.catalog.recordNewFileBatch(
			[fixture.context.prepared("/Volumes/Test/Second/e.jpg")],
			importId: importC, rootFolderId: fixture.folderB,
			rootUrl: URL(fileURLWithPath: "/Volumes/Test/Second")
		)
		try await eventually("grows live") { hub.workingSet.count == 1 }
	}

	@Test func lensFlipAnswersWithFiles() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/a.jpg"),
			context.prepared("/Volumes/Test/Shoot/b.jpg"),
		])
		try await context.form()

		let hub = CatalogViewState(catalog: context.catalog)
		try await eventually("initial delivery") { hub.workingSet.count == 2 }
		#expect(hub.workingSet.allSatisfy { if case .asset = $0 { true } else { false } })

		hub.setLens(.files)
		try await eventually("files answer") {
			hub.workingSet.count == 2
				&& hub.workingSet.allSatisfy { if case .file = $0 { true } else { false } }
		}
	}

	@Test func sourceNarrowsMembershipAndPositionReconciles() async throws {
		let context = try await ImportContext.make()
		try await context.record([context.prepared("/Volumes/Test/Shoot/a.jpg")])
		try await context.form()
		let volumeId = try await context.catalog.databaseWriter.read {
			try Volume.fetchAll($0).first!.id
		}
		let emptyFolder = try await context.catalog.findOrCreateRootFolder(
			named: "Empty", on: volumeId, rootPath: "Empty"
		)

		let hub = CatalogViewState(catalog: context.catalog)
		try await eventually("initial delivery") { hub.workingSet.count == 1 }

		// The formed asset is reachable through its file's folder.
		hub.setSource(.folder(context.rootFolderId))
		try await eventually("folder answer") { hub.workingSet.count == 1 }

		// Select it, then look somewhere it isn't: the whole position
		// reconciles against the empty answer.
		hub.setSelection(Set(hub.workingSet))
		hub.setSource(.folder(emptyFolder))
		try await eventually("empty answer") { hub.workingSet.isEmpty }
		#expect(hub.selection.isEmpty)
		#expect(hub.cursor == nil)

		// Back to the library: the cursor falls to first again.
		hub.setSource(.library)
		try await eventually("library answer") {
			hub.cursor == hub.workingSet.first && !hub.workingSet.isEmpty
		}
	}

	@Test func rapidQuestionSwapsLandOnTheLastQuestion() async throws {
		let fixture = try await TwoFolderFixture.make()
		let hub = CatalogViewState(catalog: fixture.context.catalog)

		// Three swaps before any delivery can land. Every question in the
		// chain has a distinguishable answer — (4 assets), (4 files),
		// (3 files), (1 file) — so ANY superseded answer landing late is
		// visible, not just one of them.
		hub.setLens(.files)
		hub.setSource(.folder(fixture.context.rootFolderId))
		hub.setSource(.folder(fixture.folderB))
		try await eventually("last question's answer") {
			hub.workingSet.count == 1
				&& hub.workingSet.allSatisfy { if case .file = $0 { true } else { false } }
		}
		#expect(hub.answeredQuery == WorkingSetQuery(
			lens: .files, source: .folder(fixture.folderB), arrangement: Arrangement()
		))

		// And a superseded answer must not land late and clobber it.
		try await Task.sleep(for: .milliseconds(200))
		#expect(hub.workingSet.count == 1)
		#expect(hub.answeredQuery == WorkingSetQuery(
			lens: .files, source: .folder(fixture.folderB), arrangement: Arrangement()
		))
	}

	// MARK: The collection source (collections round, 2026-09-12)

	/// Ruling 5, pinned exactly: the sectioned depth-first walk — own
	/// members by order_key (NOT added order), child blocks in Finder
	/// sequence (NOT lexicographic, NOT creation order), subtree before
	/// sibling, duplicate at first appearance. Direction is deliberately
	/// NOT consulted (ruled 2026-09-14): authored order has no reverse,
	/// so both directions answer the same walk.
	@Test func manualUnionOrderIsTheSectionedWalk() async throws {
		let fixture = try await CollectionFixture.make()
		let ascending = WorkingSetQuery(
			lens: .assets, source: .collection(fixture.trips),
			arrangement: Arrangement(sortKey: .manual, direction: .ascending)
		)
		let descending = WorkingSetQuery(
			lens: .assets, source: .collection(fixture.trips),
			arrangement: Arrangement(sortKey: .manual, direction: .descending)
		)
		let (walked, alsoWalked) = try await fixture.catalog.databaseWriter.read {
			(try ascending.fetchIdentifiers($0), try descending.fetchIdentifiers($0))
		}
		#expect(walked == fixture.sectioned)
		#expect(alsoWalked == fixture.sectioned)
	}

	/// Sorting means sorting: a regular key over a collection source is
	/// one flat order across the union — the grandchild's newest asset
	/// lands first, interleaved across blocks, never sectioned.
	@Test func addedOnACollectionIsAFlatSortAcrossTheUnion() async throws {
		let fixture = try await CollectionFixture.make()
		let query = WorkingSetQuery(
			lens: .assets, source: .collection(fixture.trips), arrangement: Arrangement()
		)
		let ids = try await fixture.catalog.databaseWriter.read {
			try query.fetchIdentifiers($0)
		}
		let newestFirst = [fixture.deep, fixture.ten, fixture.nine, fixture.dup, fixture.loose2, fixture.loose1]
		#expect(ids == newestFirst.map(SubjectID.asset))
	}

	/// The judgment loop over manual order: a leaf collection answers in
	/// order_key order, and a reorder commit re-delivers through the
	/// observation — the hub was never told.
	@Test func manualLeafOrderFollowsOrderKeysAndReorderRedelivers() async throws {
		let catalog = try Catalog(DatabaseQueue())
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 2_000)
		let c = try await seedAsset(catalog, at: 3_000)
		let picks = try await catalog.createCollection(named: "Picks")
		try await catalog.addMembers([a, b, c], to: picks.id)

		let hub = CatalogViewState(catalog: catalog)
		hub.setSource(.collection(picks.id))
		hub.setArrangement(Arrangement(sortKey: .manual, direction: .ascending))
		try await eventually("manual answer") {
			hub.workingSet == [.asset(a), .asset(b), .asset(c)]
		}

		try await catalog.reorderMembers([c], before: a, in: picks.id)
		try await eventually("reorder delivery") {
			hub.workingSet == [.asset(c), .asset(a), .asset(b)]
		}
	}

	/// Ruled 2026-09-14: no files-lens machinery over collections yet. The
	/// members' files EXIST — the assets lens proves the membership is real
	/// — and the files lens still answers empty, pinned so the branch can't
	/// silently become a guess.
	@Test func filesLensOverACollectionIsDeliberatelyEmpty() async throws {
		let context = try await ImportContext.make()
		try await context.record([context.prepared("/Volumes/Test/Shoot/a.jpg")])
		try await context.form()
		let asset = try await context.catalog.databaseWriter.read {
			try Asset.fetchAll($0).first!.id
		}
		let picks = try await context.catalog.createCollection(named: "Picks")
		try await context.catalog.addMembers([asset], to: picks.id)

		let assets = WorkingSetQuery(
			lens: .assets, source: .collection(picks.id), arrangement: Arrangement()
		)
		let files = WorkingSetQuery(
			lens: .files, source: .collection(picks.id), arrangement: Arrangement()
		)
		let (assetIds, fileIds) = try await context.catalog.databaseWriter.read {
			(try assets.fetchIdentifiers($0), try files.fetchIdentifiers($0))
		}
		#expect(assetIds.count == 1)
		#expect(fileIds.isEmpty)
	}

	/// The normalize rule, both entry points: leaving a collection while
	/// manual falls back to .added (direction kept) where the user can see
	/// it, and manual can't be authored onto a non-collection source. The
	/// second asset stays OUT of the collection so the collection-manual
	/// answer is distinguishable from the library's — the settle-gate is
	/// on answeredQuery, never on a count a stale answer could satisfy.
	@Test func manualFallsBackToAddedOffCollectionSources() async throws {
		let catalog = try Catalog(DatabaseQueue())
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 2_000)
		let picks = try await catalog.createCollection(named: "Picks")
		try await catalog.addMembers([a], to: picks.id)

		let hub = CatalogViewState(catalog: catalog)
		hub.setSource(.collection(picks.id))
		hub.setArrangement(Arrangement(sortKey: .manual, direction: .ascending))
		try await eventually("manual answer") {
			hub.answeredQuery == WorkingSetQuery(
				lens: .assets, source: .collection(picks.id),
				arrangement: Arrangement(sortKey: .manual, direction: .ascending)
			)
		}
		#expect(hub.workingSet == [.asset(a)])

		hub.setSource(.library)
		#expect(hub.arrangement == Arrangement(sortKey: .added, direction: .ascending))
		try await eventually("library answer") {
			hub.workingSet == [.asset(a), .asset(b)]
		}
		#expect(hub.answeredQuery == WorkingSetQuery(
			lens: .assets, source: .library,
			arrangement: Arrangement(sortKey: .added, direction: .ascending)
		))

		hub.setArrangement(Arrangement(sortKey: .manual, direction: .ascending))
		#expect(hub.arrangement.sortKey == .added)
	}

	/// The posture rule, pinned pure — every cell, synchronously, no
	/// observation machinery in the loop. The intents' only job on top of
	/// this is applying it, which the hub-level tests below pin.
	@Test func arrangementNormalizationRuleCoversEveryCell() throws {
		let collection = Source.collection(Identifier<Collection>(rawValue: .v7()))
		// .added passes through untouched, everywhere.
		#expect(Arrangement().normalized(for: .library) == Arrangement())
		#expect(Arrangement().normalized(for: collection) == Arrangement())
		// Manual over a collection passes through untouched — direction
		// included: the rule never moves a direction (ruled 2026-09-14).
		#expect(
			Arrangement(sortKey: .manual, direction: .descending).normalized(for: collection)
				== Arrangement(sortKey: .manual, direction: .descending)
		)
		#expect(
			Arrangement(sortKey: .manual, direction: .ascending).normalized(for: collection)
				== Arrangement(sortKey: .manual, direction: .ascending)
		)
		// Manual over every other source falls back to .added, direction
		// kept.
		#expect(
			Arrangement(sortKey: .manual, direction: .descending).normalized(for: .library)
				== Arrangement(sortKey: .added, direction: .descending)
		)
		#expect(
			Arrangement(sortKey: .manual, direction: .ascending)
				.normalized(for: .folder(Identifier<Folder>(rawValue: .v7())))
				== Arrangement(sortKey: .added, direction: .ascending)
		)
		#expect(
			Arrangement(sortKey: .manual, direction: .descending).normalized(for: .latestImport)
				== Arrangement(sortKey: .added, direction: .descending)
		)
		#expect(
			Arrangement(sortKey: .manual, direction: .ascending)
				.normalized(for: .import(Identifier<Import>(rawValue: .v7())))
				== Arrangement(sortKey: .added, direction: .ascending)
		)
	}

	/// The verification review's composed walk, pinned (2026-09-14):
	/// direction is the USER'S property and manual never moves it — enter
	/// manual from the newest-first default, leave the collection, and the
	/// library comes back exactly as it was left, never flipped to
	/// oldest-first by a rule the user didn't invoke.
	@Test func manualNeverMovesTheUsersDirection() async throws {
		let catalog = try Catalog(DatabaseQueue())
		let a = try await seedAsset(catalog, at: 1_000)
		let picks = try await catalog.createCollection(named: "Picks")
		try await catalog.addMembers([a], to: picks.id)

		let hub = CatalogViewState(catalog: catalog)
		#expect(hub.arrangement == Arrangement(sortKey: .added, direction: .descending))

		hub.setSource(.collection(picks.id))
		hub.setArrangement(Arrangement(sortKey: .manual, direction: .descending))
		#expect(hub.arrangement == Arrangement(sortKey: .manual, direction: .descending))

		hub.setSource(.library)
		#expect(hub.arrangement == Arrangement(sortKey: .added, direction: .descending))
	}

	/// The files lens over a collection through the OBSERVATION: the empty
	/// answer delivers. (The branch runs no SQL, so the tracked region is
	/// empty and the observation never re-fires — which is correct, since
	/// the answer cannot change until the deferred ruling builds it.)
	@Test func filesLensOverACollectionDeliversEmptyThroughTheHub() async throws {
		let catalog = try Catalog(DatabaseQueue())
		let a = try await seedAsset(catalog, at: 1_000)
		let picks = try await catalog.createCollection(named: "Picks")
		try await catalog.addMembers([a], to: picks.id)

		let hub = CatalogViewState(catalog: catalog)
		hub.setLens(.files)
		hub.setSource(.collection(picks.id))
		try await eventually("empty files answer") {
			hub.answeredQuery == WorkingSetQuery(
				lens: .files, source: .collection(picks.id), arrangement: Arrangement()
			) && hub.workingSet.isEmpty
		}
	}

	/// A ghost collection id answers empty under both sort keys — never a
	/// throw, never a phantom.
	@Test func ghostCollectionSourceAnswersEmpty() async throws {
		let catalog = try Catalog(DatabaseQueue())
		_ = try await seedAsset(catalog, at: 1_000)
		let ghost = Identifier<Collection>(rawValue: .v7())
		for arrangement in [Arrangement(), Arrangement(sortKey: .manual, direction: .ascending)] {
			let query = WorkingSetQuery(
				lens: .assets, source: .collection(ghost), arrangement: arrangement
			)
			let ids = try await catalog.databaseWriter.read { try query.fetchIdentifiers($0) }
			#expect(ids.isEmpty)
		}
	}

	// MARK: The position side

	@Test func selectionAdmitsOnlyWorkingSetMembers() async throws {
		let catalog = try Catalog(DatabaseQueue())
		let known = try await seedAsset(catalog, at: 1_000)
		let hub = CatalogViewState(catalog: catalog)
		try await eventually("initial delivery") { hub.workingSet.count == 1 }

		let unknown = Identifier<Asset>(rawValue: .v7())
		hub.setSelection([.asset(known), .asset(unknown)])
		#expect(hub.selection == [.asset(known)])
	}

	@Test func cursorMovesOnlyToMembers() async throws {
		let catalog = try Catalog(DatabaseQueue())
		let older = try await seedAsset(catalog, at: 1_000)
		_ = try await seedAsset(catalog, at: 2_000)
		let hub = CatalogViewState(catalog: catalog)
		try await eventually("initial delivery") { hub.workingSet.count == 2 }

		hub.moveCursor(to: .asset(older))
		#expect(hub.cursor == .asset(older))

		hub.moveCursor(to: .asset(Identifier<Asset>(rawValue: .v7())))
		#expect(hub.cursor == .asset(older))
	}

	// MARK: Renderer posture

	/// Grid density lives in the hub (ruled 2026-09-12: launch-surviving UI
	/// state lives here for the viewpoint round's persistence to find), and
	/// its one clamp rule serves every author — slider, keys, restore.
	@Test func gridColumnsClampToTheirRange() throws {
		let hub = CatalogViewState(catalog: try Catalog(DatabaseQueue()))
		let range = CatalogViewState.gridColumnRange
		#expect(range.contains(hub.gridColumns))

		hub.setGridColumns(range.upperBound + 10)
		#expect(hub.gridColumns == range.upperBound)

		hub.setGridColumns(range.lowerBound - 10)
		#expect(hub.gridColumns == range.lowerBound)

		hub.setGridColumns(7)
		#expect(hub.gridColumns == 7)
	}
}
