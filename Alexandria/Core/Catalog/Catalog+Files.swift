//
//  Catalog+Files.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation
import GRDB

nonisolated struct BatchOutcome: Sendable {
	var recorded = 0
	var skipped = 0
	var failed = 0
}

extension Catalog {
	func recordNewFileBatch(
		_ prepared: [PreparedFile],
		importId: Identifier<Import>,
		rootFolderId: Identifier<Folder>,
		rootUrl: URL
	) async throws -> BatchOutcome {
		let rootPath = rootUrl.standardizedFileURL.path(percentEncoded: false)
		return try await databaseWriter.write { database in
			var outcome = BatchOutcome()
			var folderCache: [String: Identifier<Folder>] = [:]
			
			for file in prepared {
				// Parent folder: mint the chain from root to this file's directory.
				let directoryPath = file.discovered.url.deletingLastPathComponent()
					.standardizedFileURL.path(percentEncoded: false)
				let relative = String(directoryPath.dropFirst(rootPath.count))
					.split(separator: "/").map(String.init)
				let folderId = try Self.folderId(
					in: database, forChain: relative, under: rootFolderId, cache: &folderCache
				)
				
				// Skip-existing (ruling B): already cataloged = not an error.
				let exists = try File
					.filter(File.Columns.folderId == folderId)
					.filter(File.Columns.nameKey == file.nameKey)
					.fetchOne(database) != nil
				if exists {
					outcome.skipped += 1
					continue
				}
				
				// Formation, v0 rule: every non-sidecar file gets its own asset.
				var assetId: Identifier<Asset>? = nil
				var formationRule: String? = nil
				if file.discovered.format.kind != .sidecar {
					let asset = Asset(id: .mint(), kind: file.discovered.format.kind.rawValue,
									  rating: nil, flag: nil, representativeFileId: nil)
					try asset.insert(database)
					assetId = asset.id
					formationRule = "one_asset_per_file"
				}
				
				let record = File(
					id: .mint(), folderId: folderId, assetId: assetId, importId: importId,
					name: file.name, nameKey: file.nameKey,
					fileStem: file.fileStem, fileExtension: file.fileExtension,
					kind: file.discovered.format.kind,
					sizeBytes: file.discovered.size, modifiedAt: file.discovered.modifiedAt,
					contentHash: file.contentHash, missing: false,
					metadata: try file.metadata?.databaseJSON(),
					thumbnailAt: nil, formationRule: formationRule
				)
				try record.insert(database)
				
				if file.extractionFailed {   // ruling C: post-identity residue
					try database.execute(
						sql: """
	  INSERT INTO file_errors (file_id, task, reason_code, message)
	  VALUES (?, 'metadata', 'decode_failed', 'extraction failed during import')
	  """,
						arguments: [record.id]
					)
					outcome.failed += 1
				}
				outcome.recorded += 1
			}
			return outcome
		}
	}

	/// Database-level chain minting: runs INSIDE recordNewFileBatch's
	/// transaction. Cache key is the joined relative path; volume is derived
	/// from the parent row, never passed.
	private nonisolated static func folderId(
		in database: Database,
		forChain components: [String],
		under rootId: Identifier<Folder>,
		cache: inout [String: Identifier<Folder>]
	) throws -> Identifier<Folder> {
		var parentId = rootId
		var pathKey = ""
		for component in components {
			pathKey += "/" + component
			if let cached = cache[pathKey] {
				parentId = cached
				continue
			}
			let nameKey = component.precomposedStringWithCanonicalMapping
			if let existing = try Folder
				.filter(Folder.Columns.parentId == parentId)
				.filter(Folder.Columns.nameKey == nameKey)
				.fetchOne(database)
			{
				parentId = existing.id
			} else {
				guard let parent = try Folder.fetchOne(database, key: parentId) else {
					throw ImportError.folderChainBroken(pathKey: pathKey)
				}
				let folder = Folder(id: .mint(), volumeId: parent.volumeId, parentId: parentId,
				                    name: component, nameKey: nameKey, rootPath: nil)
				try folder.insert(database)
				parentId = folder.id
			}
			cache[pathKey] = parentId
		}
		return parentId
	}
}
