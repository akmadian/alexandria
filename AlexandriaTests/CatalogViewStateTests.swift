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

/// Two tracked roots on one volume: the context's root holds two files
/// under its import, "Second" holds one file under a second import — so
/// every (lens, source) pair answers with a distinguishable working set.
private struct TwoFolderFixture {
	let context: ImportContext
	let folderB: Identifier<Folder>
	let importB: Identifier<Import>

	static func make() async throws -> TwoFolderFixture {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/a.jpg"),
			context.prepared("/Volumes/Test/Shoot/b.jpg"),
		])
		try await context.form()

		let volumeId = try await context.catalog.databaseWriter.read {
			try Volume.fetchAll($0).first!.id
		}
		let folderB = try await context.catalog.findOrCreateRootFolder(
			named: "Second", on: volumeId, rootPath: "Second"
		)
		let importB = Identifier<Import>.mint()
		try await context.catalog.recordImportStarted(id: importB, folderId: folderB)
		try await context.catalog.recordNewFileBatch(
			[context.prepared("/Volumes/Test/Second/c.jpg")],
			importId: importB, rootFolderId: folderB,
			rootUrl: URL(fileURLWithPath: "/Volumes/Test/Second")
		)
		let files = try await context.catalog.files(inImport: importB)
		try await context.catalog.recordFormedAssets(AssetFormation.form(files: files).clusters)
		return TwoFolderFixture(context: context, folderB: folderB, importB: importB)
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
			(.assets, .library, 3),
			(.files, .library, 3),
			(.assets, .folder(folderA), 2),
			(.files, .folder(folderA), 2),
			(.assets, .folder(fixture.folderB), 1),
			(.files, .folder(fixture.folderB), 1),
			(.assets, .import(importA), 2),
			(.files, .import(importA), 2),
			(.assets, .import(fixture.importB), 1),
			(.files, .import(fixture.importB), 1),
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
		// chain has a distinguishable answer — (3 assets), (3 files),
		// (2 files), (1 file) — so ANY superseded answer landing late is
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
}
