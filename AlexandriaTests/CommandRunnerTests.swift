//
//  CommandRunnerTests.swift
//  AlexandriaTests
//
//  Dispatch's contract (keybinding round, 2026-09-14): canPerform is the one
//  gate — context plus precondition — and perform lands on the real verbs and
//  intents through a real hub over an in-memory catalog.
//

import Foundation
import Testing
import GRDB
@testable import Alexandria

/// Three assets, a hub that has answered the library question, and a runner
/// over both.
private struct RunnerFixture {
	let catalog: Catalog
	let hub: CatalogViewState
	let runner: CommandRunner
	let assets: [Identifier<Asset>]

	@MainActor
	static func make() async throws -> RunnerFixture {
		let catalog = try Catalog(DatabaseQueue())
		let a = try await seedAsset(catalog, at: 1_000)
		let b = try await seedAsset(catalog, at: 2_000)
		let c = try await seedAsset(catalog, at: 3_000)
		let hub = CatalogViewState(catalog: catalog)
		let runner = CommandRunner(catalog: catalog, viewState: hub)
		try await eventually("initial delivery") { hub.workingSet.count == 3 }
		return RunnerFixture(catalog: catalog, hub: hub, runner: runner, assets: [a, b, c])
	}

	/// The committed judgments, read back the way any other observer sees
	/// them — the verbs commit through a Task, so callers poll.
	func judgments(
		of ids: [Identifier<Asset>]
	) async throws -> [(rating: Int?, flag: Asset.Flag?)] {
		try await catalog.databaseWriter.read { database in
			try ids.compactMap { try Asset.fetchOne(database, key: $0) }
				.map { (rating: $0.rating, flag: $0.flag) }
		}
	}
}

@MainActor
struct CommandRunnerTests {

	// MARK: The gate

	/// A judgment with nothing to judge is refused — and "nothing" means an
	/// empty working set, since the cursor stands in for an empty selection.
	/// View commands never depended on a target.
	@Test func judgmentsAreRefusedWithNothingToJudge() async throws {
		let catalog = try Catalog(DatabaseQueue())
		let hub = CatalogViewState(catalog: catalog)
		let runner = CommandRunner(catalog: catalog, viewState: hub)
		try await eventually("empty answer") { hub.answeredQuery != nil }

		#expect(hub.judgmentTargets.isEmpty)
		#expect(runner.canPerform(.rate3) == false)
		#expect(runner.canPerform(.unrate) == false)
		#expect(runner.canPerform(.pick) == false)
		#expect(runner.canPerform(.showLoupe))
	}

	@Test func selectingMakesJudgmentsPerformable() async throws {
		let fixture = try await RunnerFixture.make()
		fixture.hub.setSelection([.asset(fixture.assets[0])])
		#expect(fixture.runner.canPerform(.rate3))
		#expect(fixture.runner.canPerform(.unflag))
	}

	/// The context gate: zoom is grid-only, so switching renderer takes it
	/// away — and the switch itself stays available in both.
	@Test func loupeTakesZoomAway() async throws {
		let fixture = try await RunnerFixture.make()
		#expect(fixture.hub.viewMode == .grid)
		#expect(fixture.runner.canPerform(.zoomIn))

		fixture.runner.perform(.showLoupe)
		#expect(fixture.hub.viewMode == .loupe)
		#expect(fixture.runner.canPerform(.zoomIn) == false)
		#expect(fixture.runner.canPerform(.zoomOut) == false)
		#expect(fixture.runner.canPerform(.showGrid))

		fixture.runner.perform(.showGrid)
		#expect(fixture.hub.viewMode == .grid)
		#expect(fixture.runner.canPerform(.zoomIn))
	}

	/// The zoom precondition is the hub's own clamp range, asked before the
	/// intent rather than discovered by a no-op.
	@Test func zoomIsRefusedAtTheEndsOfTheRange() async throws {
		let fixture = try await RunnerFixture.make()
		let range = CatalogViewState.gridColumnRange

		// Zooming IN means fewer, larger cells (the toolbar's sense).
		fixture.hub.setGridColumns(range.lowerBound)
		#expect(fixture.runner.canPerform(.zoomIn) == false)
		#expect(fixture.runner.canPerform(.zoomOut))

		fixture.hub.setGridColumns(range.upperBound)
		#expect(fixture.runner.canPerform(.zoomIn))
		#expect(fixture.runner.canPerform(.zoomOut) == false)
	}

	// MARK: Dispatch

	@Test func ratingLandsOnEverySelectedAsset() async throws {
		let fixture = try await RunnerFixture.make()
		let selected = Array(fixture.assets.prefix(2))
		fixture.hub.setSelection(Set(selected.map(SubjectID.asset)))

		fixture.runner.perform(.rate3)
		try await eventually("rating committed") {
			try await fixture.judgments(of: selected).allSatisfy { $0.rating == 3 }
		}
		// And only the selection: the third asset was never touched, even
		// though it is the cursor.
		let untouched = try await fixture.judgments(of: [fixture.assets[2]])
		#expect(untouched.first?.rating == nil)

		fixture.runner.perform(.unrate)
		try await eventually("rating cleared") {
			try await fixture.judgments(of: selected).allSatisfy { $0.rating == nil }
		}
	}

	@Test func flaggingAndUnflaggingLandOnTheSelection() async throws {
		let fixture = try await RunnerFixture.make()
		fixture.hub.setSelection(Set(fixture.assets.map(SubjectID.asset)))

		fixture.runner.perform(.pick)
		try await eventually("pick committed") {
			try await fixture.judgments(of: fixture.assets).allSatisfy { $0.flag == .pick }
		}

		fixture.runner.perform(.reject)
		try await eventually("reject committed") {
			try await fixture.judgments(of: fixture.assets).allSatisfy { $0.flag == .reject }
		}

		fixture.runner.perform(.unflag)
		try await eventually("flag cleared") {
			try await fixture.judgments(of: fixture.assets).allSatisfy { $0.flag == nil }
		}
	}

	/// With nothing selected the cursor stands in — the judgment lands on it
	/// alone, which is the whole point of the fallback.
	@Test func judgingWithNoSelectionLandsOnTheCursor() async throws {
		let fixture = try await RunnerFixture.make()
		#expect(fixture.hub.selection.isEmpty)
		// Newest first: the cursor is the last-seeded asset.
		let cursorAsset = fixture.assets[2]
		#expect(fixture.hub.cursor == .asset(cursorAsset))

		fixture.runner.perform(.rate5)
		try await eventually("cursor rated") {
			try await fixture.judgments(of: [cursorAsset]).first?.rating == 5
		}
		let others = try await fixture.judgments(of: Array(fixture.assets.prefix(2)))
		#expect(others.allSatisfy { $0.rating == nil })
	}

	@Test func zoomMovesGridColumnsBothWays() async throws {
		let fixture = try await RunnerFixture.make()
		let start = fixture.hub.gridColumns

		fixture.runner.perform(.zoomIn)
		#expect(fixture.hub.gridColumns == start - 1)

		fixture.runner.perform(.zoomOut)
		#expect(fixture.hub.gridColumns == start)
	}

}
