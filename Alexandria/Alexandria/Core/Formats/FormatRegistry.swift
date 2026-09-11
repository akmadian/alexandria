//
//  FormatRegistry.swift
//  Alexandria
//

import Foundation
internal import UniformTypeIdentifiers

/// One recognized file format — a row in the registry (registry round,
/// 2026-09-11). The table is the single source of truth for what Alexandria
/// knows about each format; everything else is generic code reading rows.
/// Add a format = add a row, nowhere else.
///
/// A format's KIND is the coarse class recorded on file rows; the asset's
/// kind is derived later, at formation, from its member files — the registry
/// never speaks about assets. Capability columns (metadata extraction,
/// thumbnailing, pairing class) join with their consuming rounds; a missing
/// capability degrades gracefully — skip the work, show the generic card,
/// never error.
nonisolated struct FileFormat: Hashable, Sendable {
	/// Dispatch keys: lowercase, no dot. Empty only for the family and floor
	/// entries, which are reached by UTType conformance, never by extension.
	let extensions: Set<String>
	/// Platform anchor where a stable constant exists; informational, not a key.
	let contentType: UTType?
	let kind: FileKind
}

// MARK: - Resolution

extension FileFormat {
	/// Total resolution: exact row by extension, else UTType family, else the
	/// universal floor. Every file gets a format; strangers get modest
	/// capabilities, never an error.
	static func resolve(extension rawExtension: String, contentType: UTType?) -> FileFormat {
		if let match = byExtension[normalize(extension: rawExtension)] {
			return match
		}
		if let contentType {
			if contentType.conforms(to: .image) { return .genericImage }
			if contentType.conforms(to: .movie) { return .genericVideo }
			if contentType.conforms(to: .audio) { return .genericAudio }
		}
		return .unrecognized
	}

	static func normalize(extension rawExtension: String) -> String {
		let trimmed = rawExtension.hasPrefix(".") ? String(rawExtension.dropFirst()) : rawExtension
		return trimmed.lowercased()
	}

	/// Conformance-reached entries: recognizably image/video/audio, no row.
	static let genericImage = FileFormat(extensions: [], contentType: .image, kind: .image)
	static let genericVideo = FileFormat(extensions: [], contentType: .movie, kind: .video)
	static let genericAudio = FileFormat(extensions: [], contentType: .audio, kind: .audio)
	/// The universal floor.
	static let unrecognized = FileFormat(extensions: [], contentType: nil, kind: .other)

	/// Traps on a duplicate extension claim — the table must stay unambiguous.
	private static let byExtension: [String: FileFormat] = Dictionary(
		uniqueKeysWithValues: all.flatMap { format in
			format.extensions.map { ($0, format) }
		}
	)
}

// MARK: - The table

extension FileFormat {
	static let all: [FileFormat] = [
		// images
		FileFormat(extensions: ["jpg", "jpeg"], contentType: .jpeg, kind: .image),
		FileFormat(extensions: ["png"], contentType: .png, kind: .image),
		FileFormat(extensions: ["gif"], contentType: .gif, kind: .image),
		FileFormat(extensions: ["webp"], contentType: .webP, kind: .image),
		FileFormat(extensions: ["tif", "tiff"], contentType: .tiff, kind: .image),
		FileFormat(extensions: ["heic"], contentType: .heic, kind: .image),
		FileFormat(extensions: ["bmp"], contentType: .bmp, kind: .image),
		// camera raw — one row per format: capabilities will differ per vendor.
		// No stable per-vendor UTType constants; rawness is a facet, kind is image.
		FileFormat(extensions: ["cr2"], contentType: nil, kind: .image),
		FileFormat(extensions: ["cr3"], contentType: nil, kind: .image),
		FileFormat(extensions: ["nef"], contentType: nil, kind: .image),
		FileFormat(extensions: ["arw"], contentType: nil, kind: .image),
		FileFormat(extensions: ["dng"], contentType: nil, kind: .image),
		FileFormat(extensions: ["orf"], contentType: nil, kind: .image),
		FileFormat(extensions: ["raf"], contentType: nil, kind: .image),
		FileFormat(extensions: ["rw2"], contentType: nil, kind: .image),
		// video
		FileFormat(extensions: ["mov"], contentType: .quickTimeMovie, kind: .video),
		FileFormat(extensions: ["mp4"], contentType: .mpeg4Movie, kind: .video),
		FileFormat(extensions: ["m4v"], contentType: nil, kind: .video),
		FileFormat(extensions: ["avi"], contentType: .avi, kind: .video),
		FileFormat(extensions: ["mkv"], contentType: nil, kind: .video),
		// audio
		FileFormat(extensions: ["mp3"], contentType: .mp3, kind: .audio),
		FileFormat(extensions: ["wav"], contentType: .wav, kind: .audio),
		FileFormat(extensions: ["flac"], contentType: nil, kind: .audio),
		FileFormat(extensions: ["aac"], contentType: nil, kind: .audio),
		FileFormat(extensions: ["m4a"], contentType: .mpeg4Audio, kind: .audio),
		// vector
		FileFormat(extensions: ["svg"], contentType: .svg, kind: .vector),
		FileFormat(extensions: ["ai"], contentType: nil, kind: .vector),
		FileFormat(extensions: ["eps"], contentType: nil, kind: .vector),
		// documents
		FileFormat(extensions: ["pdf"], contentType: .pdf, kind: .document),
		FileFormat(extensions: ["psd"], contentType: nil, kind: .document),
		FileFormat(extensions: ["indd"], contentType: nil, kind: .document),
		// editor working files
		FileFormat(extensions: ["pxd"], contentType: nil, kind: .project),
		// sidecars — companions that describe another file; their content is
		// about their subject, so no capabilities of their own.
		FileFormat(extensions: ["xmp"], contentType: nil, kind: .sidecar),
		FileFormat(extensions: ["aae"], contentType: nil, kind: .sidecar),
		FileFormat(extensions: ["thm"], contentType: nil, kind: .sidecar),
		FileFormat(extensions: ["lrv"], contentType: nil, kind: .sidecar),
		FileFormat(extensions: ["pp3"], contentType: nil, kind: .sidecar),
		FileFormat(extensions: ["dop"], contentType: nil, kind: .sidecar),
		FileFormat(extensions: ["on1"], contentType: nil, kind: .sidecar),
	]
}
