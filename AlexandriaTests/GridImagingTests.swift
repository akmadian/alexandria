//
//  GridImagingTests.swift
//  AlexandriaTests
//
//  The grid round's pinned image-path facts (2026-09-12, rebuilt clean):
//  bucket selection from geometry and the monotonic paint guard, tested with
//  no Nuke and no AppKit — the pure half, same as GridDiffTests. Plus one
//  end-to-end check that the engine decodes a local JPEG to a bucket size,
//  which is the loading seam the (skipped) spike would have proven.
//
//  The motion/settle tiering and its tests were deleted this round: one size
//  per cell, so there is no second tier to select, upgrade, or blank.
//

import AppKit
import CoreGraphics
import Foundation
import Nuke
import Testing
@testable import Alexandria

// MARK: - Bucket selection from geometry

struct DecodeBucketTests {

	private let ladder = ThumbnailStore.decodeLadder  // [128, 256, 512, 1024]

	@Test func ladderIsTheStoredJPEGsDCTScales() {
		// maxPixelSize >> k, ascending — the sizes a clean inverse-DCT yields.
		#expect(ladder.map(\.pixels) == [128, 256, 512, 1024])
		#expect(ladder.last?.pixels == ThumbnailStore.maxPixelSize)
	}

	@Test func picksSmallestRungCoveringPhysicalPixels() {
		func bucket(_ side: CGFloat, _ scale: CGFloat) -> Int {
			GridImaging.bucket(forCellSide: side, scale: scale, ladder: ladder).pixels
		}
		// 1× display: points == pixels.
		#expect(bucket(120, 1) == 128)
		#expect(bucket(128, 1) == 128)
		#expect(bucket(129, 1) == 256)
		#expect(bucket(256, 1) == 256)
		#expect(bucket(300, 1) == 512)
	}

	@Test func accountsForBackingScale() {
		func bucket(_ side: CGFloat, _ scale: CGFloat) -> Int {
			GridImaging.bucket(forCellSide: side, scale: scale, ladder: ladder).pixels
		}
		// 200pt on 2× needs 400px → 512, not the 1× 256.
		#expect(bucket(200, 2) == 512)
		#expect(bucket(200, 1) == 256)
	}

	@Test func floorsAtSmallestRungAndClampsToCeiling() {
		func bucket(_ side: CGFloat, _ scale: CGFloat) -> Int {
			GridImaging.bucket(forCellSide: side, scale: scale, ladder: ladder).pixels
		}
		#expect(bucket(40, 2) == 128)     // tiny cell → floor
		#expect(bucket(800, 2) == 1024)   // past the ceiling → clamp
		#expect(bucket(5000, 2) == 1024)
	}
}

// MARK: - The monotonic paint guard

struct GridImagingPaintTests {

	private func b(_ px: Int) -> DecodeBucket { DecodeBucket(pixels: px) }

	@Test func neverDowngradesWhatIsShown() {
		// An instant cache paint is never overwritten by nothing, and a late
		// small arrival can't overwrite a sharper image already on screen.
		#expect(GridImaging.shouldPaint(incoming: b(512), over: nil) == true)
		#expect(GridImaging.shouldPaint(incoming: b(512), over: b(256)) == true)
		#expect(GridImaging.shouldPaint(incoming: b(256), over: b(512)) == false)
		#expect(GridImaging.shouldPaint(incoming: b(512), over: b(512)) == true)
	}
}

// MARK: - Heal eligibility (the live-healing fix)

struct HealEligibilityTests {

	@Test func offersOnlyPlaceholdersWhoseBytesExistNow() {
		// The regression this pins: a visible placeholder whose thumbnail
		// isn't written yet must NOT be offered (so it isn't marked one-shot
		// and can heal when its own stamp lands).
		#expect(GridImaging.shouldOfferHeal(
			placeholder: true, inFlight: false, alreadyOffered: false, fileExists: true) == true)
		#expect(GridImaging.shouldOfferHeal(
			placeholder: true, inFlight: false, alreadyOffered: false, fileExists: false) == false)
		// Already showing pixels, already loading, or already offered → skip.
		#expect(GridImaging.shouldOfferHeal(
			placeholder: false, inFlight: false, alreadyOffered: false, fileExists: true) == false)
		#expect(GridImaging.shouldOfferHeal(
			placeholder: true, inFlight: true, alreadyOffered: false, fileExists: true) == false)
		#expect(GridImaging.shouldOfferHeal(
			placeholder: true, inFlight: false, alreadyOffered: true, fileExists: true) == false)
	}
}

// MARK: - The loading seam, end to end

/// repo-root/TestData, resolved from this source file's location.
private nonisolated var testData: URL {
	URL(fileURLWithPath: #filePath)
		.deletingLastPathComponent()
		.deletingLastPathComponent()
		.appending(path: "TestData")
}

@MainActor
struct StageImagingLoadingTests {

	/// Proves the seam the spike would have: read a local JPEG through the
	/// request's data closure, decode it to a bucket size, get pixels back —
	/// and that the long edge honors the target-size decode.
	@Test func pipelineDecodesLocalJPEGToBucketSize() async throws {
		let imaging = StageImaging()
		let jpeg = testData.appending(path: "real-jpg_6150009.JPG")
		let request = imaging.request(
			file: .mint(), fileURL: jpeg, bucket: DecodeBucket(pixels: 256), urgency: .content
		)

		let image: NSImage = try await withCheckedThrowingContinuation { continuation in
			_ = imaging.pipeline.loadImage(with: request) { result in
				continuation.resume(with: result.map(\.image))
			}
		}

		// Decoded at the bucket: the long edge is the target, not the source.
		#expect(max(image.size.width, image.size.height) <= 256)
		#expect(min(image.size.width, image.size.height) > 0)
	}

	/// Invariant 2 / "one place builds requests": the same (file, bucket)
	/// keys the cache identically regardless of urgency, and a different
	/// bucket keys differently — so a prefetch warms the exact hit a content
	/// request finds, and sizes never collide.
	@Test func sameFileAndBucketKeyIdenticallyRegardlessOfUrgency() {
		let imaging = StageImaging()
		let file = Identifier<File>.mint()
		let url = URL(fileURLWithPath: "/tmp/does-not-matter.jpg")
		let cache = imaging.pipeline.cache

		let content = imaging.request(file: file, fileURL: url, bucket: DecodeBucket(pixels: 256), urgency: .content)
		let preheat = imaging.request(file: file, fileURL: url, bucket: DecodeBucket(pixels: 256), urgency: .preheat)
		let larger = imaging.request(file: file, fileURL: url, bucket: DecodeBucket(pixels: 512), urgency: .content)

		#expect(cache.makeImageCacheKey(for: content) == cache.makeImageCacheKey(for: preheat))
		#expect(cache.makeImageCacheKey(for: content) != cache.makeImageCacheKey(for: larger))
	}
}
