//
//  ImportRun.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation
import Algorithms
import CryptoKit
import Logging
internal import UniformTypeIdentifiers

@MainActor @Observable
final class ImportRun {
	/// Carries the import id as metadata so every line of this run —
	/// formation's included — is correlatable.
	private let log: Logger

	let id: Identifier<Import>  // = the imports row id: one identity, carried everywhere
	private let folderUrl: URL
	private let catalog: Catalog
	private(set) var totalFiles = 0
	private(set) var completedFiles = 0
	private(set) var skippedFiles = 0
	private var task: Task<Void, Error>?

	init(folderUrl: URL, catalog: Catalog) {
		let id = Identifier<Import>.mint()
		var log = Logger(label: "import")
		log[metadataKey: "importId"] = "\(id.rawValue)"
		self.id = id
		self.log = log
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

			// Bracket discipline: every ending past this point stamps an
			// outcome. Only a dead process leaves the row open (NULL =
			// interrupted, the schema's signal).
			do {
				let (discovered, failures) = try await self.walk(dirSource: self.folderUrl)
				self.totalFiles = discovered.count
				self.log.info("Walk complete", metadata: [
					"folder": "\(self.folderUrl.lastPathComponent)",
					"files": "\(self.totalFiles)",
					"failures": "\(failures.count)",
				])

				try await self.catalog.recordImportErrors(
					failures.map { (observed.relativePath(of: $0.url), $0.reasonCode, $0.message) },
					importId: self.id
				)

				var assetsMinted = 0
				var assetsAbsorbed = 0
				for batch in discovered.chunks(ofCount: 200) {
					try Task.checkCancellation()
					let prepared = await self.prepareFiles(batch: Array(batch))
					let outcome = try await self.catalog.recordNewFileBatch(
						prepared, importId: self.id,
						rootFolderId: rootFolderId, rootUrl: self.folderUrl
					)
					self.completedFiles += outcome.recorded
					self.skippedFiles += outcome.skipped

					// Per-batch formation pass: assets trickle in behind the
					// batches. A mid-import failure is retried by the next
					// pass — the worklist (asset_id IS NULL) is intact. A
					// cancellation is not a pass failure; rethrow it.
					do {
						let resolution = try await self.formAssets()
						assetsMinted += resolution.assetsMinted
						assetsAbsorbed += resolution.assetsAbsorbed
					} catch is CancellationError {
						throw CancellationError()
					} catch {
						self.log.warning("Formation pass failed; next pass retries", metadata: [
							"error": "\(error)"
						])
					}
				}

				// The final completeness sweep is the gate: formation is
				// import-critical, so its failure fails the import.
				let resolution: AssetFormation.Resolution
				do {
					resolution = try await self.formAssets()
				} catch is CancellationError {
					throw CancellationError()
				} catch {
					self.log.error("Final formation sweep failed; the import fails with it", metadata: [
						"error": "\(error)"
					])
					throw error
				}
				assetsMinted += resolution.assetsMinted
				assetsAbsorbed += resolution.assetsAbsorbed
				if resolution.sidecarsPending > 0 {
					self.log.info("Orphan sidecars left formation-pending", metadata: [
						"count": "\(resolution.sidecarsPending)"
					])
				}
				try await self.catalog.recordImportFinished(id: self.id, outcome: .completed)
				self.log.info("Import completed", metadata: [
					"files": "\(self.totalFiles)",
					"recorded": "\(self.completedFiles)",
					"skipped": "\(self.skippedFiles)",
					"walkFailures": "\(failures.count)",
					"assets": "\(assetsMinted - assetsAbsorbed)",
					"sidecarsPending": "\(resolution.sidecarsPending)",
				])
			} catch is CancellationError {
				try await self.catalog.recordImportFinished(id: self.id, outcome: .canceled)
			} catch {
				self.log.error("Import failed", metadata: ["error": "\(error)"])
				try? await self.catalog.recordImportFinished(id: self.id, outcome: .failed)
				throw error
			}
		}
	}

	@concurrent
	private func probeVolume(containing url: URL) async throws -> ObservedVolume {
		try ObservedVolume(containing: url)
	}

	/// One asset-formation pass over this import's committed files — the
	/// distinct pipeline step, peer to walk and prepare: read the import's
	/// files, let the engine decide, have the catalog persist the clusters.
	/// Idempotent, so safe per batch and as the final sweep. Internal so a
	/// test can exercise this exact composition.
	@concurrent
	func formAssets() async throws -> AssetFormation.Resolution {
		let files = try await self.catalog.files(inImport: self.id)
		let resolution = AssetFormation.form(files: files, log: self.log)
		let skipped = try await self.catalog.recordFormedAssets(resolution.clusters)
		if skipped > 0 {
			self.log.warning("Formation admissions skipped: rows no longer unformed at write", metadata: [
				"count": "\(skipped)"
			])
		}
		return resolution
	}
	
	@concurrent
	func prepareFiles(batch: [DiscoveredFile]) async -> [PreparedFile] {
		var prepared: [PreparedFile] = []
		
		for discoveredFile in batch {
			let name: String = discoveredFile.url.lastPathComponent
			let nameKey: String = name.precomposedStringWithCanonicalMapping
			let (fileStem, fileExtension) = Self.splitStem(nameKey)

			let hash = self.sha256First64KB(from: discoveredFile.url)
			if hash == nil {
				// Per-file and potentially numerous (a permission-denied
				// subtree); the file still indexes with a NULL hash.
				self.log.debug("Hash not available", metadata: ["url": "\(discoveredFile.url)"])
			}
			
			var metadata: FileMetadata? = nil
			var extractionFailed = false
			
			if let extractor = discoveredFile.format.metadataExtractor {
				do {
					let extracted = try await extractor.extract(from: discoveredFile.url)
					metadata = extracted.isEmpty ? nil : extracted
				} catch {
					extractionFailed = true
					self.log.warning("Metadata extraction failed", metadata: [
						"url": "\(discoveredFile.url)",
						"error": "\(error)"
					])
				}
			}
			
			prepared.append(
				PreparedFile(
					discovered: discoveredFile,
					name: name,
					nameKey: nameKey,
					fileStem: fileStem,
					fileExtension: fileExtension,
					contentHash: hash,
					metadata: metadata,
					extractionFailed: extractionFailed
				)
			)
		}

		return prepared
	}

	/// The ratified stem/extension derivation (schema: file_stem,
	/// file_extension): lowercase+NFC, final-dot rule — "photo.raw.xmp"
	/// yields ("photo.raw", "xmp"). A leading dot is part of the stem, and
	/// no dot means extension ''.
	nonisolated static func splitStem(_ nameKey: String) -> (stem: String, ext: String) {
		let lowered = nameKey.lowercased().precomposedStringWithCanonicalMapping
		guard let dot = lowered.lastIndex(of: "."), dot != lowered.startIndex else {
			return (lowered, "")
		}
		return (String(lowered[..<dot]), String(lowered[lowered.index(after: dot)...]))
	}
	
	@concurrent
	func walk(dirSource: URL) async throws -> ([DiscoveredFile], [WalkFailure]) {
		var discovered: [DiscoveredFile] = []
		var failures: [WalkFailure] = []

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
				self.log.warning("Failed to enumerate dir", metadata: [
					"url": "\(url)",
					"error": "\(error)"
				])
				failures.append(WalkFailure(
					url: url, reasonCode: "read_failed", message: "\(error)"
				))
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
				failures.append(WalkFailure(
					url: fileUrl, reasonCode: "read_failed", message: "\(error)"
				))
				continue
			}

			if resourceValues.isDirectory == true {
				continue
			}

			guard let size = resourceValues.fileSize,
				  let mtime = resourceValues.contentModificationDate else {
				self.log.warning("Missing size or mtime for file", metadata: ["url": "\(fileUrl)"])
				failures.append(WalkFailure(
					url: fileUrl, reasonCode: "attributes_missing",
					message: "no size or modification date"
				))
				continue
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

		return (discovered, failures)
	}

	func cancel() { task?.cancel() }
	
	nonisolated func sha256First64KB(from url: URL) -> String? {
		let handle: FileHandle
		
		do{ try handle = FileHandle(forReadingFrom: url) }
		catch { return nil }
		defer { try? handle.close() }
		
		// Read exactly 64KB
		let hashData = handle.readData(ofLength: 65536)
		
		var hasher = SHA256()
		hasher.update(data: hashData)
		let digest = hasher.finalize()
		
		// Convert to hex string
		return digest.map { String(format: "%02hhx", $0) }.joined()
	}
}
