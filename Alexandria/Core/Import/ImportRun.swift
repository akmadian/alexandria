//
//  ImportTask.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation
import Algorithms
internal import UniformTypeIdentifiers

@MainActor @Observable
final class ImportRun {
	let id = Identifier<Import>.mint()  // = the imports row id: one identity, carried everywhere
	private var folderUrl: URL
	private var catalog: Catalog
	private(set) var totalFiles = 0
	private(set) var completedFiles = 0
	private var task: Task<Void, Error>?
	
	init(folderUrl: URL, catalog: Catalog) {
		print("ImportRun: initialized with id \(id)")
		self.folderUrl = folderUrl
		self.catalog = catalog
		
		// Check volume tracked state, if not tracked, probably track it
		// Probably gather some information about import size, how many files etc?
	}
	
	func start() {
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

			let discovered: [URL] = await self.walk(source: self.folderUrl)
			self.totalFiles = discovered.count

			for batch in discovered.chunks(ofCount: 200) {
				_ = batch  // TODO(ari): recordBatch — the recording slice
			}
		}
	}

	@concurrent
	private func probeVolume(containing url: URL) async throws -> ObservedVolume {
		try ObservedVolume(containing: url)
	}
	
	@concurrent
	func walk(source: URL) async -> [URL] {
		let enumerator = FileManager.default.enumerator(
			at: source,
			includingPropertiesForKeys: [.contentTypeKey, .fileAllocatedSizeKey, .nameKey]
		)!
		return enumerator.map { $0 as! URL }
//		var allContentTypes: Set<String> = []
//		var allMimeTypes: Set<String> = []
//		var allFileExtensions: Set<String> = []
//		var discovered: [String] = []
//		
//		for fileUrl in allUrls {
//			let keys: Set<URLResourceKey> = [
//				.contentTypeKey,
//				.fileAllocatedSizeKey,
//				.nameKey,
//				
//				.volumeURLKey,
//				.volumeUUIDStringKey,
//				.volumeNameKey,
//				.volumeIsInternalKey,
//				.volumeIsLocalKey
//			]
//			let values = try? fileUrl.resourceValues(forKeys: keys)
//			let contentType: String = values?.contentType?.identifier ?? ""
//			let mimeType: String = values?.contentType?.preferredMIMEType ?? ""
//			let file_extension: String = values?.contentType?.preferredFilenameExtension ?? ""
//			allContentTypes.insert(contentType)
//			allMimeTypes.insert(mimeType)
//			allFileExtensions.insert(file_extension)
//			
//			discovered.append(DiscoveredFile(url: fileUrl, size: <#T##Int64#>))
//		}
//		print(allContentTypes)
//		print(allMimeTypes)
//		print(allFileExtensions)
//		// Given an importsource, unroll to files
//		// Files should have
//		//	- file registry key (kind)
//		//	- siz
//		return []
	}
	
	

	func cancel() { task?.cancel() }
}
