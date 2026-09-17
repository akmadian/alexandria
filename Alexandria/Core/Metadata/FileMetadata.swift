//
//  FileMetadata.swift
//  Alexandria
//

import Foundation

/// Import-critical extracted metadata, composed of FACETS (metadata modeling
/// round, 2026-09-17): sections by overlapping nature, never by file kind.
/// Kind routes the extractor; EVIDENCE decides which facets are present — a
/// silent screen recording has no `audio`, a scan has no `capture`. nil = the
/// file doesn't say, for facets and fields alike. An all-nil facet normalizes
/// to nil before storage so equal metadata is equal bytes.
///
/// Serialized into `files.metadata` as sectioned snake_case JSON; every key
/// is spelled in an explicit CodingKeys table because the keys are a storage
/// contract — generated columns (`files.capture_sort`) and future promotions
/// read them by path, so they never move. They double as stable field
/// identifiers (`capture.iso`) for future per-kind display configuration.
///
/// The long-tail store is deliberately absent: the catalog stores the
/// queryable surface only; a future on-demand exiftool lane answers the deep
/// view (deliberately unsettled: stored-tail cache, revisit only on
/// offline-deep-metadata or tail-search evidence).
nonisolated struct FileMetadata: Codable, Equatable, Sendable {
	var visual: VisualFacet?
	var timing: TimingFacet?
	var capture: CaptureFacet?
	var media: MediaFacet?
	var audio: AudioFacet?
	var authorship: AuthorshipFacet?
	var location: LocationFacet?

	/// The facet keys are storage contract like every field key below them —
	/// `capture_sort` reads `$.capture.captured_at` by this spelling. A
	/// property rename must not be able to move a blob key silently.
	enum CodingKeys: String, CodingKey {
		case visual, timing, capture, media, audio, authorship, location
	}

	var isEmpty: Bool { normalized() == FileMetadata() }

	// PERF: the blob is text JSON; if promoted generated columns multiply,
	// each one re-parses it per row write — SQLite JSONB (jsonb() at write)
	// removes the re-tokenizing. Trigger: a second or third promoted column.

	/// The storage form: every empty facet becomes nil, so `{}` never lands
	/// in a blob and byte-determinism holds. Extractors return through this.
	func normalized() -> FileMetadata {
		FileMetadata(
			visual: visual?.normalized(),
			timing: timing?.normalized(),
			capture: capture?.normalized(),
			media: media?.normalized(),
			audio: audio?.normalized(),
			authorship: authorship?.normalized(),
			location: location?.normalized()
		)
	}

	/// Bumped when the facet rosters change shape: the re-extraction
	/// worklist key (`files.metadata_version` records the writer's version).
	static let currentVersion = 1
}

// MARK: - Facets

/// A facet: one overlapping nature a file can have. Presence is evidence.
/// Conformers get `isEmpty` (all-nil) and `normalized()` (nil when empty)
/// once, here — never hand-rolled per facet.
nonisolated protocol MetadataFacet: Codable, Equatable, Sendable {
	init()
}

nonisolated extension MetadataFacet {
	var isEmpty: Bool { self == Self() }
	func normalized() -> Self? { isEmpty ? nil : self }
}

/// Pixel truth of anything drawn: images and video frames. Dimensions are
/// ENCODED dimensions — the file's testimony, the industry convention
/// (exiftool/MediaInfo/digiKam all store encoded + rotation separately) —
/// with `orientation` as the transform onto the upright picture; display
/// size derives, never the reverse. The observed value; a user-editable
/// orientation override is a later concept, never a rewrite of this one.
nonisolated struct VisualFacet: MetadataFacet {
	var width: Int?
	var height: Int?
	var orientation: ExifOrientation?
	var colorSpace: String?
	var bitsPerSample: Int?
	// "RGB", "CMYK", "Gray" — ImageIO's ColorModel vocabulary. Image-only
	// for now: video color description is a different vocabulary
	// (primaries/transfer) and lands here only when that's designed
	// (deliberately unsettled, with the HDR flag).
	var colorModel: String?

	enum CodingKeys: String, CodingKey {
		case width, height, orientation
		case colorSpace = "color_space"
		case bitsPerSample = "bits_per_sample"
		case colorModel = "color_model"
	}
}

/// Timed media: video and audio. One field today, its own facet on purpose —
/// duration is a first-class user concept and `$.timing.duration_seconds`
/// must not live under a codec-tech section.
nonisolated struct TimingFacet: MetadataFacet {
	var durationSeconds: Double?

	enum CodingKeys: String, CodingKey {
		case durationSeconds = "duration_seconds"
	}
}

/// Facts about the creation event THE FILE STATES (ruling 2026-09-17):
/// `capturedAt` alone admits a file — camera identity is optional within the
/// facet, so screen recordings and copied files keep a real capture time and
/// the capture sort never falls back to copy-time mtime for them.
nonisolated struct CaptureFacet: MetadataFacet {
	var make: String?
	var model: String?
	var serialNumber: String?    // EXIF BodySerialNumber; MakerNote-only serials wait for the exiftool lane
	var lensModel: String?
	var focalLength: Double?     // millimeters
	var focalLength35mm: Int?    // 35mm-equivalent millimeters
	var aperture: Double?        // f-number
	var exposureSeconds: Double? // numeric so "faster than 1/500" can filter; display derives
	var exposureBias: Double?    // EV compensation as dialed
	var iso: Int?
	var flash: Bool?             // fired; nil = no flash function / not stated
	// Mapped EXIF code names (the colorSpace precedent): "Aperture priority",
	// "Pattern", "Auto" — unknown codes yield nil. Expectation-parity fields
	// (ruling 2026-09-17): LrC's panel and digiKam's columns both carry them.
	var exposureProgram: String?
	var meteringMode: String?
	var whiteBalance: String?
	// Wall-clock, UTC-labelled, millisecond precision (SubSecTimeOriginal
	// folded in — burst order is real). The zone lives in captureOffset so
	// the capture-time sort key is one scale across files with and without
	// an offset.
	var capturedAt: Date?
	var captureOffset: String?   // "-05:00"; nil = zone unknown, from any cause

	enum CodingKeys: String, CodingKey {
		case make, model, aperture, iso, flash
		case serialNumber = "serial_number"
		case lensModel = "lens_model"
		case focalLength = "focal_length"
		case focalLength35mm = "focal_length_35mm"
		case exposureSeconds = "exposure_seconds"
		case exposureBias = "exposure_bias"
		case exposureProgram = "exposure_program"
		case meteringMode = "metering_mode"
		case whiteBalance = "white_balance"
		case capturedAt = "captured_at"
		case captureOffset = "capture_offset"
	}
}

/// Container/stream tech of AV media. Stream sound facts live here beside
/// their video twins (frame_rate ↔ sample_rate) — the `audio` facet is for
/// tags, not streams.
nonisolated struct MediaFacet: MetadataFacet {
	var videoCodec: String?
	var audioCodec: String?
	var frameRate: Double?
	var sampleRate: Int?     // Hz
	var channelCount: Int?

	enum CodingKeys: String, CodingKey {
		case videoCodec = "video_codec"
		case audioCodec = "audio_codec"
		case frameRate = "frame_rate"
		case sampleRate = "sample_rate"
		case channelCount = "channel_count"
	}
}

/// Embedded tags of standalone audio files (ID3/iTunes/Vorbis lineage).
/// Named `audio`, not `music` — not all audio files are music (ruling
/// 2026-09-17). Artist/title normalization is parked with the editorial
/// fields (see AuthorshipFacet).
nonisolated struct AudioFacet: MetadataFacet {
	var album: String?
	var trackNumber: Int?
	var genre: String?
	var composer: String?

	enum CodingKeys: String, CodingKey {
		case album, genre, composer
		case trackNumber = "track_number"
	}
}

/// Editorial fields, DUAL-SOURCE by ruling (2026-09-17): the TIFF (device/
/// legacy) and IPTC (human/workflow) variants stored side by side, LrC-style
/// — no collapsing, no precedence minted. A normalized layer and XMP-only
/// sources belong to the parked source-normalization round.
nonisolated struct AuthorshipFacet: MetadataFacet {
	var tiffArtist: String?
	var tiffImageDescription: String?
	var tiffCopyright: String?
	var tiffSoftware: String?     // provenance: what app wrote the file (LrC shows it)
	var iptcCreator: String?      // IPTC By-line
	var iptcCaption: String?      // IPTC Caption/Abstract
	var iptcTitle: String?        // IPTC ObjectName
	var iptcCopyright: String?    // IPTC CopyrightNotice

	enum CodingKeys: String, CodingKey {
		case tiffArtist = "tiff_artist"
		case tiffImageDescription = "tiff_image_description"
		case tiffCopyright = "tiff_copyright"
		case tiffSoftware = "tiff_software"
		case iptcCreator = "iptc_creator"
		case iptcCaption = "iptc_caption"
		case iptcTitle = "iptc_title"
		case iptcCopyright = "iptc_copyright"
	}
}

/// Where the content was made, when geo-tagged. Signed decimal degrees;
/// altitude signed meters (below sea level is negative).
nonisolated struct LocationFacet: MetadataFacet {
	var latitude: Double?
	var longitude: Double?
	var altitude: Double?

	enum CodingKeys: String, CodingKey {
		case latitude, longitude, altitude
	}
}

/// EXIF orientation 1–8: the transform from encoded pixels onto the upright
/// picture. One domain for both families — video's preferredTransform maps
/// onto the same codes at extraction.
nonisolated enum ExifOrientation: Int, Codable, Equatable, Sendable {
	case up = 1
	case upMirrored = 2
	case down = 3
	case downMirrored = 4
	case leftMirrored = 5
	case right = 6
	case rightMirrored = 7
	case left = 8

	/// Lenient admission for observed codes: out-of-range values (some tools
	/// write 0) are "the file doesn't say", never an error.
	init?(code: Int?) {
		guard let code, let value = ExifOrientation(rawValue: code) else { return nil }
		self = value
	}

	/// Whether the upright picture swaps the encoded axes (90°/270° family).
	var swapsAxes: Bool {
		switch self {
		case .leftMirrored, .right, .rightMirrored, .left: true
		default: false
		}
	}
}

// MARK: - Storage form

nonisolated extension FileMetadata {
	/// The `files.metadata` column form: deterministic (sorted keys, explicit
	/// spelled-out snake_case CodingKeys), dates in `catalogDateFormatter`'s
	/// millisecond ISO 8601 — the catalog's one ratified timestamp form, so
	/// blob dates and column dates share a scale and lexicographic order is
	/// chronological everywhere. Encoder and decoder move together, always.
	func databaseJSON() throws -> String {
		let encoder = JSONEncoder()
		encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
		encoder.dateEncodingStrategy = .formatted(catalogDateFormatter)
		return String(decoding: try encoder.encode(normalized()), as: UTF8.self)
	}

	/// databaseJSON()'s round trip: reads the column form back. nil for
	/// undecodable content — consumers (formation's evidence guards) treat
	/// missing metadata as absent evidence, never as an error. Unknown keys
	/// are ignored by Codable, so newer rosters never break older readers.
	init?(databaseJSON: String) {
		let decoder = JSONDecoder()
		decoder.dateDecodingStrategy = .formatted(catalogDateFormatter)
		guard let decoded = try? decoder.decode(FileMetadata.self, from: Data(databaseJSON.utf8)) else {
			return nil
		}
		self = decoded
	}
}

// MARK: - Extraction contract

/// One registry capability: reads normalized metadata from a file on disk.
/// Best-effort inside — a corrupt metadata block yields partial data, never
/// a stop. A throw means the source itself was unreadable: the caller
/// records a DLQ row and the file still indexes.
nonisolated protocol MetadataExtracting: Sendable {
	func extract(from url: URL) async throws -> FileMetadata
}

nonisolated enum MetadataExtractionError: Error {
	case unreadableSource(URL)
}
