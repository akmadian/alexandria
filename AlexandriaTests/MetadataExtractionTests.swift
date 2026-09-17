//
//  MetadataExtractionTests.swift
//  AlexandriaTests
//
//  Runs against the real camera files in TestData/ at the repo root.
//

import Foundation
import Testing
import GRDB
@testable import Alexandria

struct MetadataExtractionTests {

	/// repo-root/TestData, resolved from this source file's location.
	private var testData: URL {
		URL(fileURLWithPath: #filePath)
			.deletingLastPathComponent()   // AlexandriaTests/
			.deletingLastPathComponent()   // repo root
			.appending(path: "TestData")
	}

	// MARK: Image family

	@Test func cameraJPEGYieldsFacetedFields() async throws {
		let metadata = try await ImagePropertiesExtractor()
			.extract(from: testData.appending(path: "exif-original.JPG"))
		let visual = try #require(metadata.visual)
		#expect(visual.width == 7728)
		#expect(visual.height == 5152)
		#expect(visual.orientation == .up)
		let capture = try #require(metadata.capture)
		#expect(capture.make == "FUJIFILM")
		#expect(capture.model == "X-T5")
		#expect(capture.capturedAt != nil)
		#expect(capture.iso != nil)
		#expect(capture.aperture != nil)
		#expect(capture.exposureSeconds != nil)
	}

	@Test func rawFileYieldsSameFirstClassFieldsAsItsJPEG() async throws {
		// The headline claim of the platform switch: one extractor, RAW included.
		let metadata = try await ImagePropertiesExtractor()
			.extract(from: testData.appending(path: "_DSF0796.RAF"))
		#expect(metadata.capture?.make == "FUJIFILM")
		#expect(metadata.capture?.model == "X-T5")
		#expect(metadata.visual?.width != nil)
		#expect(metadata.capture?.capturedAt != nil)
	}

	@Test func olympusRawExtracts() async throws {
		let metadata = try await ImagePropertiesExtractor()
			.extract(from: testData.appending(path: "olympus-raw_6150009.ORF"))
		#expect(metadata.capture?.make != nil)
		#expect(metadata.capture?.capturedAt != nil)
	}

	@Test func paddedASCIIFieldsAreTrimmed() async throws {
		// The TG-7 writes "TG-7            " — padding must not reach the catalog.
		let metadata = try await ImagePropertiesExtractor()
			.extract(from: testData.appending(path: "real-jpg_6150009.JPG"))
		#expect(metadata.capture?.model == "TG-7")
		#expect(metadata.capture?.make == "OM Digital Solutions")
	}

	@Test func truncatedFileIsBestEffortNeverACrash() async throws {
		// Corrupt input: partial data or a throw are both acceptable; a crash
		// or a hang is not.
		_ = try? await ImagePropertiesExtractor()
			.extract(from: testData.appending(path: "truncated.jpg"))
	}

	@Test func missingFileThrowsSoTheCallerCanRecordADLQRow() async {
		await #expect(throws: MetadataExtractionError.self) {
			_ = try await ImagePropertiesExtractor()
				.extract(from: testData.appending(path: "does-not-exist.jpg"))
		}
	}

	// MARK: AV family

	@Test func videoYieldsTimingVisualAndMediaFacets() async throws {
		let metadata = try await AVPropertiesExtractor()
			.extract(from: testData.appending(path: "video-tiny.mp4"))
		let duration = try #require(metadata.timing?.durationSeconds)
		#expect(duration > 0)
		#expect(metadata.visual?.width != nil)
		#expect(metadata.visual?.height != nil)
		#expect(metadata.media?.videoCodec != nil)
	}

	@Test func rotatedVideoStoresEncodedDimensionsPlusOrientation() async throws {
		// One meaning per field (metadata round, 2026-09-17): both files carry
		// the same encoded frame; the rotation lives in orientation, exactly
		// as a rotated photo stores it.
		let extractor = AVPropertiesExtractor()
		let plain = try await extractor.extract(from: testData.appending(path: "video-tiny.mp4"))
		let rotated = try await extractor.extract(from: testData.appending(path: "video-rotated90.mp4"))
		#expect(rotated.visual?.width == plain.visual?.width)
		#expect(rotated.visual?.height == plain.visual?.height)
		// Exact code, not just axis-swap: the transform matrix math is the
		// thing this fixture exists to pin. The fixture's tkhd matrix is
		// (0,-1,1,0) — 270° CW onto upright, EXIF code 8.
		#expect(rotated.visual?.orientation == .left)
	}

	@Test func audioOnlyContainerHasNoVisualFacetButHasSound() async throws {
		let metadata = try await AVPropertiesExtractor()
			.extract(from: testData.appending(path: "video-audio-only.mp4"))
		#expect(metadata.timing?.durationSeconds != nil)
		#expect(metadata.visual == nil)   // evidence decides presence
		#expect(metadata.media?.audioCodec != nil)
		#expect(metadata.media?.sampleRate != nil)
	}

	// MARK: Registry wiring

	@Test func imageFamilyRowsShareTheImageExtractor() {
		for ext in ["jpg", "raf", "heic", "orf"] {
			let format = FileFormat.resolve(extension: ext, contentType: nil)
			#expect(format.metadataExtractor is ImagePropertiesExtractor, "missing wiring: \(ext)")
		}
	}

	@Test func avFamilyRowsShareTheAVExtractor() {
		for ext in ["mov", "mp4", "mkv", "mp3", "flac", "m4a", "wav"] {
			let format = FileFormat.resolve(extension: ext, contentType: nil)
			#expect(format.metadataExtractor is AVPropertiesExtractor, "missing wiring: \(ext)")
		}
	}

	@Test func sidecarsAndTheFloorHaveNoExtractorAndThatIsFine() {
		#expect(FileFormat.resolve(extension: "xmp", contentType: nil).metadataExtractor == nil)
		#expect(FileFormat.unrecognized.metadataExtractor == nil)
	}

	// MARK: Pure helpers

	@Test func shutterSpeedSnapsToTheStandardLadderWithinTolerance() {
		#expect(shutterSpeedDisplay(seconds: 0.004) == "1/250")
		#expect(shutterSpeedDisplay(seconds: 1.0 / 101.0) == "1/100")  // APEX rational wobble
		#expect(shutterSpeedDisplay(seconds: 1.0 / 160.0) == "1/160")  // 1/3-stop mark, exact
		#expect(shutterSpeedDisplay(seconds: 0.5) == "1/2")
		#expect(shutterSpeedDisplay(seconds: 0.4) == "0.4")            // off-ladder slow fraction reads as a decimal, camera-style
		#expect(shutterSpeedDisplay(seconds: 2.5) == "2.5")
		#expect(shutterSpeedDisplay(seconds: 30) == "30")
		#expect(shutterSpeedDisplay(seconds: 0) == nil)
		#expect(shutterSpeedDisplay(seconds: .infinity) == nil)
	}

	@Test func exifCodeNamesMapLikeThePanels() {
		#expect(exposureProgramName(code: 3) == "Aperture priority")
		#expect(exposureProgramName(code: 0) == nil)   // not defined
		#expect(meteringModeName(code: 5) == "Pattern")
		#expect(meteringModeName(code: 255) == nil)    // other
		#expect(whiteBalanceName(code: 0) == "Auto")
		#expect(whiteBalanceName(code: 1) == "Manual")
		#expect(whiteBalanceName(code: nil) == nil)
	}

	@Test func flashBitfieldCollapsesHonestly() {
		#expect(flashFired(code: 1) == true)       // fired
		#expect(flashFired(code: 0) == false)      // did not fire
		#expect(flashFired(code: 0x10) == false)   // suppressed mode, not fired
		#expect(flashFired(code: 0x20) == nil)     // no flash function
		#expect(flashFired(code: nil) == nil)
	}

	@Test func gpsHemisphereSigning() {
		#expect(signedCoordinate(64.13, reference: "N", negativeReference: "S") == 64.13)
		#expect(signedCoordinate(21.89, reference: "W", negativeReference: "W") == -21.89)
		#expect(signedCoordinate(nil, reference: "W", negativeReference: "W") == nil)
		#expect(signedAltitude(86.0, reference: 1) == -86.0)  // Death Valley reads below sea level
		#expect(signedAltitude(86.0, reference: 0) == 86.0)
		#expect(signedAltitude(nil, reference: 0) == nil)
	}

	@Test func nonFiniteDoublesAreAbsentEvidenceNeverEncoderThrows() {
		#expect(double(Double.infinity as NSNumber) == nil)
		#expect(double(Double.nan as NSNumber) == nil)
		#expect(double(2.8 as NSNumber) == 2.8)
	}

	@Test func subsecondCaptureTimeFoldsIn() throws {
		let base = try #require(exifWallClockDate("2026:08:14 09:31:02"))
		let burst = try #require(exifWallClockDate("2026:08:14 09:31:02", subseconds: "570"))
		// Tolerance: Date stores seconds-since-reference, so adding 0.57 to a
		// large base costs a few ulps — the ms encoding is what must hold.
		#expect(abs(burst.timeIntervalSince(base) - 0.57) < 0.0005)
		// Garbage subseconds degrade to the whole second, never to nil.
		#expect(exifWallClockDate("2026:08:14 09:31:02", subseconds: "n/a") == base)
	}

	@Test func quickTimeCreationDateSplitsIntoWallClockAndOffset() throws {
		let (wallClock, offset) = try #require(iso8601WallClock("2026-01-12T09:31:02-0800"))
		#expect(offset == "-08:00")
		// Wall-clock: the literal digits, UTC-labelled — NOT the instant.
		let formatter = DateFormatter()
		formatter.locale = Locale(identifier: "en_US_POSIX")
		formatter.timeZone = TimeZone(identifier: "UTC")
		formatter.dateFormat = "yyyy-MM-dd HH:mm:ss"
		#expect(formatter.string(from: wallClock) == "2026-01-12 09:31:02")
		#expect(iso8601WallClock("2026-01-12T09:31:02Z")?.offset == "+00:00")
		#expect(iso8601WallClock("2026-01-12T09:31:02+05:30")?.offset == "+05:30")
		#expect(iso8601WallClock("not a date") == nil)
		// Fractional seconds must not eat the offset (review finding 3):
		// subseconds fold into the wall-clock, the zone still parses.
		let fractional = try #require(iso8601WallClock("2026-01-12T09:31:02.500-0800"))
		#expect(fractional.offset == "-08:00")
		#expect(abs(fractional.wallClock.timeIntervalSince(wallClock) - 0.5) < 0.0005)
	}

	@Test func iso6709PointFormParses() throws {
		let point = try #require(parseISO6709("+37.3349-122.0090+021.086/"))
		#expect(point.latitude == 37.3349)
		#expect(point.longitude == -122.0090)
		#expect(point.altitude == 21.086)
		let flat = try #require(parseISO6709("-33.8688+151.2093/"))
		#expect(flat.altitude == nil)
		#expect(parseISO6709("garbage") == nil)
	}

	// MARK: Storage form

	@Test func databaseJSONIsDeterministicSectionedAndExplicitlyKeyed() throws {
		// The golden pin: explicit CodingKeys spelled snake_case (digits
		// included — focal_length_35mm, the key convertToSnakeCase can't
		// write), sections sorted recursively, ms ISO 8601 dates. If this
		// test moves, a promoted column somewhere broke.
		var capture = CaptureFacet()
		capture.make = "FUJIFILM"
		capture.iso = 800
		capture.focalLength35mm = 35
		capture.capturedAt = Date(timeIntervalSince1970: 1_700_000_000.57)
		var visual = VisualFacet()
		visual.width = 100
		visual.orientation = .right
		let metadata = FileMetadata(visual: visual, capture: capture)
		let json = try metadata.databaseJSON()
		#expect(json == #"{"capture":{"captured_at":"2023-11-14T22:13:20.570Z","focal_length_35mm":35,"iso":800,"make":"FUJIFILM"},"visual":{"orientation":6,"width":100}}"#)
	}

	@Test func emptyFacetsNormalizeToNilSoBlobBytesStayCanonical() throws {
		var metadata = FileMetadata()
		metadata.visual = VisualFacet()      // all-nil facet: evidence-less
		metadata.capture = CaptureFacet()
		#expect(metadata.isEmpty)
		#expect(try metadata.databaseJSON() == "{}")
		#expect(metadata.normalized() == FileMetadata())

		var capture = CaptureFacet()
		capture.iso = 400
		metadata.capture = capture
		#expect(!metadata.isEmpty)
		#expect(try metadata.databaseJSON() == #"{"capture":{"iso":400}}"#)
	}

	@Test func blobRoundTripsByteStablyThroughTheCatalogDateForm() throws {
		var capture = CaptureFacet()
		capture.capturedAt = Date(timeIntervalSince1970: 1_700_000_000.123)
		let metadata = FileMetadata(capture: capture)
		let json = try metadata.databaseJSON()
		let decoded = try #require(FileMetadata(databaseJSON: json))
		// The storage invariant: decode → re-encode is byte-identical, ms kept.
		#expect(try decoded.databaseJSON() == json)
		#expect(json.contains(".123Z"))
	}

	@Test func unknownBlobKeysAreIgnoredSoRosterGrowthNeverBreaksReaders() {
		let futureBlob = #"{"capture":{"iso":200,"future_field":"x"},"future_facet":{"a":1}}"#
		let decoded = FileMetadata(databaseJSON: futureBlob)
		#expect(decoded?.capture?.iso == 200)
	}

	// MARK: metadata_version worklist stamp

	/// The stamp records that this roster LOOKED, not that it found anything:
	/// an evidence-less read still stamps the version (blob NULL carries the
	/// yield); 0 is reserved for never-attempted (no extractor, or it failed)
	/// so a roster bump's worklist re-reads exactly the right files.
	@MainActor
	@Test func metadataVersionStampsLookedNotFound() async throws {
		let context = try await ImportContext.make()
		var capture = CaptureFacet()
		capture.iso = 400
		try await context.record([
			context.prepared("/Volumes/Test/Shoot/evidence.jpg", metadata: FileMetadata(capture: capture)),
			context.prepared("/Volumes/Test/Shoot/bare.jpg", metadata: nil),         // read, no evidence
			context.prepared("/Volumes/Test/Shoot/notes.xmp", metadata: nil),        // no extractor wired
			context.prepared("/Volumes/Test/Shoot/broken.jpg", metadata: nil, extractionFailed: true),
		])
		let versions = try await context.catalog.reader.read { database in
			try Row.fetchAll(database, sql: "SELECT name, metadata_version FROM files ORDER BY name")
				.map { ($0["name"] as String, $0["metadata_version"] as Int) }
		}
		#expect(versions.first { $0.0 == "evidence.jpg" }?.1 == FileMetadata.currentVersion)
		#expect(versions.first { $0.0 == "bare.jpg" }?.1 == FileMetadata.currentVersion)
		#expect(versions.first { $0.0 == "notes.xmp" }?.1 == 0)
		#expect(versions.first { $0.0 == "broken.jpg" }?.1 == 0)
	}
}
