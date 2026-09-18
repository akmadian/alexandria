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
@MainActor @Observable
final class ImportService {
	@ObservationIgnored private let catalog: Catalog
	@ObservationIgnored private let log = Logger(label: "ImportService")

	/// Runs with something left to show: executing, or finished and
	/// lingering until dismissed. Keyed by the run's root folder — the
	/// browser row's lookup key. Session-scoped on purpose: relaunch clears
	/// lingering chrome; the imports table is the durable unfinished signal.
	private(set) var runs: [Identifier<Folder>: ImportRun] = [:]
	/// The in-flight reservation: startImport suspends four times between
	/// its guard and its registry write, and MainActor reentrancy would let
	/// two rapid invocations both pass an empty-registry guard (review
	/// finding, 2026-09-18). Flipped synchronously before the first await,
	/// cleared on every exit.
	@ObservationIgnored private var startInFlight = false

	init(catalog: Catalog) {
		self.catalog = catalog
	}

	/// The lingering check/alert's dismiss — removal IS the state change.
	/// A folder whose latest import is still unfinished then degrades to
	/// the tree-fed resume badge; nothing strands.
	func dismiss(folder id: Identifier<Folder>) {
		if let run = runs.removeValue(forKey: id) {
			log.debug("Import chrome dismissed", metadata: [
				"importId": "\(run.id.rawValue)",
			])
		}
	}

	@discardableResult
	func startImport(of folderUrl: URL) async throws -> ImportRun {
		// Serialized (ruled 2026-09-18): one executing run at a time —
		// lingering finished runs don't block. Guard and reservation are one
		// synchronous step; no await sits between them.
		// TODO: when concurrency lands, a second import against the same
		// network share warns ("running a second may make both slower")
		// instead of refusing outright.
		guard !startInFlight, !runs.values.contains(where: { !$0.isFinished }) else {
			log.info("Import refused: one already running", metadata: [
				"source": "\(folderUrl.path())",
			])
			throw ImportError.importAlreadyRunning
		}
		startInFlight = true
		defer { startInFlight = false }
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
		// Registered BEFORE start(): the registry entry must mask the DB's
		// unfinished flag from the first frame, or a resume flashes the
		// alert badge. Replaces this folder's lingering finished run, if any.
		runs[rootFolderId] = run
		run.start()
		return run
	}

	@concurrent
	private func probeVolume(containing url: URL) async throws -> ObservedVolume {
		try ObservedVolume(containing: url)
	}
}
