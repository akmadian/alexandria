//
//  ExiftoolReader.swift
//  Alexandria
//

import Foundation
import Logging
import os

private nonisolated let log = Logger(label: "metadata.exiftool")

/// Reads the EXIF block that cameras embed in video containers — vendor
/// atoms AVFoundation cannot see (Fuji's MVTG lineage, Canon CNTH, …).
/// exiftool is the one reader that knows every vendor's layout; this type
/// asks it for exactly the tags with a facet home and nothing else — the
/// long tail never crosses this boundary (metadata.md, storage ruling).
///
/// The tool is an optional capability, the thumbnails-lane posture: absent
/// binary or a failed run yields nil — absent fields, never an error and
/// never DLQ residue on its own. AVFoundation remains the authority for
/// geometry, codecs, and timing; this reader only answers what the moov
/// can't: capture, authorship, and (gap-fill) location.
nonisolated struct ExiftoolReader: Sendable {
	/// nil = exiftool not installed; every read yields nil.
	let binary: URL?

	/// Process-lifetime discovery, the Homebrew/MacPorts install locations.
	/// A mid-session install is picked up on next app launch. The one log
	/// line is what makes blank camera fields diagnosable from a log alone.
	static let discovered: ExiftoolReader = {
		let binary = locate()
		log.debug("exiftool discovery", metadata: ["binary": "\(binary?.path ?? "absent")"])
		return ExiftoolReader(binary: binary)
	}()

	static var isAvailable: Bool { discovered.binary != nil }

	private static func locate() -> URL? {
		["/opt/homebrew/bin/exiftool", "/usr/local/bin/exiftool", "/opt/local/bin/exiftool"]
			.map { URL(fileURLWithPath: $0) }
			.first { FileManager.default.isExecutableFile(atPath: $0.path) }
	}

	/// Every requested tag names the facet field it feeds; a tag with no
	/// facet home is the long tail and does not belong here. `#` asks for
	/// the numeric (unconverted) value per tag; Composite GPS folds the
	/// hemisphere refs into signed decimal degrees, the LocationFacet form.
	private static let arguments: [String] = [
		"-j",
		"-api", "largefilesupport=1",  // camera clips routinely exceed 4GB
		// capture
		"-EXIF:Make", "-EXIF:Model", "-EXIF:SerialNumber",
		"-EXIF:LensModel", "-EXIF:LensInfo",
		"-EXIF:FocalLength#", "-EXIF:FocalLengthIn35mmFormat#",
		"-EXIF:FNumber#", "-EXIF:ExposureTime#", "-EXIF:ExposureCompensation#",
		"-EXIF:ISO#", "-EXIF:Flash#",
		"-EXIF:ExposureProgram#", "-EXIF:MeteringMode#", "-EXIF:WhiteBalance#",
		// SubSecTimeOriginal is deliberately unrequested: exiftool's JSON
		// numerifies "057" to 57 — an undetectable 10x corruption of the
		// fold — and video has no burst-order need.
		"-EXIF:DateTimeOriginal", "-EXIF:OffsetTimeOriginal",
		// authorship — dual-source by ruling: TIFF and IPTC side by side
		"-IFD0:Artist", "-IFD0:ImageDescription", "-IFD0:Copyright", "-IFD0:Software",
		"-IPTC:By-line", "-IPTC:Caption-Abstract", "-IPTC:ObjectName", "-IPTC:CopyrightNotice",
		// location — gap-fill for cameras writing a GPS IFD instead of ISO 6709
		"-Composite:GPSLatitude#", "-Composite:GPSLongitude#", "-Composite:GPSAltitude#",
	]

	/// One spawn per file. Best-effort: launch failure, non-zero exit,
	/// timeout, and undecodable output all log at debug and yield nil.
	// PERF: per-file spawn costs Perl startup (~200ms); batch `-j` over an
	// import's videos, or exiftool's -stay_open daemon, if video-heavy
	// imports drag.
	func readEmbeddedExif(from url: URL) async -> EmbeddedExif? {
		guard let binary else { return nil }
		let process = Process()
		process.executableURL = binary
		process.arguments = Self.arguments + [url.path]
		let stdout = Pipe()
		let stderr = Pipe()
		process.standardOutput = stdout
		process.standardError = stderr

		// `launched` gates terminate(): calling it — like reading
		// terminationStatus — on a never-launched Process raises an
		// uncatchable ObjC exception and aborts the app.
		let launched = OSAllocatedUnfairLock(initialState: false)
		let terminateIfLaunched: @Sendable () -> Void = {
			if launched.withLock({ $0 }) { process.terminate() }
		}

		// nil = never launched; terminationStatus must not be read then.
		var watchdog: Task<Void, Never>?
		let data: Data? = await withTaskCancellationHandler {
			await withCheckedContinuation { continuation in
				process.terminationHandler = { _ in
					// Safe to drain after exit: the explicit tag list bounds
					// stdout far below the 64KB pipe buffer, so the process
					// never blocks on a full pipe.
					continuation.resume(returning: (try? stdout.fileHandleForReading.readToEnd()) ?? Data())
				}
				do {
					try process.run()
				} catch {
					log.debug("exiftool launch failed", metadata: [
						"url": "\(url.lastPathComponent)", "error": "\(error)",
					])
					continuation.resume(returning: nil)
					return
				}
				launched.withLock { $0 = true }
				// Wall-clock ceiling: a child hung on a stalled volume must
				// not park an import worker forever. SIGTERM fires the
				// termination handler; the non-zero status logs below.
				watchdog = Task {
					try? await Task.sleep(for: .seconds(30))
					guard !Task.isCancelled else { return }
					terminateIfLaunched()
				}
			}
		} onCancel: {
			// A cancelled import reclaims its worker immediately.
			terminateIfLaunched()
		}
		watchdog?.cancel()
		guard let data else { return nil }  // never launched; already logged

		guard process.terminationStatus == 0, !data.isEmpty else {
			// stderr drained only after exit, and only here: exiftool's
			// warnings are a handful of lines, nowhere near the 64KB pipe
			// buffer that could block the child mid-run.
			let reason = (try? stderr.fileHandleForReading.readToEnd())
				.map { String(decoding: $0, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines) }
			log.debug("exiftool read failed", metadata: [
				"url": "\(url.lastPathComponent)",
				"status": "\(process.terminationStatus)",
				"stderr": "\(reason ?? "")",
			])
			return nil
		}
		guard let tags = try? JSONDecoder().decode([EmbeddedExif].self, from: data).first else {
			log.debug("exiftool output undecodable", metadata: ["url": "\(url.lastPathComponent)"])
			return nil
		}
		return tags
	}
}

/// The embedded-EXIF answer, one property per requested tag. Every value
/// decodes through `TagValue` because exiftool's JSON typing follows the
/// file, not the tag: a mangled field arrives as the wrong scalar kind and
/// must read as "the file doesn't say", never fail the whole decode.
nonisolated struct EmbeddedExif: Decodable, Sendable {
	var make: String?
	var model: String?
	var serialNumber: String?
	var lensModel: String?
	var lensInfo: String?
	var focalLength: Double?
	var focalLength35mm: Int?
	var fNumber: Double?
	var exposureTime: Double?
	var exposureCompensation: Double?
	var iso: Int?
	var flash: Int?              // bitfield code, flashFired() collapses
	var exposureProgram: Int?    // code, exposureProgramName() maps
	var meteringMode: Int?       // code, meteringModeName() maps
	var whiteBalance: Int?       // code, whiteBalanceName() maps
	var dateTimeOriginal: String?
	var offsetTimeOriginal: String?
	var artist: String?
	var imageDescription: String?
	var copyright: String?
	var software: String?
	var iptcCreator: String?
	var iptcCaption: String?
	var iptcTitle: String?
	var iptcCopyright: String?
	var gpsLatitude: Double?
	var gpsLongitude: Double?
	var gpsAltitude: Double?

	enum CodingKeys: String, CodingKey {
		case make = "Make"
		case model = "Model"
		case serialNumber = "SerialNumber"
		case lensModel = "LensModel"
		case lensInfo = "LensInfo"
		case focalLength = "FocalLength"
		case focalLength35mm = "FocalLengthIn35mmFormat"
		case fNumber = "FNumber"
		case exposureTime = "ExposureTime"
		case exposureCompensation = "ExposureCompensation"
		case iso = "ISO"
		case flash = "Flash"
		case exposureProgram = "ExposureProgram"
		case meteringMode = "MeteringMode"
		case whiteBalance = "WhiteBalance"
		case dateTimeOriginal = "DateTimeOriginal"
		case offsetTimeOriginal = "OffsetTimeOriginal"
		case artist = "Artist"
		case imageDescription = "ImageDescription"
		case copyright = "Copyright"
		case software = "Software"
		case iptcCreator = "By-line"
		case iptcCaption = "Caption-Abstract"
		case iptcTitle = "ObjectName"
		case iptcCopyright = "CopyrightNotice"
		case gpsLatitude = "GPSLatitude"
		case gpsLongitude = "GPSLongitude"
		case gpsAltitude = "GPSAltitude"
	}

	init(from decoder: any Decoder) throws {
		let container = try decoder.container(keyedBy: CodingKeys.self)
		func string(_ key: CodingKeys) -> String? {
			trimmedString(TagValue.decode(container, key)?.stringValue)
		}
		func number(_ key: CodingKeys) -> Double? {
			// through the one finiteness gate every facet Double crosses
			double(TagValue.decode(container, key)?.numberValue)
		}
		func code(_ key: CodingKeys) -> Int? {
			number(key).map { Int($0.rounded()) }
		}
		make = string(.make)
		model = string(.model)
		serialNumber = string(.serialNumber)
		lensModel = string(.lensModel)
		lensInfo = string(.lensInfo)
		focalLength = number(.focalLength)
		focalLength35mm = code(.focalLength35mm)
		fNumber = number(.fNumber)
		exposureTime = number(.exposureTime)
		exposureCompensation = number(.exposureCompensation)
		iso = code(.iso)
		flash = code(.flash)
		exposureProgram = code(.exposureProgram)
		meteringMode = code(.meteringMode)
		whiteBalance = code(.whiteBalance)
		dateTimeOriginal = string(.dateTimeOriginal)
		offsetTimeOriginal = string(.offsetTimeOriginal)
		artist = string(.artist)
		imageDescription = string(.imageDescription)
		copyright = string(.copyright)
		software = string(.software)
		iptcCreator = string(.iptcCreator)
		iptcCaption = string(.iptcCaption)
		iptcTitle = string(.iptcTitle)
		iptcCopyright = string(.iptcCopyright)
		gpsLatitude = number(.gpsLatitude)
		gpsLongitude = number(.gpsLongitude)
		gpsAltitude = number(.gpsAltitude)
	}
}

/// A tolerant JSON scalar: whatever kind arrives, reads as the kind asked
/// for or nil. List-valued tags (IPTC By-line) key on their first element,
/// the image lane's firstElement semantics.
private nonisolated enum TagValue: Decodable {
	case string(String)
	case number(Double)
	case list([TagValue])
	case other

	init(from decoder: any Decoder) throws {
		let single = try decoder.singleValueContainer()
		if let text = try? single.decode(String.self) {
			self = .string(text)
		} else if let value = try? single.decode(Double.self) {
			self = .number(value)
		} else if let values = try? single.decode([TagValue].self) {
			self = .list(values)
		} else {
			self = .other
		}
	}

	static func decode(_ container: KeyedDecodingContainer<EmbeddedExif.CodingKeys>, _ key: EmbeddedExif.CodingKeys) -> TagValue? {
		try? container.decodeIfPresent(TagValue.self, forKey: key)
	}

	var stringValue: String? {
		switch self {
		case .string(let text): text
		case .number(let value):
			// exiftool numerifies numeric-looking strings; integral values
			// render without a fraction so an all-digit serial or model
			// round-trips exactly, never as "1234567890.0".
			value == value.rounded() && value.magnitude < 1e15
				? String(Int64(value)) : String(value)
		case .list(let values): values.first?.stringValue
		case .other: nil
		}
	}

	var numberValue: Double? {
		switch self {
		case .number(let value): value
		case .string(let text): Double(text)
		case .list(let values): values.first?.numberValue
		case .other: nil
		}
	}
}
