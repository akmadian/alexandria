//
//  FileMetadata.swift
//  Alexandria
//

import Foundation

/// Import-critical extracted metadata: the normalized target every extractor
/// maps onto (metadata round, 2026-09-11; field list carried from the old
/// core's ruling). nil = the file doesn't say. Serialized into
/// `files.metadata` as snake_case JSON — those keys are the field catalog's
/// v0 names. The long-tail store (deny-list curated, exiftool lane) is a
/// later round.
nonisolated struct FileMetadata: Codable, Equatable, Sendable {
	// Geometry. Stored dimensions are ENCODED dimensions; orientation
	// (EXIF 1..8) is the transform onto the upright picture — they travel
	// together, or a portrait reads as landscape. This is the observed
	// value; the user-editable orientation is a later override, never a
	// rewrite of what the file said.
	var width: Int?
	var height: Int?
	var orientation: Int?

	var durationSeconds: Double?
	// EXIF timestamps carry no timezone: parsed as wall-clock, UTC-labelled,
	// so the capture time displays as the camera recorded it.
	var capturedAt: Date?

	var cameraMake: String?
	var cameraModel: String?
	var lensModel: String?
	var focalLength: Double?   // millimeters
	var aperture: Double?      // f-number
	var shutterSpeed: String?  // display form: "1/250", "2.5"
	var iso: Int?

	var gpsLatitude: Double?   // signed decimal degrees
	var gpsLongitude: Double?

	var colorSpace: String?
	var bitDepth: Int?         // bits per sample
	var creator: String?
	var copyright: String?

	var isEmpty: Bool { self == FileMetadata() }
}

nonisolated extension FileMetadata {
	/// The `files.metadata` column form: deterministic (sorted keys),
	/// snake_case, ISO 8601 dates. Dates use `.iso8601` (second precision) —
	/// deliberately coarser than the millisecond column format: EXIF capture
	/// times are second-granularity and this encoder is the blob's only
	/// writer. If sub-second capture evidence ever matters, move encode AND
	/// decode to catalogDateFormatter together — a one-sided change makes
	/// every existing blob undecodable.
	func databaseJSON() throws -> String {
		let encoder = JSONEncoder()
		encoder.keyEncodingStrategy = .convertToSnakeCase
		encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
		encoder.dateEncodingStrategy = .iso8601
		return String(decoding: try encoder.encode(self), as: UTF8.self)
	}

	/// databaseJSON()'s round trip: reads the column form back. nil for
	/// undecodable content — consumers (formation's evidence guards) treat
	/// missing metadata as absent evidence, never as an error.
	init?(databaseJSON: String) {
		let decoder = JSONDecoder()
		decoder.keyDecodingStrategy = .convertFromSnakeCase
		decoder.dateDecodingStrategy = .iso8601
		guard let decoded = try? decoder.decode(FileMetadata.self, from: Data(databaseJSON.utf8)) else {
			return nil
		}
		self = decoded
	}
}

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
