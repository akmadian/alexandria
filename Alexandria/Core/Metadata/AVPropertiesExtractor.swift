//
//  AVPropertiesExtractor.swift
//  Alexandria
//

import Foundation
import AVFoundation
import CoreMedia
import Logging

private nonisolated let log = Logger(label: "metadata.av")

/// The one extractor for both AVFoundation families — video AND audio
/// (metadata modeling round, 2026-09-17; replaces the old core's ffprobe
/// lane and the video-only extractor). One AVURLAsset open per file; facets
/// fill by TRACK EVIDENCE, which is exactly what evidence-decides-presence
/// wants: a silent screen recording gets no sound fields, a voice memo gets
/// no visual facet, and neither is an error.
///
/// Best-effort inside: individual load failures are logged at debug and
/// yield absent fields. A file where every load failed AND nothing was read
/// throws `unreadableSource` — the image lane's precedent — so a wholly
/// unreadable container leaves DLQ residue instead of a silent NULL.
nonisolated struct AVPropertiesExtractor: MetadataExtracting {
	func extract(from url: URL) async throws -> FileMetadata {
		let asset = AVURLAsset(url: url)
		var metadata = FileMetadata()
		var loadFailures = 0

		func attempt<T>(_ what: StaticString, _ load: () async throws -> T) async -> T? {
			do {
				return try await load()
			} catch {
				loadFailures += 1
				log.debug("AV load failed", metadata: [
					"what": "\(what)", "url": "\(url.lastPathComponent)", "error": "\(error)",
				])
				return nil
			}
		}

		var timing = TimingFacet()
		if let duration = await attempt("duration", { try await asset.load(.duration) }),
		   duration.isNumeric {
			timing.durationSeconds = double(duration.seconds as NSNumber)
		}
		metadata.timing = timing

		var media = MediaFacet()

		if let videoTrack = await attempt("videoTracks", { try await asset.loadTracks(withMediaType: .video) })?.first {
			var visual = VisualFacet()
			if let (naturalSize, transform) = await attempt("naturalSize", {
				try await videoTrack.load(.naturalSize, .preferredTransform)
			}) {
				// ENCODED dimensions + the transform mapped onto the same 1-8
				// codes photos use — one meaning for width/height across kinds
				// (ruling 2026-09-17). A non-axis-aligned transform (rare,
				// authored) stores the display size upright instead.
				if let orientation = ExifOrientation(transform: transform) {
					visual.width = pixelCount(naturalSize.width)
					visual.height = pixelCount(naturalSize.height)
					visual.orientation = orientation
				} else {
					let display = naturalSize.applying(transform)
					visual.width = pixelCount(abs(display.width))
					visual.height = pixelCount(abs(display.height))
					visual.orientation = .up
				}
			}
			metadata.visual = visual

			if let rate = await attempt("frameRate", { try await videoTrack.load(.nominalFrameRate) }),
			   let finite = double(rate as NSNumber), finite > 0 {
				media.frameRate = finite
			}
			if let format = await attempt("videoFormat", { try await videoTrack.load(.formatDescriptions) })?.first {
				media.videoCodec = fourCCString(format.mediaSubType.rawValue)
			}
		}

		if let audioTrack = await attempt("audioTracks", { try await asset.loadTracks(withMediaType: .audio) })?.first,
		   let format = await attempt("audioFormat", { try await audioTrack.load(.formatDescriptions) })?.first {
			media.audioCodec = fourCCString(format.mediaSubType.rawValue)
			if let asbd = format.audioStreamBasicDescription {
				media.sampleRate = double(asbd.mSampleRate as NSNumber).flatMap(hertz)
				media.channelCount = asbd.mChannelsPerFrame > 0 && asbd.mChannelsPerFrame < 256
					? Int(asbd.mChannelsPerFrame) : nil
			}
		}
		metadata.media = media

		let items = await attempt("metadata", { try await asset.load(.metadata) }) ?? []
		metadata.capture = await captureFacet(asset: asset, items: items)
		metadata.audio = await audioFacet(items: items)
		metadata.location = await locationFacet(items: items)

		let normalized = metadata.normalized()
		if normalized.isEmpty && loadFailures > 0 {
			// Nothing readable AND loads actually errored: the source itself
			// is the problem. A genuinely bare-but-readable file (zero
			// failures) stays a valid empty result.
			throw MetadataExtractionError.unreadableSource(url)
		}
		return normalized
	}

	/// The creation event as the container states it. QuickTime's own
	/// creationdate is an ISO 8601 string WITH zone — parsed as wall-clock +
	/// offset, matching EXIF semantics exactly, so photos and camera clips
	/// shot in the same minute sort together. The common-key instant is the
	/// fallback (AVI etc.): its UTC wall-clock reading, offset unknown.
	private func captureFacet(asset: AVURLAsset, items: [AVMetadataItem]) async -> CaptureFacet {
		var capture = CaptureFacet()

		let qtDate = AVMetadataItem.metadataItems(
			from: items, filteredByIdentifier: .quickTimeMetadataCreationDate
		).first
		if let text = try? await qtDate?.load(.stringValue),
		   let (wallClock, offset) = iso8601WallClock(text) {
			capture.capturedAt = wallClock
			capture.captureOffset = offset
		} else if let creationDate = try? await asset.load(.creationDate),
				  let instant = try? await creationDate.load(.dateValue) {
			capture.capturedAt = instant
		}

		capture.make = await stringValue(items, .quickTimeMetadataMake)
		capture.model = await stringValue(items, .quickTimeMetadataModel)
		return capture
	}

	/// Embedded tags: no common keyspace covers the roster, so each field
	/// reads its per-format identifiers in one fixed order (iTunes/MP4, then
	/// ID3) — a keyspace mapping, not a source precedence: a file carries
	/// one tagging keyspace, so at most one answers.
	// TODO: fixture-prove this path — TestData has no tagged .mp3/.m4a; the
	// identifier walk is verified against AVFoundation semantics only.
	private func audioFacet(items: [AVMetadataItem]) async -> AudioFacet {
		var audio = AudioFacet()
		audio.album = await stringValue(items, .commonIdentifierAlbumName)
		audio.composer = await stringValue(items, .iTunesMetadataComposer, .id3MetadataComposer)
		audio.genre = await stringValue(
			items, .iTunesMetadataUserGenre, .iTunesMetadataPredefinedGenre, .id3MetadataContentType
		)
		audio.trackNumber = await trackNumber(items)
		return audio
	}

	/// ID3's TRCK is text ("4/12"); iTunes' trkn atom is BINARY — a
	/// big-endian pad/track/total tuple that stringValue can't read.
	private func trackNumber(_ items: [AVMetadataItem]) async -> Int? {
		if let text = await stringValue(items, .id3MetadataTrackNumber) {
			return Int(text.prefix(while: \.isNumber))
		}
		let trkn = AVMetadataItem.metadataItems(from: items, filteredByIdentifier: .iTunesMetadataTrackNumber)
		for item in trkn {
			if let number = try? await item.load(.numberValue) {
				return number.intValue
			}
			if let data = try? await item.load(.dataValue), data.count >= 4 {
				// Bytes 2-3 are the track (big-endian UInt16).
				return Int(data[data.startIndex + 2]) << 8 | Int(data[data.startIndex + 3])
			}
		}
		return nil
	}

	private func locationFacet(items: [AVMetadataItem]) async -> LocationFacet {
		var location = LocationFacet()
		if let iso6709 = await stringValue(items, .quickTimeMetadataLocationISO6709),
		   let parsed = parseISO6709(iso6709) {
			location.latitude = parsed.latitude
			location.longitude = parsed.longitude
			location.altitude = parsed.altitude
		}
		return location
	}

	/// First non-nil string among the given identifiers, in order.
	private func stringValue(_ items: [AVMetadataItem], _ identifiers: AVMetadataIdentifier...) async -> String? {
		for identifier in identifiers {
			let matches = AVMetadataItem.metadataItems(from: items, filteredByIdentifier: identifier)
			for item in matches {
				if let text = trimmedString(try? await item.load(.stringValue)) { return text }
			}
		}
		return nil
	}
}

// MARK: - Pure helpers

/// A pixel dimension from AV geometry: finite and plausibly sized, or nil.
/// `Int(_: Double)` TRAPS on NaN/inf — this is the gate that keeps a
/// damaged container's geometry from crashing the import.
private nonisolated func pixelCount(_ value: CGFloat) -> Int? {
	guard let finite = double(value as NSNumber), finite >= 0, finite < 1e9 else { return nil }
	return Int(finite.rounded())
}

/// A sample rate in Hz: positive, finite, below absurdity.
private nonisolated func hertz(_ value: Double) -> Int? {
	guard value > 0, value < 1e9 else { return nil }
	return Int(value)
}

nonisolated extension ExifOrientation {
	/// Maps an axis-aligned QuickTime preferredTransform onto the EXIF
	/// rotation codes (mirrored codes don't occur in track transforms).
	/// nil = not axis-aligned; the caller stores display dimensions upright.
	init?(transform: CGAffineTransform) {
		func near(_ value: CGFloat, _ target: CGFloat) -> Bool { abs(value - target) < 0.001 }
		if near(transform.a, 1), near(transform.b, 0), near(transform.c, 0), near(transform.d, 1) {
			self = .up
		} else if near(transform.a, -1), near(transform.b, 0), near(transform.c, 0), near(transform.d, -1) {
			self = .down
		} else if near(transform.a, 0), near(transform.b, 1), near(transform.c, -1), near(transform.d, 0) {
			self = .right   // 90° CW to display
		} else if near(transform.a, 0), near(transform.b, -1), near(transform.c, 1), near(transform.d, 0) {
			self = .left    // 270° CW to display
		} else {
			return nil
		}
	}
}

/// FourCC codec identifier as its display string ("hvc1", "aac "→"aac").
nonisolated func fourCCString(_ code: FourCharCode) -> String? {
	let bytes = [
		UInt8((code >> 24) & 0xFF), UInt8((code >> 16) & 0xFF),
		UInt8((code >> 8) & 0xFF), UInt8(code & 0xFF),
	]
	guard bytes.allSatisfy({ $0 >= 0x20 && $0 < 0x7F }) else { return nil }
	return trimmedString(String(decoding: bytes, as: UTF8.self))
}

/// Cached: DateFormatter construction is expensive and this runs per file
/// on the import hot path (the catalogDateFormatter precedent). Formatting
/// through a shared DateFormatter is thread-safe.
private nonisolated let wallClockParser: DateFormatter = {
	let formatter = DateFormatter()
	formatter.locale = Locale(identifier: "en_US_POSIX")
	formatter.timeZone = TimeZone(identifier: "UTC")
	formatter.dateFormat = "yyyy-MM-dd'T'HH:mm:ss"
	return formatter
}()

/// Splits an ISO 8601 timestamp with zone ("2026-01-12T09:31:02-0800",
/// fractional seconds allowed) into the EXIF-style pair: wall-clock Date
/// (UTC-labelled, as the camera's clock read, subseconds folded in) +
/// normalized offset ("-08:00"). nil when the text isn't that shape.
nonisolated func iso8601WallClock(_ text: String) -> (wallClock: Date, offset: String?)? {
	let trimmed = text.trimmingCharacters(in: .whitespaces)
	guard trimmed.count >= 19,
		  var wallClock = wallClockParser.date(from: String(trimmed.prefix(19))) else { return nil }

	// Optional fractional seconds between the seconds and the zone.
	var tail = String(trimmed.dropFirst(19))
	if tail.first == "." {
		let digits = tail.dropFirst().prefix(while: \.isNumber)
		if let fraction = Double("0." + digits) { wallClock += fraction }
		tail = String(tail.dropFirst(1 + digits.count))
	}

	if tail.isEmpty { return (wallClock, nil) }
	if tail == "Z" { return (wallClock, "+00:00") }
	// "±HH:MM", "±HHMM", or "±HH" → "±HH:MM"
	let sign = tail.prefix(1)
	guard sign == "+" || sign == "-" else { return (wallClock, nil) }
	let digits = tail.dropFirst().filter(\.isNumber)
	switch digits.count {
	case 2: return (wallClock, "\(sign)\(digits):00")
	case 4: return (wallClock, "\(sign)\(digits.prefix(2)):\(digits.suffix(2))")
	default: return (wallClock, nil)
	}
}

/// ISO 6709 point form ("+37.3349-122.0090+021.086/") → signed decimal
/// degrees + optional altitude meters.
nonisolated func parseISO6709(_ text: String) -> (latitude: Double, longitude: Double, altitude: Double?)? {
	var numbers: [Double] = []
	var current = ""
	for character in text {
		if character == "+" || character == "-" {
			if !current.isEmpty, let value = Double(current) { numbers.append(value) }
			current = String(character)
		} else if character.isNumber || character == "." {
			current.append(character)
		} else {
			break  // "/" or CRS suffix ends the point
		}
	}
	if !current.isEmpty, let value = Double(current) { numbers.append(value) }
	guard numbers.count >= 2, numbers[0].isFinite, numbers[1].isFinite else { return nil }
	let altitude = numbers.count >= 3 && numbers[2].isFinite ? numbers[2] : nil
	return (numbers[0], numbers[1], altitude)
}
