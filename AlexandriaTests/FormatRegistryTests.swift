//
//  FormatRegistryTests.swift
//  AlexandriaTests
//

import Foundation
import UniformTypeIdentifiers
import Testing
@testable import Alexandria

/// The registry's own fences: the table stays unambiguous and resolution
/// stays total.
struct FormatRegistryTests {

	@Test func tableClaimsEachExtensionExactlyOnceAndNormalized() {
		var seen: Set<String> = []
		for format in FileFormat.all {
			#expect(!format.extensions.isEmpty, "table rows must be reachable by extension")
			for ext in format.extensions {
				#expect(ext == FileFormat.normalize(extension: ext), "unnormalized key: \(ext)")
				#expect(seen.insert(ext).inserted, "extension claimed twice: \(ext)")
			}
		}
	}

	@Test func knownFormatsResolveByExtensionRegardlessOfSpelling() {
		for spelling in ["raf", "RAF", ".raf", ".RAF"] {
			let format = FileFormat.resolve(extension: spelling, contentType: nil)
			#expect(format.extensions.contains("raf"), "failed spelling: \(spelling)")
		}
		#expect(FileFormat.resolve(extension: "xmp", contentType: nil).kind == .sidecar)
		#expect(FileFormat.resolve(extension: "MOV", contentType: nil).kind == .video)
	}

	@Test func rawFormatsAreImagesNotTheirOwnKind() {
		for rawExtension in ["cr2", "cr3", "nef", "arw", "dng", "orf", "raf", "rw2"] {
			let format = FileFormat.resolve(extension: rawExtension, contentType: nil)
			#expect(format.kind == .image, "\(rawExtension) should classify as image")
		}
	}

	@Test func strangersWithRecognizableContentFallToTheirFamily() {
		// Real formats with no table row: kind still resolves via UTType conformance.
		#expect(FileFormat.resolve(extension: "ico", contentType: .ico) == .genericImage)
		#expect(FileFormat.resolve(extension: "aiff", contentType: .aiff) == .genericAudio)
	}

	@Test func totalStrangersLandOnTheUniversalFloor() {
		let format = FileFormat.resolve(extension: "zzz", contentType: nil)
		#expect(format == .unrecognized)
		#expect(format.kind == .other)
	}
}
