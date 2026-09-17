//
//  ImagePropertiesExtractor.swift
//  Alexandria
//

import Foundation
import ImageIO

/// The one extractor for the whole image family, RAW included — ImageIO
/// reads a RAF's EXIF exactly as a JPEG's, replacing the old core's separate
/// raster and exiftool lanes for first-class fields.
///
/// One CGImageSource open per file (the shared-read-window discipline);
/// every facet the source testifies to is filled from that one read. The
/// extractor is source-shaped, its private methods are facet-shaped.
nonisolated struct ImagePropertiesExtractor: MetadataExtracting {
	func extract(from url: URL) async throws -> FileMetadata {
		let options = [kCGImageSourceShouldCache: false] as CFDictionary
		guard
			let source = CGImageSourceCreateWithURL(url as CFURL, options),
			let properties = CGImageSourceCopyPropertiesAtIndex(source, 0, options) as? [CFString: Any]
		else {
			throw MetadataExtractionError.unreadableSource(url)
		}

		let tiff = properties[kCGImagePropertyTIFFDictionary] as? [CFString: Any]
		let exif = properties[kCGImagePropertyExifDictionary] as? [CFString: Any]
		let iptc = properties[kCGImagePropertyIPTCDictionary] as? [CFString: Any]
		let gps = properties[kCGImagePropertyGPSDictionary] as? [CFString: Any]

		return FileMetadata(
			visual: visualFacet(properties, exif: exif),
			capture: captureFacet(tiff: tiff, exif: exif),
			authorship: authorshipFacet(tiff: tiff, iptc: iptc),
			location: locationFacet(gps)
		).normalized()
	}

	private func visualFacet(_ properties: [CFString: Any], exif: [CFString: Any]?) -> VisualFacet {
		var visual = VisualFacet()
		// PixelWidth/Height at index 0 are the ENCODED frame; orientation is
		// the separate transform — the file's own model, stored as-is.
		visual.width = integer(properties[kCGImagePropertyPixelWidth])
		visual.height = integer(properties[kCGImagePropertyPixelHeight])
		visual.orientation = ExifOrientation(code: integer(properties[kCGImagePropertyOrientation]))
		visual.bitsPerSample = integer(properties[kCGImagePropertyDepth])
		visual.colorModel = trimmedString(properties[kCGImagePropertyColorModel])
		if let code = integer(exif?[kCGImagePropertyExifColorSpace]) {
			// TODO: Adobe RGB hides behind Uncalibrated + interop index
			// "R03"; refine when a fixture proves the ImageIO key for it.
			visual.colorSpace = colorSpaceName(code: code)
		}
		return visual
	}

	private func captureFacet(tiff: [CFString: Any]?, exif: [CFString: Any]?) -> CaptureFacet {
		var capture = CaptureFacet()
		capture.make = trimmedString(tiff?[kCGImagePropertyTIFFMake])
		capture.model = trimmedString(tiff?[kCGImagePropertyTIFFModel])
		capture.serialNumber = trimmedString(exif?[kCGImagePropertyExifBodySerialNumber])
		capture.lensModel = trimmedString(exif?[kCGImagePropertyExifLensModel])
		capture.focalLength = double(exif?[kCGImagePropertyExifFocalLength])
		capture.focalLength35mm = integer(exif?[kCGImagePropertyExifFocalLenIn35mmFilm])
		capture.aperture = double(exif?[kCGImagePropertyExifFNumber])
		capture.exposureSeconds = double(exif?[kCGImagePropertyExifExposureTime])
		capture.exposureBias = double(exif?[kCGImagePropertyExifExposureBiasValue])
		capture.iso = integer(firstElement(exif?[kCGImagePropertyExifISOSpeedRatings]))
		capture.flash = flashFired(code: integer(exif?[kCGImagePropertyExifFlash]))
		capture.exposureProgram = exposureProgramName(code: integer(exif?[kCGImagePropertyExifExposureProgram]))
		capture.meteringMode = meteringModeName(code: integer(exif?[kCGImagePropertyExifMeteringMode]))
		capture.whiteBalance = whiteBalanceName(code: integer(exif?[kCGImagePropertyExifWhiteBalance]))
		capture.capturedAt = exifWallClockDate(
			trimmedString(exif?[kCGImagePropertyExifDateTimeOriginal]),
			subseconds: trimmedString(exif?[kCGImagePropertyExifSubsecTimeOriginal])
		)
		capture.captureOffset = trimmedString(exif?[kCGImagePropertyExifOffsetTimeOriginal])
		return capture
	}

	/// Dual-source by ruling: TIFF and IPTC variants side by side, one read
	/// each, no precedence anywhere.
	private func authorshipFacet(tiff: [CFString: Any]?, iptc: [CFString: Any]?) -> AuthorshipFacet {
		var authorship = AuthorshipFacet()
		authorship.tiffArtist = trimmedString(tiff?[kCGImagePropertyTIFFArtist])
		authorship.tiffImageDescription = trimmedString(tiff?[kCGImagePropertyTIFFImageDescription])
		authorship.tiffCopyright = trimmedString(tiff?[kCGImagePropertyTIFFCopyright])
		authorship.tiffSoftware = trimmedString(tiff?[kCGImagePropertyTIFFSoftware])
		authorship.iptcCreator = trimmedString(firstElement(iptc?[kCGImagePropertyIPTCByline]))
		authorship.iptcCaption = trimmedString(iptc?[kCGImagePropertyIPTCCaptionAbstract])
		authorship.iptcTitle = trimmedString(iptc?[kCGImagePropertyIPTCObjectName])
		authorship.iptcCopyright = trimmedString(iptc?[kCGImagePropertyIPTCCopyrightNotice])
		return authorship
	}

	private func locationFacet(_ gps: [CFString: Any]?) -> LocationFacet {
		var location = LocationFacet()
		location.latitude = signedCoordinate(
			double(gps?[kCGImagePropertyGPSLatitude]),
			reference: trimmedString(gps?[kCGImagePropertyGPSLatitudeRef]),
			negativeReference: "S"
		)
		location.longitude = signedCoordinate(
			double(gps?[kCGImagePropertyGPSLongitude]),
			reference: trimmedString(gps?[kCGImagePropertyGPSLongitudeRef]),
			negativeReference: "W"
		)
		location.altitude = signedAltitude(
			double(gps?[kCGImagePropertyGPSAltitude]),
			reference: integer(gps?[kCGImagePropertyGPSAltitudeRef])
		)
		return location
	}
}

// MARK: - Coercion and rendering helpers (pure)

/// EXIF ASCII fields are NUL-padded to their slot width; plain whitespace
/// trimming leaves the NULs, so they must be cut too or they land verbatim
/// in the catalog.
nonisolated func trimmedString(_ value: Any?) -> String? {
	guard let text = value as? String else { return nil }
	let trimmed = text.trimmingCharacters(
		in: CharacterSet.whitespacesAndNewlines.union(CharacterSet(charactersIn: "\0"))
	)
	return trimmed.isEmpty ? nil : trimmed
}

nonisolated func integer(_ value: Any?) -> Int? {
	(value as? NSNumber)?.intValue
}

/// The one numeric gate for every Double that reaches a facet: a non-finite
/// value (a 1/0 EXIF rational, an indefinite AV estimate) is "the file
/// doesn't say" — it must never reach the JSON encoder, whose non-conforming
/// -float strategy throws inside the insert transaction.
nonisolated func double(_ value: Any?) -> Double? {
	guard let number = (value as? NSNumber)?.doubleValue, number.isFinite else { return nil }
	return number
}

/// List-valued EXIF tags (ISOSpeedRatings, IPTC By-line) key on their first element.
nonisolated func firstElement(_ value: Any?) -> Any? {
	if let array = value as? [Any] { return array.first }
	return value
}

/// Cached: DateFormatter construction is expensive and this runs per file
/// on the import hot path (the catalogDateFormatter precedent).
private nonisolated let exifDateParser: DateFormatter = {
	let formatter = DateFormatter()
	formatter.locale = Locale(identifier: "en_US_POSIX")
	formatter.timeZone = TimeZone(identifier: "UTC")
	formatter.dateFormat = "yyyy:MM:dd HH:mm:ss"
	return formatter
}()

/// EXIF timestamps ("2026:08:14 09:31:02") carry no timezone: parsed as
/// wall-clock, UTC-labelled. SubSecTimeOriginal ("057") folds in as
/// fractional seconds so burst frames within one second keep shutter order.
nonisolated func exifWallClockDate(_ text: String?, subseconds: String? = nil) -> Date? {
	guard let text else { return nil }
	guard let base = exifDateParser.date(from: text) else { return nil }
	guard let subseconds, !subseconds.isEmpty, subseconds.allSatisfy(\.isNumber),
		  let fraction = Double("0." + subseconds) else {
		return base
	}
	return base.addingTimeInterval(fraction)
}

/// EXIF Flash (0xA025) is a bitfield, not a boolean: bit 0 = fired,
/// bit 5 = no flash function. Collapses honestly: nil when the device has
/// no flash to report on, else whether it fired.
nonisolated func flashFired(code: Int?) -> Bool? {
	guard let code else { return nil }
	if code & 0b100000 != 0 { return nil }
	return code & 1 == 1
}

/// EXIF ExposureProgram (0x8822) → LrC-style name. 0 = not defined → nil.
nonisolated func exposureProgramName(code: Int?) -> String? {
	switch code {
	case 1: "Manual"
	case 2: "Program"
	case 3: "Aperture priority"
	case 4: "Shutter priority"
	case 5: "Creative"
	case 6: "Action"
	case 7: "Portrait"
	case 8: "Landscape"
	default: nil
	}
}

/// EXIF MeteringMode (0x9207) → common name. 0/255 (unknown/other) → nil.
nonisolated func meteringModeName(code: Int?) -> String? {
	switch code {
	case 1: "Average"
	case 2: "Center-weighted"
	case 3: "Spot"
	case 4: "Multi-spot"
	case 5: "Pattern"
	case 6: "Partial"
	default: nil
	}
}

/// EXIF WhiteBalance (0xA403) → name.
nonisolated func whiteBalanceName(code: Int?) -> String? {
	switch code {
	case 0: "Auto"
	case 1: "Manual"
	default: nil
	}
}

/// Maps the EXIF ColorSpace short (0xA001) onto its common display name;
/// rare vendor codes yield nil.
nonisolated func colorSpaceName(code: Int) -> String? {
	switch code {
	case 1: "sRGB"
	case 2: "Adobe RGB"  // non-standard, but some cameras write it
	case 65535: "Uncalibrated"
	default: nil
	}
}

/// ImageIO reports GPS coordinates unsigned; the hemisphere lives in the
/// ref tag. Folds them into signed decimal degrees.
nonisolated func signedCoordinate(_ degrees: Double?, reference: String?, negativeReference: String) -> Double? {
	guard let degrees else { return nil }
	if let reference, reference.caseInsensitiveCompare(negativeReference) == .orderedSame {
		return -degrees
	}
	return degrees
}

/// GPS altitude is unsigned with AltitudeRef 1 meaning below sea level —
/// signedCoordinate's concept for the vertical axis.
nonisolated func signedAltitude(_ meters: Double?, reference: Int?) -> Double? {
	guard let meters else { return nil }
	return reference == 1 ? -meters : meters
}
