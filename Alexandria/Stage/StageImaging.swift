//
//  StageImaging.swift
//  Alexandria
//
//  The stage's image engine owner (grid round, 2026-09-12). NOT a facade —
//  the coordinator sees Nuke types freely; this type exists for two honest
//  reasons the design round settled:
//
//   1. Ownership: the Nuke pipeline holds the decoded-image cache, and the
//      cache must outlive a grid↔loupe renderer swap (grid.md invariant
//      10). So it's held by the stage (a StateObject-lived reference), above
//      the renderers, never by a coordinator that dies with its view.
//   2. One place builds requests: content, upgrade, and prefetch all come
//      through `request(...)`, so the same (file, bucket) always yields the
//      same cache key — a warmed image is a hit, never a near-miss cached
//      twice under drifting keys (invariant 2).
//
//  Nuke's own cache and scheduling defaults are used as-is (grid.md: start
//  naked, add a compensation only when a watchpoint fires). No aggressive
//  disk DataCache: the Thumbnails/ store IS the disk layer, so a second
//  on-disk copy would double the bytes for nothing.
//

import AppKit
import Nuke

@MainActor final class StageImaging {
	let pipeline: ImagePipeline
	let prefetcher: ImagePrefetcher

	init() {
		let pipeline = ImagePipeline()
		self.pipeline = pipeline
		// Prefetch decodes into the memory cache and rides the lowest
		// priority, so speculation can warm cells ahead of the viewport
		// without ever delaying a visible cell's demand.
		let prefetcher = ImagePrefetcher(pipeline: pipeline, destination: .memoryCache)
		prefetcher.priority = .veryLow
		self.prefetcher = prefetcher
	}

	/// The ONE request constructor — it owns the whole cache key, so a caller
	/// hands it a `file` and a `bucket`, never a pre-formed key string. The
	/// key is `file id @ bucket pixels`; the bytes come from the local
	/// thumbnail via a closure, so no DataLoader/file-URL round trip is in
	/// play. Thumbnail decode clamps the long edge to the bucket — a
	/// target-size decode off the stored 1024px JPEG's DCT ladder. Every
	/// caller — content, prefetch, cache probe — comes through here, so the
	/// same (file, bucket) always keys the cache identically and a warmed
	/// image is a hit, never a miss cached twice (invariant 2).
	func request(
		file: Identifier<File>, fileURL: URL, bucket: DecodeBucket, urgency: Urgency
	) -> ImageRequest {
		var request = ImageRequest(
			id: Self.cacheID(file, bucket: bucket),
			data: { try Data(contentsOf: fileURL) },
			userInfo: [.thumbnailKey: ImageRequest.ThumbnailOptions(maxPixelSize: Float(bucket.pixels))]
		)
		request.priority = Self.priority(for: urgency)
		return request
	}

	/// The best decoded size already in memory for this file, largest first —
	/// the progressive-paint source, so a visible cell shows whatever it has
	/// instantly instead of blanking while the target decodes. `ladder` is the
	/// store's set of valid sizes to probe. Nil when nothing is cached; a
	/// cache read never invokes the data closure.
	func cachedImage(
		file: Identifier<File>, fileURL: URL, ladder: [DecodeBucket]
	) -> (image: NSImage, bucket: DecodeBucket)? {
		for bucket in ladder.sorted(by: >) {
			let request = request(file: file, fileURL: fileURL, bucket: bucket, urgency: .content)
			if let image = pipeline.cache[request]?.image {
				return (image, bucket)
			}
		}
		return nil
	}

	/// The cache key: file id + bucket pixels, lowercased. The single place
	/// this string is formed, so no caller can derive it differently.
	private static func cacheID(_ file: Identifier<File>, bucket: DecodeBucket) -> String {
		"\(file.rawValue.uuidString.lowercased())@\(bucket.pixels)"
	}

	private static func priority(for urgency: Urgency) -> ImageRequest.Priority {
		switch urgency {
		case .content: return .high
		case .preheat: return .veryLow
		}
	}
}
