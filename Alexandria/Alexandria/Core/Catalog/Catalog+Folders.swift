//
//  Catalog+Folders.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation
import GRDB

extension Catalog {
	func findOrCreateFolder(
		named name: String,
		under parentId: Identifier<Folder>,
		on volumeId: Identifier<Volume>
	) async throws -> Identifier<Folder> {
		try await databaseWriter.write { database in
			let nameKey = name.precomposedStringWithCanonicalMapping
			
			if let existing = try Folder
				.filter(Folder.Columns.parentId == parentId)
				.filter(Folder.Columns.nameKey == nameKey)
				.fetchOne(database)
			{
				return existing.id
			}
			
			let folder = Folder(
				id: .mint(),
				volumeId: volumeId,
				parentId: parentId,
				name: name,
				nameKey: nameKey,
				rootPath: nil
			)
			try folder.insert(database)
			return folder.id
		}
	}
	
	func createFolder(folderUrl: URL) async throws -> Folder {
		
	}
	
	func createFolder(folder: Folder) async throws -> Folder? {
		try await databaseWriter.write { database in
			return try? folder.saved(database)
		}
	}
}
