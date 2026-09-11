//
//  ImportTask.swift
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
	private let log = Logger(label: "import")
	
	let id = Identifier<Import>.mint()  // = the imports row id: one identity, carried everywhere
	private var folderUrl: URL
	private var catalog: Catalog
	private(set) var totalFiles = 0
	private(set) var completedFiles = 0
	private(set) var skippedFiles = 0
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

			// Bracket discipline: every ending past this point stamps an
			// outcome. Only a dead process leaves the row open (NULL =
			// interrupted, the schema's signal).
			do {
				let (discovered, failures) = try await self.walk(dirSource: self.folderUrl)
				self.totalFiles = discovered.count
				self.log.info("Discovered \(self.totalFiles) files in \(self.folderUrl.lastPathComponent)")

				try await self.catalog.recordImportErrors(
					failures.map { (observed.relativePath(of: $0.url), $0.reasonCode, $0.message) },
					importId: self.id
				)

				for batch in discovered.chunks(ofCount: 200) {
					try Task.checkCancellation()
					let prepared = await self.prepareFiles(batch: Array(batch))
					let outcome = try await self.catalog.recordNewFileBatch(
						prepared, importId: self.id,
						rootFolderId: rootFolderId, rootUrl: self.folderUrl
					)
					self.completedFiles += outcome.recorded
					self.skippedFiles += outcome.skipped
				}
				try await self.catalog.recordImportFinished(id: self.id, outcome: .completed)
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
	
	@concurrent
	func prepareFiles(batch: [DiscoveredFile]) async -> [PreparedFile] {
		var prepared: [PreparedFile] = []
		
		for discoveredFile in batch {
			let name: String = discoveredFile.url.lastPathComponent
			let nameKey: String = name.precomposedStringWithCanonicalMapping
			let (fileStem, fileExtension) = Self.splitStem(nameKey)

			let hash = self.sha256First64KB(from: discoveredFile.url)
			if hash == nil { self.log.warning("Hash not available for \(discoveredFile.url)") }
			
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
				self.log.error("Failed to enumerate dir", metadata: [
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
