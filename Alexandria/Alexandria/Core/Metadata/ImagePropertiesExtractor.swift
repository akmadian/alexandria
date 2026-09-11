//
//  ImagePropertiesExtractor.swift
//  Alexandria
//

import Foundation
import ImageIO

/// The one extractor for the whole image family, RAW included — ImageIO
/// reads a RAF's EXIF exactly as a JPEG's, replacing the old core's separate
/// raster and exiftool lanes for first-class fields.
nonisolated struct ImagePropertiesExtractor: MetadataExtracting {
	func extract(from url: URL) async throws -> FileMetadata {
		let options = [kCGImageSourceShouldCache: false] as CFDictionary
		guard
			let source = CGImageSourceCreateWithURL(url as CFURL, options),
			let properties = CGImageSourceCopyPropertiesAtIndex(source, 0, options) as? [CFString: Any]
		else {
			throw MetadataExtractionError.unreadableSource(url)
		}

		var metadata = FileMetadata()
		metadata.width = integer(properties[kCGImagePropertyPixelWidth])
		metadata.height = integer(properties[kCGImagePropertyPixelHeight])
		metadata.orientation = integer(properties[kCGImagePropertyOrientation])
		metadata.bitDepth = integer(properties[kCGImagePropertyDepth])

		if let tiff = properties[kCGImagePropertyTIFFDictionary] as? [CFString: Any] {
			metadata.cameraMake = trimmedString(tiff[kCGImagePropertyTIFFMake])
			metadata.cameraModel = trimmedString(tiff[kCGImagePropertyTIFFModel])
			metadata.creator = trimmedString(tiff[kCGImagePropertyTIFFArtist])
			metadata.copyright = trimmedString(tiff[kCGImagePropertyTIFFCopyright])
		}

		if let exif = properties[kCGImagePropertyExifDictionary] as? [CFString: Any] {
			metadata.lensModel = trimmedString(exif[kCGImagePropertyExifLensModel])
			metadata.capturedAt = exifWallClockDate(trimmedString(exif[kCGImagePropertyExifDateTimeOriginal]))
			metadata.aperture = double(exif[kCGImagePropertyExifFNumber])
			metadata.focalLength = double(exif[kCGImagePropertyExifFocalLength])
			metadata.iso = integer(firstElement(exif[kCGImagePropertyExifISOSpeedRatings]))
			if let seconds = double(exif[kCGImagePropertyExifExposureTime]) {
				metadata.shutterSpeed = shutterSpeedDisplay(seconds: seconds)
			}
			if let code = integer(exif[kCGImagePropertyExifColorSpace]) {
				// TODO: Adobe RGB hides behind Uncalibrated + interop index
				// "R03"; refine when a fixture proves the ImageIO key for it.
				metadata.colorSpace = colorSpaceName(code: code)
			}
		}

		if let gps = properties[kCGImagePropertyGPSDictionary] as? [CFString: Any] {
			metadata.gpsLatitude = signedCoordinate(
				double(gps[kCGImagePropertyGPSLatitude]),
				reference: trimmedString(gps[kCGImagePropertyGPSLatitudeRef]),
				negativeReference: "S"
			)
			metadata.gpsLongitude = signedCoordinate(
				double(gps[kCGImagePropertyGPSLongitude]),
				reference: trimmedString(gps[kCGImagePropertyGPSLongitudeRef]),
				negativeReference: "W"
			)
		}
		return metadata
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

nonisolated func double(_ value: Any?) -> Double? {
	(value as? NSNumber)?.doubleValue
}

/// List-valued EXIF tags (ISOSpeedRatings) key on their first element.
nonisolated func firstElement(_ value: Any?) -> Any? {
	if let array = value as? [Any] { return array.first }
	return value
}

/// EXIF timestamps ("2026:08:14 09:31:02") carry no timezone: parsed as
/// wall-clock, UTC-labelled.
nonisolated func exifWallClockDate(_ text: String?) -> Date? {
	guard let text else { return nil }
	let formatter = DateFormatter()
	formatter.locale = Locale(identifier: "en_US_POSIX")
	formatter.timeZone = TimeZone(identifier: "UTC")
	formatter.dateFormat = "yyyy:MM:dd HH:mm:ss"
	return formatter.date(from: text)
}

/// Renders an exposure time as a human shutter speed: "1/250" for fast,
/// decimal seconds for slow.
nonisolated func shutterSpeedDisplay(seconds: Double) -> String? {
	guard seconds > 0 else { return nil }
	if seconds < 1 {
		return "1/\(Int((1 / seconds).rounded()))"
	}
	var formatted = String(format: "%.1f", seconds)
	while formatted.hasSuffix("0") { formatted.removeLast() }
	if formatted.hasSuffix(".") { formatted.removeLast() }
	return formatted
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
