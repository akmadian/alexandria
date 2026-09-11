//
//  MetadataExtractionTests.swift
//  AlexandriaTests
//
//  Runs against the real camera files in testdata/ at the repo root.
//

import Foundation
import Testing
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

	@Test func cameraJPEGYieldsFirstClassFields() async throws {
		let metadata = try await ImagePropertiesExtractor()
			.extract(from: testData.appending(path: "exif-original.JPG"))
		#expect(metadata.width == 7728)
		#expect(metadata.height == 5152)
		#expect(metadata.cameraMake == "FUJIFILM")
		#expect(metadata.cameraModel == "X-T5")
		#expect(metadata.orientation == 1)
		#expect(metadata.capturedAt != nil)
		#expect(metadata.iso != nil)
		#expect(metadata.aperture != nil)
		#expect(metadata.shutterSpeed != nil)
	}

	@Test func rawFileYieldsSameFirstClassFieldsAsItsJPEG() async throws {
		// The headline claim of the platform switch: one extractor, RAW included.
		let metadata = try await ImagePropertiesExtractor()
			.extract(from: testData.appending(path: "_DSF0796.RAF"))
		#expect(metadata.cameraMake == "FUJIFILM")
		#expect(metadata.cameraModel == "X-T5")
		#expect(metadata.width != nil)
		#expect(metadata.capturedAt != nil)
	}

	@Test func olympusRawExtracts() async throws {
		let metadata = try await ImagePropertiesExtractor()
			.extract(from: testData.appending(path: "olympus-raw_6150009.ORF"))
		#expect(metadata.cameraMake != nil)
		#expect(metadata.capturedAt != nil)
	}

	@Test func paddedASCIIFieldsAreTrimmed() async throws {
		// The TG-7 writes "TG-7            " — padding must not reach the catalog.
		let metadata = try await ImagePropertiesExtractor()
			.extract(from: testData.appending(path: "real-jpg_6150009.JPG"))
		#expect(metadata.cameraModel == "TG-7")
		#expect(metadata.cameraMake == "OM Digital Solutions")
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

	// MARK: Video family

	@Test func videoYieldsDurationAndDisplayDimensions() async throws {
		let metadata = try await VideoPropertiesExtractor()
			.extract(from: testData.appending(path: "video-tiny.mp4"))
		let duration = try #require(metadata.durationSeconds)
		#expect(duration > 0)
		#expect(metadata.width != nil)
		#expect(metadata.height != nil)
	}

	@Test func rotatedVideoReportsUprightDimensions() async throws {
		let extractor = VideoPropertiesExtractor()
		let plain = try await extractor.extract(from: testData.appending(path: "video-tiny.mp4"))
		let rotated = try await extractor.extract(from: testData.appending(path: "video-rotated90.mp4"))
		#expect(rotated.width == plain.height)
		#expect(rotated.height == plain.width)
	}

	@Test func audioOnlyContainerHasDurationButNoDimensions() async throws {
		let metadata = try await VideoPropertiesExtractor()
			.extract(from: testData.appending(path: "video-audio-only.mp4"))
		#expect(metadata.durationSeconds != nil)
		#expect(metadata.width == nil)
		#expect(metadata.height == nil)
	}

	// MARK: Registry wiring

	@Test func imageFamilyRowsShareTheImageExtractor() {
		for ext in ["jpg", "raf", "heic", "orf"] {
			let format = FileFormat.resolve(extension: ext, contentType: nil)
			#expect(format.metadataExtractor is ImagePropertiesExtractor, "missing wiring: \(ext)")
		}
	}

	@Test func sidecarsAndTheFloorHaveNoExtractorAndThatIsFine() {
		#expect(FileFormat.resolve(extension: "xmp", contentType: nil).metadataExtractor == nil)
		#expect(FileFormat.unrecognized.metadataExtractor == nil)
	}

	// MARK: Pure helpers

	@Test func shutterSpeedRendering() {
		#expect(shutterSpeedDisplay(seconds: 0.004) == "1/250")
		#expect(shutterSpeedDisplay(seconds: 0.5) == "1/2")
		#expect(shutterSpeedDisplay(seconds: 2.5) == "2.5")
		#expect(shutterSpeedDisplay(seconds: 30) == "30")
		#expect(shutterSpeedDisplay(seconds: 0) == nil)
	}

	@Test func gpsHemisphereSigning() {
		#expect(signedCoordinate(64.13, reference: "N", negativeReference: "S") == 64.13)
		#expect(signedCoordinate(21.89, reference: "W", negativeReference: "W") == -21.89)
		#expect(signedCoordinate(nil, reference: "W", negativeReference: "W") == nil)
	}

	@Test func databaseJSONIsDeterministicAndSnakeCased() throws {
		var metadata = FileMetadata()
		metadata.cameraMake = "FUJIFILM"
		metadata.iso = 800
		let json = try metadata.databaseJSON()
		#expect(json == #"{"camera_make":"FUJIFILM","iso":800}"#)
		#expect(FileMetadata().isEmpty)
		#expect(!metadata.isEmpty)
	}
}
