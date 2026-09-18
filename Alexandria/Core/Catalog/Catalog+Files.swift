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
	/// Files this batch that need no further thumbnail work from THIS import
	/// — ready on arrival. Counted here, in the same transaction that knows
	/// each row's truth, because assembling it from overlapping in-memory
	/// counters double-counts on a resume (review finding, 2026-09-18): a
	/// skipped row with thumbnail_at NULL re-enters the drain and would be
	/// counted twice. The drain's own settlements (generated + residue) are
	/// the pipeline's to add; the two populations are disjoint by the
	/// worklist predicate.
	var settled = 0
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

	/// File records by id, batched for a visible window (cell round,
	/// 2026-09-18): file subjects in the grid render name and metadata
	/// straight off the canonical record, same as the inspector. An id with
	/// no row simply yields no entry — deliberately no negative caching, so
	/// a caller's miss stays re-askable. No result order is promised.
	func files(ids: [Identifier<File>]) async throws -> [File] {
		guard !ids.isEmpty else { return [] }
		return try await reader.read { database in
			try File.fetchAll(
				database,
				sql: "SELECT * FROM files WHERE id IN (\(databaseQuestionMarks(count: ids.count)))",
				arguments: StatementArguments(ids)
			)
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
		// UNION ALL knowingly, unlike the id-only subtree CTEs: the growing
		// `path` column makes every recursive row distinct, so UNION would
		// NOT terminate a parent cycle here anyway (rows never repeat) —
		// the defense is that no write path can mint a folder cycle (disk
		// truth; folders never re-parent).
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

	/// The thumbnail doorbell's confirmation (grid round, 2026-09-12): which
	/// of these files are stamped. Write-before-stamp (thumbnails.md
	/// invariant 4) means every returned id has bytes in the store — the
	/// caller's loads cannot miss, and unstamped ids never cost disk IO.
	func thumbnailedFileIds(
		among ids: [Identifier<File>]
	) async throws -> Set<Identifier<File>> {
		guard !ids.isEmpty else { return [] }
		return try await reader.read { database in
			let stamped = try Identifier<File>.fetchAll(
				database,
				File.select(File.Columns.id, as: Identifier<File>.self)
					.filter(ids.contains(File.Columns.id))
					.filter(File.Columns.thumbnailAt != nil)
			)
			return Set(stamped)
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
				if let existing = try File
					.filter(File.Columns.folderId == folderId)
					.filter(File.Columns.nameKey == file.nameKey)
					.fetchOne(database) {
					outcome.skipped += 1
					// Settled unless the drain will still visit it — the
					// exact worklist predicate (thumbnailPending is the
					// master): this import's row, unstamped, present,
					// thumbnailable kind, no residue. An overlap import's
					// row (different import_id) is settled here whatever
					// its state — it's not this run's work.
					let pendingResidue = try Bool.fetchOne(database, sql: """
						SELECT EXISTS(SELECT 1 FROM file_errors
							WHERE file_id = ? AND task = 'thumbnail')
						""", arguments: [existing.id]) ?? false
					let drainWillVisit = existing.importId == importId
						&& existing.thumbnailAt == nil
						&& !existing.missing
						&& FileFormat.thumbnailingKinds.contains(existing.kind)
						&& !pendingResidue
					if !drainWillVisit { outcome.settled += 1 }
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
					// The worklist stamp records that this roster LOOKED, not
					// that it found anything — an evidence-less read stamps the
					// version (the blob's NULL carries the yield), so a roster
					// bump's worklist doesn't re-read bare files forever.
					// 0 = never attempted (no extractor wired, or it failed).
					metadataVersion: file.discovered.format.metadataExtractor != nil && !file.extractionFailed
						? FileMetadata.currentVersion : 0,
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
				// A kind the registry never thumbnails (sidecar, audio,
				// unrecognized…) never enters the worklist: ready on
				// arrival, or the ring could never fill on a RAW+XMP shoot.
				if !FileFormat.thumbnailingKinds.contains(record.kind) {
					outcome.settled += 1
				}
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

	/// The file's current absolute URL on disk, or nil when its volume isn't
	/// mounted. "Just put the paths together": the folder's reconstructed
	/// volume-relative path, under the volume's live mount point, plus the file
	/// name. Nothing here is stored — paths recompose through the folder tree
	/// (so a relocation is a one-row edit) and the mount is resolved live. The
	/// path is rebuilt by walking the folder's ancestors to its tracked root —
	/// the ancestor twin of the subtree walks — where the root carries the
	/// volume-relative root_path and each descendant contributes its on-disk
	/// name.
	///
	/// Synchronous over a passed `db` and `nonisolated static` so an observation
	/// can compose the URL from inside its own `fetch(_:)` (RepresentativeFile-
	/// LocationRequest) without a second reader hop.
	nonisolated static func fileURL(_ db: Database, of fileID: Identifier<File>) throws -> URL? {
		guard let file = try File.fetchOne(db, key: fileID) else { return nil }
		// The folder's ancestor chain, root first (highest depth). The extra
		// `depth` column orders the walk and is ignored by Folder decoding.
		let ancestors = try Folder.fetchAll(db, sql: """
			WITH RECURSIVE ancestry AS (
			    SELECT folders.*, 0 AS depth FROM folders WHERE id = ?
			    UNION ALL
			    SELECT folders.*, ancestry.depth + 1 FROM folders
			    JOIN ancestry ON folders.id = ancestry.parent_id
			)
			SELECT * FROM ancestry ORDER BY depth DESC
			""", arguments: [file.folderId])
		// The root anchors the volume-relative path; descendants add names.
		guard let root = ancestors.first, let rootPath = root.rootPath else { return nil }
		let relativePath = ([rootPath] + ancestors.dropFirst().map(\.name)).joined(separator: "/")
		guard let volume = try Volume.fetchOne(db, key: root.volumeId),
		      let identity = volume.identity,
		      let mount = currentMountURL(of: identity) else { return nil }
		return relativePath
			.split(separator: "/")
			.reduce(mount) { $0.appending(path: String($1)) }
			.appending(path: file.name)
	}
}
