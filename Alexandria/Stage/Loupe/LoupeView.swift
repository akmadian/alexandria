//
//  LoupeView.swift
//  Alexandria
//
//  Created by ari on 9/16/26.
//

import SwiftUI
import GRDB
import GRDBQuery
import Logging

private nonisolated let log = Logger(label: "loupe")

struct LoupeView: View {
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
	
	var body: some View {
		if let asset, let associatedFiles {
			switch asset.kind {
			case .image:
				LoupeFallbackView()
			case .video:
				LoupeFallbackView()
			case .audio:
				LoupeFallbackView()
			case .document:
				LoupeFallbackView()
			case .project:
				LoupeFallbackView()
			case .vector:
				LoupeFallbackView()
			case .sidecar:
				LoupeFallbackView()
			case .other:
				LoupeFallbackView()
			}
		} else {
			ProgressView()
		}
	}
}
