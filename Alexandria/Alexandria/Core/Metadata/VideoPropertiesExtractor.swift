//
//  VideoPropertiesExtractor.swift
//  Alexandria
//

import Foundation
import AVFoundation

/// Metadata for the video family via AVFoundation (replaces the old core's
/// ffprobe lane). Dimensions are display dimensions: the track's natural
/// size with its preferred transform applied, so a rotated phone clip
/// reports upright.
nonisolated struct VideoPropertiesExtractor: MetadataExtracting {
	func extract(from url: URL) async throws -> FileMetadata {
		let asset = AVURLAsset(url: url)
		var metadata = FileMetadata()

		let duration = try await asset.load(.duration)
		if duration.isNumeric {
			metadata.durationSeconds = duration.seconds
		}

		if let videoTrack = try await asset.loadTracks(withMediaType: .video).first {
			let (naturalSize, preferredTransform) = try await videoTrack.load(.naturalSize, .preferredTransform)
			let displaySize = naturalSize.applying(preferredTransform)
			metadata.width = Int(abs(displaySize.width).rounded())
			metadata.height = Int(abs(displaySize.height).rounded())
		}

		if let creationDate = try await asset.load(.creationDate) {
			metadata.capturedAt = try await creationDate.load(.dateValue)
		}
		return metadata
	}
}
