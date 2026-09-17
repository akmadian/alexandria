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
				VStack(spacing: 10) {
					LabeledContent("Rating") { StarRating(asset.rating) { rate(viewState.judgmentTargets, $0) } }
					LabeledContent("Size", value: formatBytesAsHumanReadable(representativeFile.sizeBytes))
					LabeledContent("Location", value: "\(repFileLocation.volume.name) > \(repFileLocation.folder.name)")
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
				
				Section("Metadata") {
					if let metadataJSON = representativeFile.metadata,
					   let metadata = FileMetadata(databaseJSON: metadataJSON) {
						VStack(spacing: 8) {
							ForEach(metadataFields(from: metadata), id: \.key) { field in
								LabeledContent {
									Text(field.value)
										.font(.system(size: 10))
								} label: {
									Text(field.key)
										.foregroundStyle(.secondary)
										.font(.system(size: 10))
								}
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

	/// Reflects over a `FileMetadata` value and returns display pairs for every
	/// non-nil field, in declaration order. The key is the camelCase property
	/// name split into words; the value is the default string representation.
	/// New fields on `FileMetadata` appear here automatically.
	private func metadataFields(from metadata: FileMetadata) -> [(key: String, value: String)] {
		Mirror(reflecting: metadata).children.compactMap { child in
			guard let label = child.label else { return nil }
			// child.value is typed as Any; unwrap Optional regardless of T.
			let valueMirror = Mirror(reflecting: child.value)
			guard valueMirror.displayStyle == .optional,
				  let wrapped = valueMirror.children.first?.value else { return nil }
			let key = label
				.replacingOccurrences(of: "([A-Z])", with: " $1", options: .regularExpression)
				.trimmingCharacters(in: .whitespaces)
				.capitalized
			return (key: key, value: "\(wrapped)")
		}
	}
}
