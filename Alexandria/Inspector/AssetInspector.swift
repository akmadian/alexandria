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

struct AssetInspector: View {
	@Environment(\.catalog) private var catalog
	@Environment(CatalogViewState.self) private var viewState
	@Query<AssetRequest> private var asset: Asset?
	@Query<AssociatedFilesRequest> private var associatedFiles: [File]?

	init(id: Identifier<Asset>) {
		_asset = Query(constant: AssetRequest(id: id))
		_associatedFiles = Query(constant: AssociatedFilesRequest(assetId: id))
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
			VStack(alignment: .leading, spacing: 8) {
				VStack {
					Text("Files Associated with \(associatedFiles[0].nameKey)")
					ForEach(associatedFiles) { file in
						Text(file.name)
					}
				}
				// The inspector SHOWS the cursor asset, but every judgment
				// targets the whole selection (ruled 2026-09-14); the
				// cursor stands in only when nothing is selected.
				StarRating(asset.rating) { rate(viewState.judgmentTargets, $0) }
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
}
