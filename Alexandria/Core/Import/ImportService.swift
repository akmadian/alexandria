//
//  ImportService.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation
import Logging

/// The import verb's entry point: a run is identified by its source, so the
/// service resolves source → identity BEFORE the run exists — residence
/// (probe, volume, root folder), then the unfinished job for that root, or a
/// fresh mint (resume ruling, 2026-09-11: restarting the same source picks
/// the unfinished task back up under its original id, and every import-scoped
/// worklist — formation and thumbnails — completes with it).
@MainActor
final class ImportService {
	private let catalog: Catalog
	private let log = Logger(label: "ImportService")

	init(catalog: Catalog) {
		self.catalog = catalog
	}

	@discardableResult
	func startImport(of folderUrl: URL) async throws -> ImportRun {
		// The store is REQUIRED (ruled 2026-09-11): the grid is the product,
		// and a run that could skip thumbnails would ship a wall of shimmer.
		// A directoryless (in-memory) catalog therefore cannot run imports.
		guard let catalogDirectory = catalog.directory else {
			throw ImportError.catalogHasNoDirectory
		}
		let store = ThumbnailStore(catalogDirectory: catalogDirectory)
		let observed = try await self.probeVolume(containing: folderUrl)
		let volumeId = try await catalog.findOrCreateVolume(observed)
		let rootFolderId = try await catalog.findOrCreateRootFolder(
			named: folderUrl.lastPathComponent,
			on: volumeId,
			rootPath: observed.relativePath(of: folderUrl)
		)
		let unfinished = try await catalog.unfinishedImport(inFolder: rootFolderId)
		if let unfinished {
			log.info("Unfinished import found for source; resuming it", metadata: [
				"importId": "\(unfinished.rawValue)",
				"source": "\(folderUrl.path())",
			])
		}
		let run = ImportRun(
			id: unfinished ?? .mint(),
			folderUrl: folderUrl,
			rootFolderId: rootFolderId,
			volume: observed,
			catalog: catalog,
			store: store,
			resuming: unfinished != nil
		)
		run.start()
		return run
	}

	@concurrent
	private func probeVolume(containing url: URL) async throws -> ObservedVolume {
		try ObservedVolume(containing: url)
	}
}
