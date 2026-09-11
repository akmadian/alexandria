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

extension FileMetadata {
	/// The `files.metadata` column form: deterministic (sorted keys),
	/// snake_case, ISO 8601 dates.
	func databaseJSON() throws -> String {
		let encoder = JSONEncoder()
		encoder.keyEncodingStrategy = .convertToSnakeCase
		encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
		encoder.dateEncodingStrategy = .iso8601
		return String(decoding: try encoder.encode(self), as: UTF8.self)
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
