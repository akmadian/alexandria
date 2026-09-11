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
/// never speaks about assets. Capability columns join with their consuming
/// rounds; a missing capability degrades gracefully — skip the work, show
/// the generic card, never error.
nonisolated struct FileFormat: Sendable {
	/// Dispatch keys: lowercase, no dot. Empty only for the family and floor
	/// entries, which are reached by UTType conformance, never by extension.
	let extensions: Set<String>
	/// Platform anchor where a stable constant exists; informational, not a key.
	let contentType: UTType?
	let kind: FileKind
	/// Rawness is a facet, never a kind: a camera capture as opposed to a
	/// rendition. Consumed by asset formation's raw_rendition_pair rule
	/// (formation round, 2026-09-11).
	let isRawCapture: Bool
	/// Metadata extraction, import-critical. nil = no extractor yet.
	let metadataExtractor: (any MetadataExtracting)?

	init(
		extensions: Set<String>,
		contentType: UTType?,
		kind: FileKind,
		isRawCapture: Bool = false,
		metadataExtractor: (any MetadataExtracting)? = nil
	) {
		self.extensions = extensions
		self.contentType = contentType
		self.kind = kind
		self.isRawCapture = isRawCapture
		self.metadataExtractor = metadataExtractor
	}
}

// Identity is what the row IS (extensions, anchor, kind) — capability
// wiring is not identity, and existentials can't synthesize equality anyway.
nonisolated extension FileFormat: Hashable {
	static func == (lhs: FileFormat, rhs: FileFormat) -> Bool {
		lhs.extensions == rhs.extensions
			&& lhs.contentType == rhs.contentType
			&& lhs.kind == rhs.kind
	}

	func hash(into hasher: inout Hasher) {
		hasher.combine(extensions)
		hasher.combine(kind)
	}
}

// MARK: - Resolution

nonisolated extension FileFormat {
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
	static let genericImage = FileFormat(
		extensions: [], contentType: .image, kind: .image,
		metadataExtractor: ImagePropertiesExtractor()
	)
	static let genericVideo = FileFormat(
		extensions: [], contentType: .movie, kind: .video,
		metadataExtractor: VideoPropertiesExtractor()
	)
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

nonisolated extension FileFormat {
	private static let imageProperties: any MetadataExtracting = ImagePropertiesExtractor()
	private static let videoProperties: any MetadataExtracting = VideoPropertiesExtractor()

	static let all: [FileFormat] = [
		// images
		FileFormat(extensions: ["jpg", "jpeg"], contentType: .jpeg, kind: .image, metadataExtractor: imageProperties),
		FileFormat(extensions: ["png"], contentType: .png, kind: .image, metadataExtractor: imageProperties),
		FileFormat(extensions: ["gif"], contentType: .gif, kind: .image, metadataExtractor: imageProperties),
		FileFormat(extensions: ["webp"], contentType: .webP, kind: .image, metadataExtractor: imageProperties),
		FileFormat(extensions: ["tif", "tiff"], contentType: .tiff, kind: .image, metadataExtractor: imageProperties),
		FileFormat(extensions: ["heic"], contentType: .heic, kind: .image, metadataExtractor: imageProperties),
		FileFormat(extensions: ["bmp"], contentType: .bmp, kind: .image, metadataExtractor: imageProperties),
		// camera raw — one row per format: capabilities will differ per
		// vendor. No stable per-vendor UTType constants; rawness is a facet,
		// kind is image. ImageIO reads their EXIF natively.
		FileFormat(extensions: ["cr2"], contentType: nil, kind: .image, isRawCapture: true, metadataExtractor: imageProperties),
		FileFormat(extensions: ["cr3"], contentType: nil, kind: .image, isRawCapture: true, metadataExtractor: imageProperties),
		FileFormat(extensions: ["nef"], contentType: nil, kind: .image, isRawCapture: true, metadataExtractor: imageProperties),
		FileFormat(extensions: ["arw"], contentType: nil, kind: .image, isRawCapture: true, metadataExtractor: imageProperties),
		FileFormat(extensions: ["dng"], contentType: nil, kind: .image, isRawCapture: true, metadataExtractor: imageProperties),
		FileFormat(extensions: ["orf"], contentType: nil, kind: .image, isRawCapture: true, metadataExtractor: imageProperties),
		FileFormat(extensions: ["raf"], contentType: nil, kind: .image, isRawCapture: true, metadataExtractor: imageProperties),
		FileFormat(extensions: ["rw2"], contentType: nil, kind: .image, isRawCapture: true, metadataExtractor: imageProperties),
		// video
		FileFormat(extensions: ["mov"], contentType: .quickTimeMovie, kind: .video, metadataExtractor: videoProperties),
		FileFormat(extensions: ["mp4"], contentType: .mpeg4Movie, kind: .video, metadataExtractor: videoProperties),
		FileFormat(extensions: ["m4v"], contentType: nil, kind: .video, metadataExtractor: videoProperties),
		FileFormat(extensions: ["avi"], contentType: .avi, kind: .video, metadataExtractor: videoProperties),
		FileFormat(extensions: ["mkv"], contentType: nil, kind: .video, metadataExtractor: videoProperties),
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
