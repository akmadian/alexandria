//
//  ThumbnailTests.swift
//  AlexandriaTests
//
//  Generation runs against the real fixtures in TestData/; worklist and
//  stamping run against an in-memory catalog; the pass composition runs
//  end-to-end against a temp-directory catalog.
//

import CoreGraphics
import Foundation
import GRDB
import ImageIO
import Testing
@testable import Alexandria

/// repo-root/TestData, resolved from this source file's location.
private nonisolated var testData: URL {
	URL(fileURLWithPath: #filePath)
		.deletingLastPathComponent()   // AlexandriaTests/
		.deletingLastPathComponent()   // repo root
		.appending(path: "TestData")
}

// MARK: - Generation against real fixtures

struct ThumbnailGenerationTests {

	@Test func rawUsesEmbeddedPreviewAtFullThumbnailSize() async throws {
		let image = try await generateRawThumbnail(
			from: testData.appending(path: "_DSF0796.RAF"), maxPixelSize: 1024
		)
		// The embedded preview, capped at 1024 on the long edge — and past
		// the 512 tiny-preview guard, so no decode fallback fired.
		#expect(max(image.width, image.height) == 1024)
		#expect(min(image.width, image.height) >= 512)
	}

	@Test func rasterDecodesToFullThumbnailSizeNotTheExifThumb() async throws {
		let image = try await generateRasterThumbnail(
			from: testData.appending(path: "real-jpg_6150009.JPG"), maxPixelSize: 1024
		)
		// IfAbsent would have returned the 160×120 EXIF thumb; Always must not.
		#expect(max(image.width, image.height) == 1024)
	}

	@Test func videoThumbnailAppliesRotation() async throws {
		let image = try await generateVideoThumbnail(
			from: testData.appending(path: "video-rotated90.mp4"), maxPixelSize: 1024
		)
		// The fixture is landscape-encoded with a 90° track transform:
		// the thumbnail must come out portrait.
		#expect(image.height > image.width)
	}

	@Test func quickLookDrawsAPDF() async throws {
		let pdf = FileManager.default.temporaryDirectory
			.appending(path: "thumbnail-test-\(UUID().uuidString).pdf")
		defer { try? FileManager.default.removeItem(at: pdf) }
		var mediaBox = CGRect(x: 0, y: 0, width: 200, height: 100)
		let context = try #require(CGContext(pdf as CFURL, mediaBox: &mediaBox, nil))
		context.beginPDFPage(nil)
		context.setFillColor(CGColor(red: 1, green: 0, blue: 0, alpha: 1))
		context.fill(CGRect(x: 20, y: 20, width: 100, height: 50))
		context.endPDFPage()
		context.closePDF()

		let image = try await generateQuickLookThumbnail(from: pdf, maxPixelSize: 512)
		#expect(image.width > 0 && image.height > 0)
	}

	@Test func truncatedImageThrows() async throws {
		await #expect(throws: (any Error).self) {
			try await generateRasterThumbnail(
				from: testData.appending(path: "truncated.jpg"), maxPixelSize: 1024
			)
		}
	}

	@Test func undecodableVideoThrows() async throws {
		// hev1-tagged HEVC — the ffmpeg-minted class Apple's stack refuses.
		await #expect(throws: (any Error).self) {
			try await generateVideoThumbnail(
				from: testData.appending(path: "video-422-10bit.mov"), maxPixelSize: 1024
			)
		}
	}

	@Test func deadlineFiresAndWins() async throws {
		await #expect(throws: ThumbnailError.timedOut) {
			try await withThumbnailDeadline(.milliseconds(50)) {
				try await Task.sleep(for: .seconds(10))
			}
		}
	}

	/// The hard half of invariant 2: the deadline must return even when the
	/// racing work is synchronous and uninterruptible — abandonment, not
	/// cancellation. A sleep-based test passes without this property.
	@Test func deadlineAbandonsUninterruptibleWork() async throws {
		let started = ContinuousClock.now
		await #expect(throws: ThumbnailError.timedOut) {
			try await withThumbnailDeadline(.milliseconds(100)) {
				try await onDecodeQueue { Thread.sleep(forTimeInterval: 2) }
			}
		}
		// Returned while the decode thread is still asleep.
		#expect(ContinuousClock.now - started < .seconds(1.5))
	}
}

// MARK: - Registry/worklist coherence

/// What keeps the SQL kind filter honest and the no_thumbnailer dead-end
/// defensive rather than live: every row whose kind the worklist admits can
/// actually thumbnail, and the nil rows stay nil.
struct ThumbnailCapabilityTests {

	@Test func everyAdmittedKindRowCarriesAThumbnailer() {
		for format in FileFormat.all where FileFormat.thumbnailingKinds.contains(format.kind) {
			#expect(format.thumbnailer != nil,
			        "\(format.extensions) admits kind \(format.kind) to the worklist but cannot thumbnail")
		}
	}

	@Test func capabilityFreeRowsAndFloorsStayNil() {
		for format in FileFormat.all where !FileFormat.thumbnailingKinds.contains(format.kind) {
			#expect(format.thumbnailer == nil)
		}
		#expect(FileFormat.genericImage.thumbnailer != nil)
		#expect(FileFormat.genericVideo.thumbnailer != nil)
		#expect(FileFormat.genericAudio.thumbnailer == nil)
		#expect(FileFormat.unrecognized.thumbnailer == nil)
	}
}

// MARK: - Store

struct ThumbnailStoreTests {

	private func makeImage(width: Int, height: Int) throws -> CGImage {
		let context = try #require(CGContext(
			data: nil, width: width, height: height, bitsPerComponent: 8, bytesPerRow: 0,
			space: CGColorSpaceCreateDeviceRGB(),
			bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
		))
		context.setFillColor(CGColor(red: 0, green: 0.5, blue: 1, alpha: 1))
		context.fill(CGRect(x: 0, y: 0, width: width, height: height))
		return try #require(context.makeImage())
	}

	/// Sharding must distribute PRODUCTION ids: v7 ids share their leading
	/// characters (timestamp bits) for years, so only minted ids prove the
	/// property — a hand-written UUID certified nothing (review finding).
	@Test func mintedIdsSpreadAcrossShards() {
		let store = ThumbnailStore(catalogDirectory: URL(fileURLWithPath: "/tmp/cat"))
		let ids = (0..<200).map { _ in Identifier<File>.mint() }
		let shards = Set(ids.map {
			store.url(for: $0).deletingLastPathComponent().lastPathComponent
		})
		#expect(shards.count > 1)

		let name = ids[0].rawValue.uuidString.lowercased()
		#expect(store.url(for: ids[0]).path()
			== "/tmp/cat/Thumbnails/\(name.suffix(2))/\(name).jpg")
	}

	@Test func writesDecodableJPEGAndOverwrites() async throws {
		let directory = FileManager.default.temporaryDirectory
			.appending(path: "thumbnail-store-\(UUID().uuidString)")
		defer { try? FileManager.default.removeItem(at: directory) }
		let store = ThumbnailStore(catalogDirectory: directory)
		let id = Identifier<File>.mint()

		try await store.write(makeImage(width: 64, height: 32), for: id)
		try await store.write(makeImage(width: 48, height: 24), for: id)  // idempotent retry path

		let source = try #require(CGImageSourceCreateWithURL(store.url(for: id) as CFURL, nil))
		let image = try #require(CGImageSourceCreateImageAtIndex(source, 0, nil))
		#expect(image.width == 48 && image.height == 24)
	}
}

// MARK: - Worklist and stamping (in-memory catalog)

struct ThumbnailWorklistTests {

	@Test func worklistExcludesUnthumbnailableKindsAndKeepsRecordOrder() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/a.raf"),
			context.prepared("/Volumes/Test/Shoot/a.xmp"),   // sidecar: no capability
			context.prepared("/Volumes/Test/Shoot/track.mp3"),  // audio: no capability
			context.prepared("/Volumes/Test/Shoot/day1/b.jpg"),
		])

		// UUIDv7 tails are random within a millisecond, so id order within
		// one batch insert is arbitrary — assert membership, not sequence.
		let pending = try await context.catalog.thumbnailPending(
			inImport: context.importId, under: context.rootFolderId, limit: 10
		)
		#expect(Set(pending.map(\.file.name)) == ["a.raf", "b.jpg"])
		let raw = try #require(pending.first { $0.file.name == "a.raf" })
		let nested = try #require(pending.first { $0.file.name == "b.jpg" })
		#expect(raw.relativeDirectory == "")
		#expect(nested.relativeDirectory == "/day1")
		#expect(nested.url(under: context.rootURL).path() == "/Volumes/Test/Shoot/day1/b.jpg")
	}

	@Test func stampsAndResidueBothLeaveTheWorklist() async throws {
		let context = try await ImportContext.make()
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/a.raf"),
			context.prepared("/Volumes/Test/Shoot/b.jpg"),
			context.prepared("/Volumes/Test/Shoot/c.png"),
		])
		let pending = try await context.catalog.thumbnailPending(
			inImport: context.importId, under: context.rootFolderId, limit: 10
		)
		#expect(pending.count == 3)
		let stampTarget = try #require(pending.first { $0.file.name == "a.raf" })
		let failTarget = try #require(pending.first { $0.file.name == "b.jpg" })

		try await context.catalog.recordThumbnails(
			generated: [stampTarget.file.id],
			failures: [(failTarget.file.id, "decode_failed", "boom")]
		)
		let remaining = try await context.catalog.thumbnailPending(
			inImport: context.importId, under: context.rootFolderId, limit: 10
		)
		#expect(remaining.map(\.file.name) == ["c.png"])

		let stamped = try await context.catalog.reader.read { database in
			try Date.fetchOne(database, sql: "SELECT thumbnail_at FROM files WHERE thumbnail_at IS NOT NULL")
		}
		#expect(stamped != nil)
		let residue = try await context.catalog.reader.read { database in
			try Row.fetchOne(database, sql: "SELECT * FROM file_errors WHERE task = 'thumbnail'")
		}
		#expect(residue?["reason_code"] == "decode_failed")
	}

	@Test func missingFilesStayOffTheWorklist() async throws {
		let context = try await ImportContext.make()
		try await context.record([context.prepared("/Volumes/Test/Shoot/a.jpg")])
		try await context.catalog.databaseWriter.write { database in
			try database.execute(sql: "UPDATE files SET missing = 1")
		}
		let pending = try await context.catalog.thumbnailPending(
			inImport: context.importId, under: context.rootFolderId, limit: 10
		)
		#expect(pending.isEmpty)
	}
}

// MARK: - The pass, end to end against real fixtures

struct ThumbnailPassTests {

	/// The exact ImportRun composition: a temp-directory catalog, real
	/// fixture files recorded as an import, then generateThumbnails drains a
	/// finished stream (= the final sweep). Successes stamp and hit the
	/// store; failures land as residue; nothing stays pending.
	@MainActor
	@Test func passStampsStoresAndRecordsResidue() async throws {
		let catalogDirectory = FileManager.default.temporaryDirectory
			.appending(path: "thumbnail-pass-\(UUID().uuidString)")
		try FileManager.default.createDirectory(at: catalogDirectory, withIntermediateDirectories: true)
		defer { try? FileManager.default.removeItem(at: catalogDirectory) }
		let catalog = try Catalog.open(at: catalogDirectory)

		let fixtures = ["_DSF0796.RAF", "_DSF0796.JPG", "video-tiny.mp4",
		                "truncated.jpg", "video-422-10bit.mov"]
		let observed = try ObservedVolume(containing: testData)
		let volumeId = try await catalog.findOrCreateVolume(observed)
		let rootFolderId = try await catalog.findOrCreateRootFolder(
			named: testData.lastPathComponent, on: volumeId,
			rootPath: observed.relativePath(of: testData)
		)
		// The pass drains ITS run's import — record under run.id, or the
		// worklist is empty by design (import-scoped, ruled 2026-09-11).
		let store = ThumbnailStore(catalogDirectory: catalogDirectory)
		let run = ImportRun(
			id: .mint(), folderUrl: testData, rootFolderId: rootFolderId,
			volume: observed, catalog: catalog, store: store, resuming: false
		)
		let importId = run.id
		try await catalog.recordImportStarted(id: importId, folderId: rootFolderId)

		let prepared = await run.prepareFiles(batch: fixtures.map { name in
			let url = testData.appending(path: name)
			return DiscoveredFile(
				url: url, size: 1, modifiedAt: Date(timeIntervalSince1970: 1_700_000_000),
				format: FileFormat.resolve(extension: url.pathExtension, contentType: nil)
			)
		})
		try await catalog.recordNewFileBatch(
			prepared, importId: importId, rootFolderId: rootFolderId, rootUrl: testData
		)

		let (nudges, nudge) = AsyncStream.makeStream(of: Void.self)
		nudge.finish()
		try await run.generateThumbnails(nudges: nudges)

		let byName = try await catalog.reader.read { database in
			try Row.fetchAll(database, sql: """
				SELECT files.name, files.id, files.thumbnail_at, file_errors.reason_code
				FROM files LEFT JOIN file_errors
				  ON file_errors.file_id = files.id AND file_errors.task = 'thumbnail'
				""")
		}
		let outcomes = Dictionary(uniqueKeysWithValues: byName.map {
			($0["name"] as String, (stamped: ($0["thumbnail_at"] as String?) != nil,
			                        reason: $0["reason_code"] as String?,
			                        id: $0["id"] as String))
		})

		for succeeding in ["_DSF0796.RAF", "_DSF0796.JPG", "video-tiny.mp4"] {
			let outcome = try #require(outcomes[succeeding])
			#expect(outcome.stamped, "\(succeeding) should stamp")
			#expect(outcome.reason == nil)
			let fileId = Identifier<File>(rawValue: UUID(uuidString: outcome.id)!)
			#expect(FileManager.default.fileExists(atPath: store.url(for: fileId).path()))
		}
		for failing in ["truncated.jpg", "video-422-10bit.mov"] {
			let outcome = try #require(outcomes[failing])
			#expect(!outcome.stamped, "\(failing) should not stamp")
			#expect(outcome.reason == "decode_failed")
		}

		// Nothing left pending: the worklist is fully resolved either way.
		let pending = try await catalog.thumbnailPending(
			inImport: importId, under: rootFolderId, limit: 10
		)
		#expect(pending.isEmpty)
	}

	/// The resume ruling's load-bearing half: cancellation writes NOTHING —
	/// no stamps, no residue — so the picked-up run finds the worklist
	/// intact.
	@MainActor
	@Test func cancellationLeavesRowsPendingAndWritesNothing() async throws {
		let context = try await ImportContext.make()
		try await context.record([context.prepared("/Volumes/Test/Shoot/a.jpg")])

		let storeDirectory = FileManager.default.temporaryDirectory
			.appending(path: "thumbnail-cancel-\(UUID().uuidString)")
		defer { try? FileManager.default.removeItem(at: storeDirectory) }
		let run = ImportRun(
			id: context.importId, folderUrl: context.rootURL,
			rootFolderId: context.rootFolderId,
			volume: ObservedVolume(
				identity: .filesystemUUID("0FA1-BATCH-FIXTURE"), name: "Test",
				kind: .external, volumeRootURL: URL(fileURLWithPath: "/Volumes/Test")
			),
			catalog: context.catalog,
			store: ThumbnailStore(catalogDirectory: storeDirectory),
			resuming: true
		)

		let (nudges, nudge) = AsyncStream.makeStream(of: Void.self)
		let worker = Task { try await run.generateThumbnails(nudges: nudges) }
		worker.cancel()
		nudge.finish()
		_ = await worker.result  // CancellationError or clean exit — either way:

		let state = try await context.catalog.reader.read { database in
			try Row.fetchOne(database, sql: """
				SELECT sum(thumbnail_at IS NOT NULL) AS stamped,
				       (SELECT COUNT(*) FROM file_errors WHERE task = 'thumbnail') AS residue
				FROM files
				""")
		}
		#expect(state?["stamped"] == 0)
		#expect(state?["residue"] == 0)
	}
}
