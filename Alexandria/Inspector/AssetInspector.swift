//
//  AssetInspector.swift
//  Alexandria
//
//  Created by ari on 9/14/26.
//

import SwiftUI
import GRDB
import GRDBQuery
import Logging

private nonisolated let log = Logger(label: "inspector")

struct AssetInspector: View {
	@Environment(\.catalog) private var catalog
	@Environment(CatalogViewState.self) private var viewState
	
	@Query<AssetRequest> private var asset: Asset?
	@Query<AssociatedFilesRequest> private var associatedFiles: [File]?
	@Query<RepresentativeFileLocationRequest> private var repFileLocation: Location?

	/// Whatever the asset elected as its representative — no view-side
	/// stand-in. Nil until formation picks one (or for a file-less asset);
	/// the body shows that state honestly rather than reinventing the choice.
	private var representativeFile: File? {
		guard let representativeFileId = asset?.representativeFileId else { return nil }
		return associatedFiles?.first(where: { $0.id == representativeFileId })
	}

	init(id: Identifier<Asset>) {
		_asset = Query(constant: AssetRequest(id: id))
		_associatedFiles = Query(constant: AssociatedFilesRequest(assetId: id))
		_repFileLocation = Query(constant: RepresentativeFileLocationRequest(assetId: id, log: log))
	}

	// TODO: Make inspector show data for multiple select (inc mixed value display)
	var body: some View {
		if let asset, let associatedFiles, let representativeFile, let repFileLocation {
			// Placeholder body — the real layout (asset display, location,
			// membership tree, keywording, metadata) is its own round.
			//
			
			// ASSET NAME
			// Preview
			//	Thumb/ Histogram (Where applicable)
			// Associated Files ("Represents")
			//	File Name
			//	Size
			// General
			//	Total Size
			//	Volume
			//	Folder
			//  Judgements
			//	Dimensions (Resolution, Aspect Ratio, Megapixel Count?)
			//	In Collections
			// Metadata
			// Map
			
			// The inspector SHOWS the cursor asset, but every judgment
			// targets the whole selection (ruled 2026-09-14); the
			// cursor stands in only when nothing is selected.
			List {
				Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 6, verticalSpacing: 10) {
					GridRow {
						metadataLabel("Rating")
						StarRating(asset.rating) { rate(viewState.judgmentTargets, $0) }
							.frame(maxWidth: .infinity, alignment: .leading)
					}
					metadataRow("Size", formatBytesAsHumanReadable(representativeFile.sizeBytes))
					metadataRow("Location", "\(repFileLocation.volume.name) > \(repFileLocation.folder.name)")
				}

				Section("Associated Files") {
					VStack(spacing: 8) {
						ForEach(associatedFiles) { file in
							HStack {
								Text(file.name)
									.padding(0)
									.padding(.leading, 2)
									.font(.system(size: 11))
								Spacer()
								if (file.id == asset.representativeFileId) {
									Button("\(file.name) is the Asset's Representative", systemImage: "circle.fill") {
										do {} // Intentional, noop
									}
									.labelStyle(.iconOnly)
									.buttonStyle(.borderless) // TODO: Diagnose no hover state
									.help("\(file.name) is the Asset's Representative")
								} else {
									Button("Change Asset Representative To \(file.name)", systemImage: "circle.dotted") {
										do {} // TODO: Logic
									}
									.labelStyle(.iconOnly) // TODO: Logic
									.buttonStyle(.borderless) // TODO: Diagnose no hover state
									.help("Change Asset Representative To \(file.name)")
									
								}
								Button("Reveal in Finder", systemImage: "folder") {
									do {} // TODO: Logic
								}
								.labelStyle(.iconOnly)
								.buttonStyle(.borderless) // TODO: Diagnose no hover state
								.help("Reveal in Finder")
							}
						}
					}
				}
				
				// Metadata sections ARE the facets (metadata round, 2026-09-17):
				// an absent facet renders nothing (evidence decides presence);
				// within a present facet, blank values are fine by ruling. Rows
				// are curated GridRows — order is line order, formatting is
				// per-field, never reflection.
				if let metadataJSON = representativeFile.metadata,
				   let metadata = FileMetadata(databaseJSON: metadataJSON) {
					if let capture = metadata.capture {
						Section("Capture") {
							Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 6, verticalSpacing: 8) {
								metadataRow("Captured", captureTimeDisplay(capture))
								metadataRow("Camera", [capture.make, capture.model].compactMap(\.self).joined(separator: " "))
								metadataRow("Serial", capture.serialNumber ?? "")
								metadataRow("Lens", capture.lensModel ?? "")
								metadataRow("Focal length", focalLengthDisplay(capture))
								metadataRow("Aperture", capture.aperture.map { "ƒ/\(formatDecimal($0))" } ?? "")
								metadataRow("Shutter", capture.exposureSeconds.flatMap(shutterSpeedDisplay(seconds:)) ?? "")
								metadataRow("Exposure bias", capture.exposureBias.map { String(format: "%+.1f EV", $0) } ?? "")
								metadataRow("ISO", capture.iso.map { "ISO \($0)" } ?? "")
								metadataRow("Flash", capture.flash.map { $0 ? "Fired" : "Did not fire" } ?? "")
								metadataRow("Program", capture.exposureProgram ?? "")
								metadataRow("Metering", capture.meteringMode ?? "")
								metadataRow("White balance", capture.whiteBalance ?? "")
							}
						}
					}
					if let visual = metadata.visual {
						Section("Visual") {
							Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 6, verticalSpacing: 8) {
								metadataRow("Dimensions", dimensionsDisplay(visual))
								metadataRow("Color space", visual.colorSpace ?? "")
								metadataRow("Color model", visual.colorModel ?? "")
								metadataRow("Bit depth", visual.bitsPerSample.map { "\($0)-bit" } ?? "")
							}
						}
					}
					if let timing = metadata.timing {
						Section("Timing") {
							Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 6, verticalSpacing: 8) {
								metadataRow("Duration", timing.durationSeconds.map(durationDisplay) ?? "")
							}
						}
					}
					if let media = metadata.media {
						Section("Media") {
							Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 6, verticalSpacing: 8) {
								metadataRow("Frame rate", media.frameRate.map { "\(formatDecimal($0)) fps" } ?? "")
								metadataRow("Video codec", media.videoCodec ?? "")
								metadataRow("Audio codec", media.audioCodec ?? "")
								metadataRow("Sample rate", media.sampleRate.map { "\(formatDecimal(Double($0) / 1000)) kHz" } ?? "")
								metadataRow("Channels", media.channelCount.map(String.init) ?? "")
							}
						}
					}
					if let audio = metadata.audio {
						Section("Audio") {
							Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 6, verticalSpacing: 8) {
								metadataRow("Album", audio.album ?? "")
								metadataRow("Track", audio.trackNumber.map(String.init) ?? "")
								metadataRow("Genre", audio.genre ?? "")
								metadataRow("Composer", audio.composer ?? "")
							}
						}
					}
					if let authorship = metadata.authorship {
						// Dual-source by ruling: TIFF and IPTC shown side by
						// side, LrC-style, never collapsed.
						Section("Authorship") {
							Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 6, verticalSpacing: 8) {
								metadataRow("Title (IPTC)", authorship.iptcTitle ?? "")
								metadataRow("Caption (IPTC)", authorship.iptcCaption ?? "")
								metadataRow("Creator (IPTC)", authorship.iptcCreator ?? "")
								metadataRow("Copyright (IPTC)", authorship.iptcCopyright ?? "")
								metadataRow("Artist (TIFF)", authorship.tiffArtist ?? "")
								metadataRow("Description (TIFF)", authorship.tiffImageDescription ?? "")
								metadataRow("Copyright (TIFF)", authorship.tiffCopyright ?? "")
								metadataRow("Software", authorship.tiffSoftware ?? "")
							}
						}
					}
					if let location = metadata.location {
						// Facet presence gates the section; blank rows within a
						// present facet are fine by ruling.
						Section("Location") {
							Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 6, verticalSpacing: 8) {
								metadataRow("Coordinates", coordinatesDisplay(location))
								metadataRow("Altitude", location.altitude.map { "\(formatDecimal($0)) m" } ?? "")
							}
						}
					}
				}
			}.listStyle(.sidebar)
		} else {
			ProgressView()
		}
	}

	/// The flag control's value: reads the observed record, writes through
	/// the verb. There is no local state to drift — the observation delivers
	/// the committed value back.
	private func flag(of asset: Asset) -> Binding<Asset.Flag?> {
		Binding(get: { asset.flag }, set: { setFlag($0, on: viewState.judgmentTargets) })
	}

	// Both verbs return the prior values for undo; undo is a later chunk, so
	// they are discarded here rather than half-kept.

	private func rate(_ ids: [Identifier<Asset>], _ rating: Int?) {
		let catalog = catalog
		Task {
			do {
				_ = try await catalog.setRating(ids, to: rating)
			} catch {
				log.error("rating failed", metadata: [
					"assets": "\(ids.count)",
					"error": "\(error)",
				])
			}
		}
	}

	private func setFlag(_ flag: Asset.Flag?, on ids: [Identifier<Asset>]) {
		let catalog = catalog
		Task {
			do {
				_ = try await catalog.setFlag(ids, to: flag)
			} catch {
				log.error("flagging failed", metadata: [
					"assets": "\(ids.count)",
					"error": "\(error)",
				])
			}
		}
	}
	
	private func formatBytesAsHumanReadable(_ bytes: Int) -> String {
		let formatter = ByteCountFormatter()
		formatter.countStyle = .file
		formatter.allowedUnits = .useAll
		return formatter.string(fromByteCount: Int64(bytes))
	}

	/// The label column of the inspector's Finder-style center line: labels
	/// right-justify to it, values left-justify from it. The line's position
	/// is measured per Grid (the widest label in that section), never fixed.
	private func metadataLabel(_ label: String) -> some View {
		Text(label)
			.foregroundStyle(.secondary)
			.font(.system(size: 10))
			.gridColumnAlignment(.trailing)
	}

	private func metadataRow(_ label: String, _ value: String) -> some View {
		GridRow {
			metadataLabel(label)
			Text(value)
				.font(.system(size: 10))
				.textSelection(.enabled)
				.frame(maxWidth: .infinity, alignment: .leading)
		}
	}

	/// Wall-clock capture time as recorded, with the zone offset shown when
	/// the file stated one — never converted through the viewer's zone.
	private func captureTimeDisplay(_ capture: CaptureFacet) -> String {
		guard let capturedAt = capture.capturedAt else { return "" }
		let base = captureTimeFormatter.string(from: capturedAt)
		guard let offset = capture.captureOffset else { return base }
		return "\(base) (\(offset))"
	}

	private func coordinatesDisplay(_ location: LocationFacet) -> String {
		guard let lat = location.latitude, let lon = location.longitude else { return "" }
		return String(format: "%.5f, %.5f", lat, lon)
	}

	private func focalLengthDisplay(_ capture: CaptureFacet) -> String {
		guard let focal = capture.focalLength else { return "" }
		let base = "\(formatDecimal(focal)) mm"
		guard let equivalent = capture.focalLength35mm, Double(equivalent) != focal else { return base }
		return "\(base) (\(equivalent) mm eq.)"
	}

	private func dimensionsDisplay(_ visual: VisualFacet) -> String {
		guard let width = visual.width, let height = visual.height else { return "" }
		// Stored dimensions are encoded; display them upright.
		let swaps = visual.orientation?.swapsAxes == true
		return swaps ? "\(height) × \(width)" : "\(width) × \(height)"
	}

	private func durationDisplay(_ seconds: Double) -> String {
		// Int(Double) traps outside Int's range; blob values are observed
		// data, so absurd-but-finite input degrades to raw text, not a crash.
		guard seconds.isFinite, seconds.magnitude < 1e12 else { return "\(seconds) s" }
		let total = Int(seconds.rounded())
		let (hours, minutes, secs) = (total / 3600, (total % 3600) / 60, total % 60)
		return hours > 0
			? String(format: "%d:%02d:%02d", hours, minutes, secs)
			: String(format: "%d:%02d", minutes, secs)
	}

	/// Whole numbers without a trailing ".0", fractions with one decimal.
	private func formatDecimal(_ value: Double) -> String {
		guard value.isFinite, value.magnitude < 1e15 else { return "\(value)" }
		return value.truncatingRemainder(dividingBy: 1) == 0
			? String(Int(value))
			: String(format: "%.1f", value)
	}
}

/// Cached (per-render allocation is cheap but pointless); wall-clock is
/// UTC-labelled, so the formatter never touches the viewer's zone.
private nonisolated let captureTimeFormatter: DateFormatter = {
	let formatter = DateFormatter()
	formatter.locale = Locale(identifier: "en_US_POSIX")
	formatter.timeZone = TimeZone(identifier: "UTC")
	formatter.dateFormat = "yyyy-MM-dd HH:mm:ss"
	return formatter
}()

// MARK: - Shutter display (presentation of capture.exposure_seconds)

/// Renders an exposure as a human shutter speed. Fast exposures snap to the
/// nearest standard speed within a bounded tolerance (APEX rationals record
/// 1/101 for a nominal 1/100); outside tolerance the raw value renders
/// as-is — the snap is presentation, the stored Double stays exact.
/// Lives with the inspector because display derivation is a view concern
/// (metadata round, 2026-09-17); internal so tests pin the ladder.
nonisolated func shutterSpeedDisplay(seconds: Double) -> String? {
	guard seconds > 0, seconds.isFinite else { return nil }
	if seconds < 1 {
		let reciprocal = 1 / seconds
		if let standard = standardShutterDenominators.min(by: {
			abs($0 - reciprocal) < abs($1 - reciprocal)
		}), abs(standard - reciprocal) / standard <= 0.05 {
			return "1/\(Int(standard))"
		}
		// Off the ladder: slow fractions read as decimals (cameras mark
		// 0.4", not 1/2.5); genuinely odd fast speeds render raw.
		if seconds >= 0.3 { return String(format: "%.1f", seconds) }
		return "1/\(Int(reciprocal.rounded()))"
	}
	guard seconds < 1e6 else { return "\(seconds)" }
	var formatted = String(format: "%.1f", seconds)
	while formatted.hasSuffix("0") { formatted.removeLast() }
	if formatted.hasSuffix(".") { formatted.removeLast() }
	return formatted
}

/// The 1/3-stop shutter ladder cameras actually mark, as denominators.
private nonisolated let standardShutterDenominators: [Double] = [
	8000, 6400, 5000, 4000, 3200, 2500, 2000, 1600, 1250, 1000,
	800, 640, 500, 400, 320, 250, 200, 160, 125, 100,
	80, 60, 50, 40, 30, 25, 20, 15, 13, 10, 8, 6, 5, 4, 3, 2,
]
