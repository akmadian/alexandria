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

struct Location {
	let volume: Volume
	let folder: Folder
}

/// Observes one asset by id. Feeds from the `\.databaseContext` environment
/// (injected at the app root), so the view re-renders whenever that asset's
/// record changes.
struct AssetRequest: ValueObservationQueryable {
	static var defaultValue: Asset? { nil }

	var id: Identifier<Asset>

	func fetch(_ db: Database) throws -> Asset? {
		try Asset.fetchOne(db, key: id)
	}
}

struct AssociatedFilesRequest: ValueObservationQueryable {
	static var defaultValue: [File]? { nil }
	
	var assetId: Identifier<Asset>
	
	func fetch(_ db: Database) throws -> [File]? {
		try File
			.filter(Column("asset_id") == assetId)
			.fetchAll(db)
	}
}

struct RepresentativeFileLocationRequest: ValueObservationQueryable {
	static var defaultValue: Location? { nil }
	
	var assetId: Identifier<Asset>
	
	func fetch(_ db: Database) throws -> Location? {
		do {
			guard let asset = try Asset.fetchOne(db, key: assetId) else {
				log.warning("location: asset not found", metadata: ["asset": "\(assetId)"]); return nil
			}
			guard let fileId = asset.representativeFileId else {
				log.warning("location: no representative file", metadata: ["asset": "\(assetId)"]); return nil
			}
			guard let file = try File.fetchOne(db, key: fileId) else {
				log.error("location: file missing", metadata: ["file": "\(fileId)"]); return nil
			}
			guard let folder = try Folder.fetchOne(db, key: file.folderId) else {
				log.error("location: folder missing", metadata: ["folder": "\(file.folderId)"]); return nil
			}
			guard let volume = try Volume.fetchOne(db, key: folder.volumeId) else {
				log.error("location: volume missing", metadata: ["volume": "\(folder.volumeId)"]); return nil
			}
			return Location(volume: volume, folder: folder)
		} catch {
			// A decode/query throw would otherwise vanish into the Query's error state.
			log.error("location fetch failed", metadata: ["asset": "\(assetId)", "error": "\(error)"])
			throw error
		}
	}
}

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
		_repFileLocation = Query(constant: RepresentativeFileLocationRequest(assetId: id))
	}

	var body: some View {
		if let asset, let associatedFiles {
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
			VStack(spacing: 8) {
				Text(representativeFile?.fileStem ?? "No representative file")
				
				DisclosureGroup("Information") {
					List {
						LabeledContent("Rating") { StarRating(asset.rating) { rate(viewState.judgmentTargets, $0) } }
						if let representativeFile {
							LabeledContent("Size", value: formatBytesAsHumanReadable(representativeFile.sizeBytes))
						}
						LabeledContent("Location", value: repFileLocation.map {
							"\($0.volume.name) / \($0.folder.name)"
						} ?? "—")
					}

					Picker("Flag", selection: flag(of: asset)) {
						Label("None", systemImage: "flag.slash")
							.tag(Asset.Flag?.none)
						Label("Pick", systemImage: "flag.fill")
							.tag(Optional(Asset.Flag.pick))
						Label("Reject", systemImage: "xmark")
							.tag(Optional(Asset.Flag.reject))
					}
					.pickerStyle(.segmented)
				}
				// The inspector SHOWS the cursor asset, but every judgment
				// targets the whole selection (ruled 2026-09-14); the
				// cursor stands in only when nothing is selected.
				
				DisclosureGroup("Associated Files") {
					ForEach(associatedFiles) { file in Text(file.name) }
				}
			}
			.padding()
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
}
