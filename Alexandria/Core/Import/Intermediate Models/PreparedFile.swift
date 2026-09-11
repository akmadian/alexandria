//
//  PreparedFile.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

nonisolated struct PreparedFile: Sendable {
	let discovered: DiscoveredFile
	let name: String
	let nameKey: String
	let fileStem: String
	let fileExtension: String
	let contentHash: String?
	let metadata: FileMetadata?
	let extractionFailed: Bool
}
