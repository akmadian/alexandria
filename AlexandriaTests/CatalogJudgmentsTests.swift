//
//  CatalogJudgmentsTests.swift
//  AlexandriaTests
//
//  The judgments round's verbs, pinned against the real catalog
//  (2026-09-14): rating and flag set and cleared, the prior-value map undo
//  will consume, ghost ids, the empty set, the 1...5 refusal, and the
//  independence of the two columns.
//

import Foundation
import GRDB
import Testing
@testable import Alexandria

struct CatalogJudgmentsTests {

	private func makeCatalog() throws -> Catalog {
		try Catalog(DatabaseQueue(path: ":memory:"))
	}

	/// The stored record, as any reader would see it after the verb commits.
	private func record(
		_ catalog: Catalog, _ id: Identifier<Asset>
	) async throws -> Asset? {
		try await catalog.reader.read { try Asset.fetchOne($0, key: id) }
	}

	// MARK: - Ratings

	@Test func ratingIsSetChangedAndCleared() async throws {
		let catalog = try makeCatalog()
		let asset = try await seedAsset(catalog, at: 1_000)

		let firstPriors = try await catalog.setRating([asset], to: 3)
		#expect(try await record(catalog, asset)?.rating == 3)
		// The prior of an unrated asset is present-and-nil, never absent:
		// the id named a real record, so undo knows to clear it again.
		#expect(try #require(firstPriors[asset]) == nil)

		let secondPriors = try await catalog.setRating([asset], to: 5)
		#expect(try #require(secondPriors[asset]) == 3)
		#expect(try await record(catalog, asset)?.rating == 5)

		let clearedPriors = try await catalog.setRating([asset], to: nil)
		#expect(try #require(clearedPriors[asset]) == 5)
		#expect(try await record(catalog, asset)?.rating == nil)
	}

	@Test func batchRatingReturnsEveryAssetsOwnPrior() async throws {
		let catalog = try makeCatalog()
		let unrated = try await seedAsset(catalog, at: 1_000)
		let rated = try await seedAsset(catalog, at: 1_001)
		let alsoRated = try await seedAsset(catalog, at: 1_002)
		_ = try await catalog.setRating([rated], to: 2)
		_ = try await catalog.setRating([alsoRated], to: 4)

		// One gesture over a mixed set: priors differ per asset.
		let priors = try await catalog.setRating([unrated, rated, alsoRated], to: 1)
		#expect(priors.count == 3)
		#expect(try #require(priors[unrated]) == nil)
		#expect(try #require(priors[rated]) == 2)
		#expect(try #require(priors[alsoRated]) == 4)

		let ratings = try await catalog.reader.read { database in
			try Asset.fetchAll(database).map(\.rating)
		}
		#expect(ratings == [1, 1, 1])
	}

	@Test func ratingsOutsideOneToFiveAreRefusedAndWriteNothing() async throws {
		let catalog = try makeCatalog()
		let asset = try await seedAsset(catalog, at: 1_000)
		_ = try await catalog.setRating([asset], to: 3)

		// 0 is not "unrated" — clearing is nil, by ruling and by schema.
		await #expect(throws: JudgmentError.invalidRating(0)) {
			try await catalog.setRating([asset], to: 0)
		}
		await #expect(throws: JudgmentError.invalidRating(6)) {
			try await catalog.setRating([asset], to: 6)
		}
		await #expect(throws: JudgmentError.invalidRating(-1)) {
			try await catalog.setRating([asset], to: -1)
		}
		#expect(try await record(catalog, asset)?.rating == 3)
	}

	// MARK: - Flags

	@Test func flagIsSetChangedAndCleared() async throws {
		let catalog = try makeCatalog()
		let asset = try await seedAsset(catalog, at: 1_000)

		let firstPriors = try await catalog.setFlag([asset], to: .pick)
		#expect(try await record(catalog, asset)?.flag == .pick)
		#expect(try #require(firstPriors[asset]) == nil)

		let secondPriors = try await catalog.setFlag([asset], to: .reject)
		#expect(try #require(secondPriors[asset]) == .pick)
		#expect(try await record(catalog, asset)?.flag == .reject)

		let clearedPriors = try await catalog.setFlag([asset], to: nil)
		#expect(try #require(clearedPriors[asset]) == .reject)
		#expect(try await record(catalog, asset)?.flag == nil)
	}

	@Test func batchFlagReturnsEveryAssetsOwnPrior() async throws {
		let catalog = try makeCatalog()
		let unflagged = try await seedAsset(catalog, at: 1_000)
		let picked = try await seedAsset(catalog, at: 1_001)
		let rejected = try await seedAsset(catalog, at: 1_002)
		_ = try await catalog.setFlag([picked], to: .pick)
		_ = try await catalog.setFlag([rejected], to: .reject)

		let priors = try await catalog.setFlag([unflagged, picked, rejected], to: .pick)
		#expect(priors.count == 3)
		#expect(try #require(priors[unflagged]) == nil)
		#expect(try #require(priors[picked]) == .pick)
		#expect(try #require(priors[rejected]) == .reject)

		let flags = try await catalog.reader.read { database in
			try Asset.fetchAll(database).map(\.flag)
		}
		#expect(flags == [.pick, .pick, .pick])
	}

	// MARK: - Set shapes both verbs share

	@Test func ghostIdsJudgeNothingAndAreAbsentFromThePriors() async throws {
		let catalog = try makeCatalog()
		let real = try await seedAsset(catalog, at: 1_000)
		let ghost = Identifier<Asset>.mint()

		let ratingPriors = try await catalog.setRating([real, ghost], to: 4)
		#expect(ratingPriors.count == 1)
		#expect(ratingPriors[ghost] == nil)          // absent, not present-and-nil
		#expect(try #require(ratingPriors[real]) == nil)

		let flagPriors = try await catalog.setFlag([ghost], to: .reject)
		#expect(flagPriors.isEmpty)

		// Nothing was minted by judging a ghost, and the real asset took its
		// rating regardless of the ghost's company.
		let assets = try await catalog.reader.read { try Asset.fetchAll($0) }
		#expect(assets.count == 1)
		#expect(assets.first?.rating == 4)
	}

	@Test func emptyIdsJudgeNothing() async throws {
		let catalog = try makeCatalog()
		let asset = try await seedAsset(catalog, at: 1_000)
		_ = try await catalog.setRating([asset], to: 2)
		_ = try await catalog.setFlag([asset], to: .pick)

		#expect(try await catalog.setRating([], to: 5).isEmpty)
		#expect(try await catalog.setFlag([], to: .reject).isEmpty)

		let stored = try await record(catalog, asset)
		#expect(stored?.rating == 2)
		#expect(stored?.flag == .pick)
	}

	/// The two judgment columns are independent: each verb writes its own
	/// column and leaves the other exactly as it found it.
	@Test func eachJudgmentLeavesTheOtherUntouched() async throws {
		let catalog = try makeCatalog()
		let asset = try await seedAsset(catalog, at: 1_000)

		_ = try await catalog.setRating([asset], to: 4)
		_ = try await catalog.setFlag([asset], to: .pick)
		var stored = try await record(catalog, asset)
		#expect(stored?.rating == 4)
		#expect(stored?.flag == .pick)

		// Re-rating must not disturb the flag…
		_ = try await catalog.setRating([asset], to: 2)
		stored = try await record(catalog, asset)
		#expect(stored?.rating == 2)
		#expect(stored?.flag == .pick)

		// …nor re-flagging the rating, clearing included.
		_ = try await catalog.setFlag([asset], to: nil)
		stored = try await record(catalog, asset)
		#expect(stored?.rating == 2)
		#expect(stored?.flag == nil)
	}
}
