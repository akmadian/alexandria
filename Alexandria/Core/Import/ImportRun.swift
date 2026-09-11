//
//  ImportTask.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation
import Algorithms
import Logging
internal import UniformTypeIdentifiers

@MainActor @Observable
final class ImportRun {
	private let log = Logger(label: "import")
	
	let id = Identifier<Import>.mint()  // = the imports row id: one identity, carried everywhere
	private var folderUrl: URL
	private var catalog: Catalog
	private(set) var totalFiles = 0
	private(set) var completedFiles = 0
	private var task: Task<Void, Error>?
	
	init(folderUrl: URL, catalog: Catalog) {
		self.log.info("New import run initialized", metadata: ["importId": "\(id.rawValue)"])
		self.folderUrl = folderUrl
		self.catalog = catalog
		
		// Check volume tracked state, if not tracked, probably track it
		// Probably gather some information about import size, how many files etc?
	}
	
	func start() {
		self.log.info("Starting import run")
		task = Task(name: "import-\(id.rawValue)") {
			// Phase 0: residence, then the run's bracket. Fail fast before
			// any walking — probe (disk, off-main), then record (transactions).
			let observed = try await self.probeVolume(containing: self.folderUrl)
			let volumeId = try await self.catalog.findOrCreateVolume(observed)
			let rootFolderId = try await self.catalog.findOrCreateRootFolder(
				named: self.folderUrl.lastPathComponent,
				on: volumeId,
				rootPath: observed.relativePath(of: self.folderUrl)
			)
			try await self.catalog.recordImportStarted(id: self.id, folderId: rootFolderId)

			var discovered: [DiscoveredFile] = []
			do {
				discovered = try await self.walk(dirSource: self.folderUrl)
				self.totalFiles = discovered.count
				self.log.info("Discovered \(self.totalFiles) files in \(self.folderUrl.lastPathComponent)")
			} catch {
				self.log.error("Failed to walk dir", metadata: [
					"dir": "\(self.folderUrl)",
					"error": "\(error)"
				])
				throw error
			}

			for batch in discovered.chunks(ofCount: 200) {
				_ = batch
			}
		}
	}

	@concurrent
	private func probeVolume(containing url: URL) async throws -> ObservedVolume {
		try ObservedVolume(containing: url)
	}
	
	@concurrent
	func walk(dirSource: URL) async throws -> [DiscoveredFile] {
		var discovered: [DiscoveredFile] = []
		
		guard let enumerator = FileManager.default.enumerator(
			at: dirSource,
			includingPropertiesForKeys: [
				.contentTypeKey,
				.fileSizeKey,
				.contentModificationDateKey,
				.isDirectoryKey
			],
			options: [.skipsHiddenFiles, .skipsPackageDescendants],
			errorHandler: { url, error in
				self.log.error("Failed to enumerate dir", metadata: [
					"url": "\(url)",
					"error": "\(error)"
				])
				// TODO: import_errors
				return true
			}
		) else {
			throw ImportError.sourceUnreadable
		}

		while let fileUrl = enumerator.nextObject() as? URL {
			let resourceValues: URLResourceValues
			do {
				resourceValues = try fileUrl.resourceValues(forKeys: [
					.contentTypeKey,
					.fileSizeKey,
					.contentModificationDateKey,
					.isDirectoryKey
				])
			} catch {
				self.log.warning("Failed to read resource values", metadata: [
					"url": "\(fileUrl)",
					"error": "\(error)"
				])
				continue
				// TODO: import_errors
			}
			
			if resourceValues.isDirectory == true {
				continue
			}
			
			guard let size = resourceValues.fileSize,
				  let mtime = resourceValues.contentModificationDate else {
				self.log.warning("Missing size or mtime for file", metadata: ["url": "\(fileUrl)"])
				continue
				// TODO: import_errors
			}
			
			let format = FileFormat.resolve(
				extension: fileUrl.pathExtension,
				contentType: resourceValues.contentType
			)

			discovered.append(
				DiscoveredFile(
					url: fileUrl,
					size: size,
					modifiedAt: mtime,
					format: format
				)
			)
		}
		
		return discovered
	}

	func cancel() { task?.cancel() }
}
