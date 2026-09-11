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
	
	/// The root variant: keyed (volume_id, root_path) per idx_folders_root.
	/// parentId nil + rootPath set is the shape CHECK's root arm.
	func findOrCreateRootFolder(
		named name: String,
		on volumeId: Identifier<Volume>,
		rootPath: String
	) async throws -> Identifier<Folder> {
		try await databaseWriter.write { database in
			if let existing = try Folder
				.filter(Folder.Columns.volumeId == volumeId)
				.filter(Folder.Columns.rootPath == rootPath)
				.fetchOne(database)
			{
				return existing.id
			}

			let folder = Folder(
				id: .mint(),
				volumeId: volumeId,
				parentId: nil,
				name: name,
				nameKey: name.precomposedStringWithCanonicalMapping,
				rootPath: rootPath
			)
			try folder.insert(database)
			return folder.id
		}
	}
	
	func createFolder(folder: Folder) async throws -> Folder? {
		try await databaseWriter.write { database in
			return try? folder.saved(database)
		}
	}
}
