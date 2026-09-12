//
//  ThumbnailGeneration.swift
//  Alexandria
//

import AVFoundation
import Foundation
import ImageIO
import QuickLookThumbnailing
import os

/// Thumbnail generation (thumbnailing round, 2026-09-11 —
/// _design/technical/thumbnails.md): domain-named functions consumed via the
/// format registry's `thumbnailer` column. Mechanism — which ImageIO flag,
/// the embedded-preview size guard — is each function's private business; the
/// registry speaks domains. An edge case is a branch inside its domain
/// function; a new behavior is a new function plus a row edit.

nonisolated enum ThumbnailError: Error {
	/// ImageIO could not open the file at all.
	case unreadable
	/// The decoder opened the file but produced no image.
	case decodeFailed
	/// JPEG encoding of a generated thumbnail failed.
	case encodeFailed
	/// The pass-level deadline fired (thumbnails.md invariant 2: abandon,
	/// not cancel — the stuck decode thread runs on; the pass moves on).
	case timedOut
}

/// Camera raw: extract the embedded JPEG preview — preview bytes only, never
/// sensor data (measured 2026-09-11: 9.3MB read of an 85MB RAF vs the whole
/// file plus a 5.5s decode; the NAS import contract lives in this gap).
nonisolated func generateRawThumbnail(from url: URL, maxPixelSize: Int) async throws -> CGImage {
	let preview = try await imageIOThumbnail(at: url, maxPixelSize: maxPixelSize, alwaysFromImage: false)
	// Tiny-preview vendors: verify the preview is worth keeping, don't trust.
	if max(preview.width, preview.height) >= 512 { return preview }
	return try await generateRasterThumbnail(from: url, maxPixelSize: maxPixelSize)
}

/// Plain images: real decode, downscaled in-decoder where the format allows
/// (JPEG/HEIF decode at reduced scale natively). Never IfAbsent here — that
/// returns an embedded EXIF thumb (160×120) as-is; MaxPixelSize is a ceiling,
/// not a floor.
nonisolated func generateRasterThumbnail(from url: URL, maxPixelSize: Int) async throws -> CGImage {
	try await imageIOThumbnail(at: url, maxPixelSize: maxPixelSize, alwaysFromImage: true)
}

/// Video: a keyframe near the start. Default (infinite) time tolerance is the
/// nearest-keyframe fast path — never tighten it for thumbnails. The cancel
/// handler is what lets the deadline actually leave (invariant 2): the
/// completion-handler async import does not observe task cancellation on its
/// own, so a wedged decode would otherwise hold the batch forever.
nonisolated func generateVideoThumbnail(from url: URL, maxPixelSize: Int) async throws -> CGImage {
	// Safety: cancelAllCGImageGeneration is documented thread-safe; the
	// generator never escapes this function.
	nonisolated(unsafe) let generator = AVAssetImageGenerator(asset: AVURLAsset(url: url))
	generator.appliesPreferredTrackTransform = true
	generator.maximumSize = CGSize(width: maxPixelSize, height: maxPixelSize)
	return try await withTaskCancellationHandler {
		try await generator.image(at: .zero).image
	} onCancel: {
		generator.cancelAllCGImageGeneration()
	}
}

/// The floor for vector/document/project rows: whatever Finder can draw,
/// via whatever QuickLook extensions are installed. Zero code per format.
/// Same cancellation story as the video path: the explicit cancel(_:) is
/// what makes the deadline able to abandon a wedged QuickLook extension.
nonisolated func generateQuickLookThumbnail(from url: URL, maxPixelSize: Int) async throws -> CGImage {
	let side = CGFloat(maxPixelSize)
	// Safety: QLThumbnailGenerator.cancel(_:) is an XPC cancel keyed by the
	// request; the request never escapes this function.
	nonisolated(unsafe) let request = QLThumbnailGenerator.Request(
		fileAt: url, size: CGSize(width: side, height: side), scale: 1,
		representationTypes: .thumbnail
	)
	return try await withTaskCancellationHandler {
		try await QLThumbnailGenerator.shared.generateBestRepresentation(for: request).cgImage
	} onCancel: {
		QLThumbnailGenerator.shared.cancel(request)
	}
}

/// Races an operation against a wall-clock deadline. On timeout the operation
/// is cancelled — cancellation-responsive work stops; a stuck synchronous
/// decode is abandoned (its thread runs on, its result is dropped) so the
/// group can leave. Without this, one wedged decode is an import that never
/// completes.
nonisolated func withThumbnailDeadline<T: Sendable>(
	_ limit: Duration,
	_ operation: @escaping @Sendable () async throws -> T
) async throws -> T {
	try await withThrowingTaskGroup(of: T.self) { group in
		group.addTask { try await operation() }
		group.addTask {
			try await Task.sleep(for: limit)
			throw ThumbnailError.timedOut
		}
		defer { group.cancelAll() }
		guard let winner = try await group.next() else { throw CancellationError() }
		return winner
	}
}

// MARK: - ImageIO bridging

/// ImageIO's home queue. Its calls are synchronous and uninterruptible, so
/// they may never run on the cooperative pool (thumbnails.md invariant 3):
/// a NAS-stalled decode there starves every async task in the app. In-flight
/// work is bounded by batch width per import in the common case, but the
/// queue itself is uncapped and abandoned decodes outlive their deadline by
/// design — a fixed-width cap is the named upgrade if wedge pileup ever
/// shows up in practice.
private nonisolated let decodeQueue = DispatchQueue(
	label: "alexandria.thumbnail.decode", qos: .utility, attributes: .concurrent
)

private nonisolated func imageIOThumbnail(
	at url: URL, maxPixelSize: Int, alwaysFromImage: Bool
) async throws -> CGImage {
	try await onDecodeQueue {
		guard let source = CGImageSourceCreateWithURL(url as CFURL, nil) else {
			throw ThumbnailError.unreadable
		}
		let options: [CFString: Any] = [
			(alwaysFromImage
				? kCGImageSourceCreateThumbnailFromImageAlways
				: kCGImageSourceCreateThumbnailFromImageIfAbsent): true,
			kCGImageSourceCreateThumbnailWithTransform: true,  // EXIF orientation; the default is false
			kCGImageSourceShouldCacheImmediately: true,        // decode here, not lazily at first render
			kCGImageSourceThumbnailMaxPixelSize: maxPixelSize,
		]
		guard let image = CGImageSourceCreateThumbnailAtIndex(source, 0, options as CFDictionary) else {
			throw ThumbnailError.decodeFailed
		}
		return image
	}
}

/// Runs synchronous work on the decode queue, resuming early with
/// `CancellationError` if the task is cancelled while the work is stuck —
/// the work itself is abandoned, its late result discarded (the lock makes
/// resume-once explicit). This is what lets a deadline actually fire against
/// an uninterruptible ImageIO call. Internal: ThumbnailStore rides the same
/// bridge for its synchronous encode + file write.
nonisolated func onDecodeQueue<T: Sendable>(
	_ work: @escaping @Sendable () throws -> T
) async throws -> T {
	let state = OSAllocatedUnfairLock<DecodeAwaitState<T>>(initialState: .pending)
	return try await withTaskCancellationHandler {
		try await withCheckedThrowingContinuation { continuation in
			let alreadyCancelled = state.withLock { current -> Bool in
				if case .abandoned = current { return true }
				current = .waiting(continuation)
				return false
			}
			if alreadyCancelled {
				continuation.resume(throwing: CancellationError())
				return
			}
			decodeQueue.async {
				let result = Result(catching: work)
				let claimed = state.withLock { current -> CheckedContinuation<T, any Error>? in
					guard case .waiting(let waiting) = current else { return nil }
					current = .resumed
					return waiting
				}
				claimed?.resume(with: result)
			}
		}
	} onCancel: {
		let claimed = state.withLock { current -> CheckedContinuation<T, any Error>? in
			if case .waiting(let waiting) = current {
				current = .abandoned
				return waiting
			}
			if case .pending = current { current = .abandoned }
			return nil
		}
		claimed?.resume(throwing: CancellationError())
	}
}

private nonisolated enum DecodeAwaitState<T: Sendable>: Sendable {
	case pending
	case waiting(CheckedContinuation<T, any Error>)
	case resumed
	case abandoned
}
