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
	let id = UUID.v7()
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
		task = Task(name: "import-\(id)") {
			var folder: Folder = try! await self.catalog.createFolder(folderUrl: self.folderUrl)
			
			let discovered: [URL] = await walk(source: self.folderUrl)
			self.totalFiles = discovered.count
			
			for batch in discovered.chunks(ofCount: 200) {
				
			}
		}
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
