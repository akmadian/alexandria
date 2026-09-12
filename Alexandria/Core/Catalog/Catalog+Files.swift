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

/// One thumbnail worklist entry: the file plus its directory path relative
/// to the import's root folder ('' = the root itself) — recomposed from the
/// folder tree so the pass can reach the bytes on disk.
nonisolated struct PendingThumbnail: Sendable {
	let file: File
	let relativeDirectory: String

	/// The file's on-disk location under the import's root URL.
	func url(under rootUrl: URL) -> URL {
		var url = rootUrl
		for component in relativeDirectory.split(separator: "/") {
			url.append(path: String(component))
		}
		return url.appending(path: file.name)
	}
}

extension Catalog {
	/// Every file of one import — asset formation's input: the unformed are
	/// its worklist, the formed are join targets.
	func files(inImport importId: Identifier<Import>) async throws -> [File] {
		try await reader.read { database in
			try File.filter(File.Columns.importId == importId).fetchAll(database)
		}
	}

	/// The thumbnail worklist (thumbnails.md): pending = `thumbnail_at IS
	/// NULL`, minus missing files, minus kinds the registry never thumbnails,
	/// minus files with residue — one attempt per file per import, the
	/// file_errors table is the DLQ. Ordered by id (UUIDv7 = record order):
	/// import-ordered, the ratified p0 ordering.
	func thumbnailPending(
		inImport importId: Identifier<Import>,
		under rootFolderId: Identifier<Folder>,
		limit: Int
	) async throws -> [PendingThumbnail] {
		let kinds = FileFormat.thumbnailingKinds.map(\.rawValue).sorted()
		let request: SQLRequest<Row> = """
			WITH RECURSIVE tree(id, path) AS (
			    SELECT id, '' FROM folders WHERE id = \(rootFolderId)
			    UNION ALL
			    SELECT folders.id, tree.path || '/' || folders.name
			    FROM folders JOIN tree ON folders.parent_id = tree.id
			)
			SELECT files.*, tree.path AS directory_path
			FROM files JOIN tree ON files.folder_id = tree.id
			WHERE files.import_id = \(importId)
			  AND files.thumbnail_at IS NULL
			  AND files.missing = 0
			  AND files.kind IN \(kinds)
			  AND NOT EXISTS (
			      SELECT 1 FROM file_errors
			      WHERE file_errors.file_id = files.id AND file_errors.task = 'thumbnail'
			  )
			ORDER BY files.id
			LIMIT \(limit)
			"""
		return try await reader.read { database in
			try Row.fetchAll(database, request).map { row in
				PendingThumbnail(file: try File(row: row), relativeDirectory: row["directory_path"])
			}
		}
	}

	/// One transaction per drained batch — the stamp granularity IS the UI's
	/// shimmer wave (thumbnails.md invariant 5). Failures land as file_errors
	/// residue and the worklist exclusion keeps them excluded: one attempt
	/// per file per import, NO retry counting (ruled 2026-09-11) — the
	/// conflict clause is crash-proofing only, refreshing what happened last.
	func recordThumbnails(
		generated: [Identifier<File>],
		failures: [(fileId: Identifier<File>, reasonCode: String, message: String)]
	) async throws {
		guard !generated.isEmpty || !failures.isEmpty else { return }
		let stampedAt = catalogTimestamp()
		try await databaseWriter.write { database in
			try File
				.filter(generated.contains(File.Columns.id))
				.updateAll(database, File.Columns.thumbnailAt.set(to: stampedAt))
			for failure in failures {
				try database.execute(
					sql: """
						INSERT INTO file_errors (file_id, task, reason_code, message)
						VALUES (?, 'thumbnail', ?, ?)
						ON CONFLICT (file_id, task) DO UPDATE SET
						    reason_code = excluded.reason_code,
						    message = excluded.message
						""",
					arguments: [failure.fileId, failure.reasonCode, failure.message]
				)
			}
		}
	}

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
				
				// Files commit unassetted: asset formation is a distinct pass
				// after the batch (AssetFormation.form via ImportRun.formAssets),
				// never chained in here.
				let record = File(
					id: .mint(), folderId: folderId, assetId: nil, importId: importId,
					name: file.name, nameKey: file.nameKey,
					fileStem: file.fileStem, fileExtension: file.fileExtension,
					kind: file.discovered.format.kind,
					sizeBytes: file.discovered.size, modifiedAt: file.discovered.modifiedAt,
					contentHash: file.contentHash, missing: false,
					metadata: try file.metadata?.databaseJSON(),
					thumbnailAt: nil, formationRule: nil
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
