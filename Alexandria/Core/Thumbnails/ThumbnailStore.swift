//
//  ThumbnailStore.swift
//  Alexandria
//

import Foundation
import ImageIO
internal import UniformTypeIdentifiers

/// The visible `Thumbnails/` store beside the `.alxcat` package (layout
/// ratified 2026-09-10). One JPEG per file at a single p0 size, sharded by id
/// prefix so the directory stays sane at the ~1M-item scale target.
/// Regenerable by design: deleting the store loses nothing the source files
/// can't rebuild.
nonisolated struct ThumbnailStore: Sendable {
	/// The p0 thumbnail size: long edge, pixels. One size for now; a ladder
	/// is a future round.
	static let maxPixelSize = 1024
	private static let jpegQuality = 0.8

	let directory: URL

	init(catalogDirectory: URL) {
		self.directory = catalogDirectory.appending(path: "Thumbnails")
	}

	func url(for id: Identifier<File>) -> URL {
		let name = id.rawValue.uuidString.lowercased()
		// Shard on the id's random TAIL: a UUIDv7's leading characters are
		// the millisecond timestamp's top bits — constant until 2039 — so a
		// prefix shard is one directory wearing a costume.
		return directory
			.appending(path: String(name.suffix(2)))
			.appending(path: name + ".jpg")
	}

	/// Write before stamp, atomically (thumbnails.md invariant 4): a crash
	/// between file and stamp leaves an orphan the retry overwrites — never a
	/// thumbnail_at without bytes. Encode and file IO are synchronous, so the
	/// whole body rides the decode queue (invariant 3), never the
	/// cooperative pool.
	func write(_ image: CGImage, for id: Identifier<File>) async throws {
		let destination = url(for: id)
		try await onDecodeQueue {
			try FileManager.default.createDirectory(
				at: destination.deletingLastPathComponent(), withIntermediateDirectories: true
			)
			let data = NSMutableData()
			guard let encoder = CGImageDestinationCreateWithData(
				data, UTType.jpeg.identifier as CFString, 1, nil
			) else {
				throw ThumbnailError.encodeFailed
			}
			let options = [kCGImageDestinationLossyCompressionQuality: Self.jpegQuality]
			CGImageDestinationAddImage(encoder, Self.flattened(image), options as CFDictionary)
			guard CGImageDestinationFinalize(encoder) else {
				throw ThumbnailError.encodeFailed
			}
			try (data as Data).write(to: destination, options: .atomic)
		}
	}

	/// JPEG has no alpha and CGImageDestination mattes transparency onto
	/// black; transparent sources (PNG, QuickLook representations) get a
	/// white ground instead.
	private static func flattened(_ image: CGImage) -> CGImage {
		switch image.alphaInfo {
		case .none, .noneSkipFirst, .noneSkipLast:
			return image
		default:
			break
		}
		guard let context = CGContext(
			data: nil, width: image.width, height: image.height,
			bitsPerComponent: 8, bytesPerRow: 0,
			space: image.colorSpace ?? CGColorSpaceCreateDeviceRGB(),
			bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue
		) else { return image }
		let bounds = CGRect(x: 0, y: 0, width: image.width, height: image.height)
		context.setFillColor(CGColor(red: 1, green: 1, blue: 1, alpha: 1))
		context.fill(bounds)
		context.draw(image, in: bounds)
		return context.makeImage() ?? image
	}
}
