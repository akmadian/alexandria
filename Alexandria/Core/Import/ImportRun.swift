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
	private let rootFolderId: Identifier<Folder>
	private let volume: ObservedVolume
	private let catalog: Catalog
	/// True when this run picked an unfinished import back up.
	private let resuming: Bool
	private(set) var totalFiles = 0
	private(set) var completedFiles = 0
	private(set) var skippedFiles = 0
	private(set) var thumbnailedFiles = 0
	private(set) var thumbnailFailures = 0
	private var task: Task<Void, Error>?

	/// Batch = width: the worklist pull size IS the parallelism — every task
	/// in a drained batch runs concurrently, batches run serially. One
	/// constant, one place; volume-aware policy is the named refinement.
	private nonisolated static let thumbnailBatchWidth = 8
	/// Per-file deadline (thumbnails.md invariant 2): abandon, not cancel —
	/// without it one wedged decode is an import that never completes.
	private nonisolated static let thumbnailDeadline: Duration = .seconds(30)

	/// Required, not optional (ruled 2026-09-11): the grid IS the product and
	/// thumbnails are its face — a run that could silently skip them would
	/// ship a wall of shimmer behind one log line. ImportService provides it
	/// or refuses to start the import.
	private let thumbnailStore: ThumbnailStore

	/// Identity and residence are the SERVICE's job (resume ruling,
	/// 2026-09-11): a run is identified by its source, so ImportService
	/// resolves source → root folder → unfinished-job-or-mint before this
	/// init — which is what lets id and the correlated logger stay immutable.
	init(
		id: Identifier<Import>,
		folderUrl: URL,
		rootFolderId: Identifier<Folder>,
		volume: ObservedVolume,
		catalog: Catalog,
		store: ThumbnailStore,
		resuming: Bool
	) {
		var log = Logger(label: "import")
		log[metadataKey: "importId"] = "\(id.rawValue)"
		self.id = id
		self.log = log
		self.folderUrl = folderUrl
		self.rootFolderId = rootFolderId
		self.volume = volume
		self.catalog = catalog
		self.resuming = resuming
		self.thumbnailStore = store
	}

	func start() {
		self.log.info(resuming ? "Resuming import run" : "Starting import run")
		task = Task(name: "import-\(id.rawValue)") {
			// The run's bracket (residence already resolved by the service).
			// A resumed bracket reopens: a canceled/failed row must read as
			// running again until this run stamps its own ending.
			if self.resuming {
				try await self.catalog.recordImportResumed(id: self.id)
			} else {
				try await self.catalog.recordImportStarted(id: self.id, folderId: self.rootFolderId)
			}

			// The thumbnail worker: a structured child beside the record loop.
			// The stream carries wake-ups, never work — the worklist query
			// (thumbnail_at IS NULL) is the single source of truth. Conflated
			// buffering makes nudges level-triggered: "something changed, go
			// look". The defer guards the error paths — an unfinished stream
			// is a worker awaiting nudges forever.
			let (nudges, nudge) = AsyncStream.makeStream(
				of: Void.self, bufferingPolicy: .bufferingNewest(1)
			)
			async let thumbnailing: Void = self.generateThumbnails(nudges: nudges)
			defer { nudge.finish() }

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
					failures.map { (self.volume.relativePath(of: $0.url), $0.reasonCode, $0.message) },
					importId: self.id
				)

				var assetsMinted = 0
				var assetsAbsorbed = 0
				for batch in discovered.chunks(ofCount: 200) {
					try Task.checkCancellation()
					let prepared = await self.prepareFiles(batch: Array(batch))
					let outcome = try await self.catalog.recordNewFileBatch(
						prepared, importId: self.id,
						rootFolderId: self.rootFolderId, rootUrl: self.folderUrl
					)
					self.completedFiles += outcome.recorded
					self.skippedFiles += outcome.skipped
					nudge.yield(())  // recorded rows ARE the thumbnail jobs; wake the worker

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

				// Import-critical, non-blocking: thumbnailing never gated the
				// batch loop, but the bracket waits for the drain — a
				// thumbnail is part of ready-to-use. Per-file failures are
				// residue; the WORKER dying fails the import (via this throw).
				nudge.finish()
				do {
					try await thumbnailing
				} catch is CancellationError {
					throw CancellationError()
				} catch {
					self.log.error("Thumbnail worker failed; the import fails with it", metadata: [
						"error": "\(error)"
					])
					throw error
				}

				try await self.catalog.recordImportFinished(id: self.id, outcome: .completed)
				self.log.info("Import completed", metadata: [
					"files": "\(self.totalFiles)",
					"recorded": "\(self.completedFiles)",
					"skipped": "\(self.skippedFiles)",
					"walkFailures": "\(failures.count)",
					"assets": "\(assetsMinted - assetsAbsorbed)",
					"sidecarsPending": "\(resolution.sidecarsPending)",
					"thumbnailed": "\(self.thumbnailedFiles)",
					"thumbnailFailures": "\(self.thumbnailFailures)",
				])
			} catch is CancellationError {
				self.log.info("Import canceled", metadata: [
					"recorded": "\(self.completedFiles)",
					"thumbnailed": "\(self.thumbnailedFiles)",
				])
				await self.recordEndingUncancelled(outcome: .canceled)
			} catch {
				self.log.error("Import failed", metadata: ["error": "\(error)"])
				try? await self.catalog.recordImportFinished(id: self.id, outcome: .failed)
				throw error
			}
		}
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
	
	/// The thumbnail worker (thumbnails.md) — formAssets's peer for the other
	/// derived artifact. Wakes on each nudge and generates batches until the
	/// worklist is empty; the run after the stream ends is the final sweep,
	/// catching whatever the last nudge's run raced past. Idempotent like
	/// formation: the database is the worklist, so re-running is always safe
	/// and crash recovery is "run it again". Internal so a test can exercise
	/// this exact composition.
	@concurrent
	func generateThumbnails(nudges: AsyncStream<Void>) async throws {
		for await _ in nudges {
			while try await self.generateThumbnailBatch(store: self.thumbnailStore) {}
		}
		while try await self.generateThumbnailBatch(store: self.thumbnailStore) {}
	}

	/// The ending stamp for a CANCELED run must land from inside the
	/// just-cancelled task, and GRDB checks cancellation on every async
	/// access — so the write hops to a fresh, uncancelled Task: the one
	/// legitimate unstructured Task in the pipeline (cleanup after
	/// cancellation is precisely what structured children cannot do).
	private func recordEndingUncancelled(outcome: ImportOutcome) async {
		let catalog = self.catalog
		let id = self.id
		let result = await Task { try await catalog.recordImportFinished(id: id, outcome: outcome) }.result
		if case .failure(let error) = result {
			self.log.error("Failed to record import ending", metadata: [
				"outcome": "\(outcome.rawValue)",
				"error": "\(error)",
			])
		}
	}

	/// One worklist batch: pull, generate every member concurrently (batch =
	/// width), stamp all in one transaction — the UI's shimmer wave. Returns
	/// whether anything was pending, so the caller loops until false.
	/// Cancellation aborts the batch unrecorded — rows stay pending, and the
	/// resumed run completes them (resume ruling, 2026-09-11); per-file
	/// failures become file_errors residue, one attempt per file per import.
	@concurrent
	private func generateThumbnailBatch(store: ThumbnailStore) async throws -> Bool {
		try Task.checkCancellation()
		let pending = try await self.catalog.thumbnailPending(
			inImport: self.id, under: self.rootFolderId, limit: Self.thumbnailBatchWidth
		)
		if pending.isEmpty { return false }

		var generated: [Identifier<File>] = []
		var failures: [(fileId: Identifier<File>, reasonCode: String, message: String)] = []
		try await withThrowingTaskGroup(of: ThumbnailOutcome.self) { group in
			for item in pending {
				let fileId = item.file.id
				let url = item.url(under: self.folderUrl)
				let thumbnailer = FileFormat.resolve(
					extension: item.file.fileExtension, recordedKind: item.file.kind
				).thumbnailer
				group.addTask {
					guard let thumbnailer else {
						// The kind filter admitted it but the resolved row
						// has no capability — residue, or the row would
						// re-query forever.
						return .failed(fileId, reasonCode: "no_thumbnailer",
						               message: "resolved format has no thumbnailer")
					}
					do {
						try await withThumbnailDeadline(Self.thumbnailDeadline) {
							let image = try await thumbnailer(url, ThumbnailStore.maxPixelSize)
							try await store.write(image, for: fileId)
						}
						return .generated(fileId)
					} catch is CancellationError {
						throw CancellationError()
					} catch ThumbnailError.timedOut {
						return .failed(fileId, reasonCode: "timed_out",
						               message: "no thumbnail within \(Self.thumbnailDeadline)")
					} catch {
						return .failed(fileId, reasonCode: "decode_failed", message: "\(error)")
					}
				}
			}
			for try await outcome in group {
				switch outcome {
				case .generated(let fileId):
					generated.append(fileId)
				case .failed(let fileId, let reasonCode, let message):
					failures.append((fileId, reasonCode, message))
				}
			}
		}

		try await self.catalog.recordThumbnails(generated: generated, failures: failures)
		await self.noteThumbnailProgress(generated: generated.count, failed: failures.count)
		self.log.debug("Thumbnail batch recorded", metadata: [
			"generated": "\(generated.count)",
			"failed": "\(failures.count)",
		])
		// One warning per failure, named: "which file ate 30 seconds" must be
		// answerable from the log, not only from file_errors via SQL.
		let namesById = Dictionary(uniqueKeysWithValues: pending.map { ($0.file.id, $0.file.name) })
		for failure in failures {
			self.log.warning("Thumbnail failed; residue recorded", metadata: [
				"file": "\(namesById[failure.fileId] ?? failure.fileId.rawValue.uuidString)",
				"reason": "\(failure.reasonCode)",
				"message": "\(failure.message)",
			])
		}
		return true
	}

	private func noteThumbnailProgress(generated: Int, failed: Int) {
		thumbnailedFiles += generated
		thumbnailFailures += failed
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

	/// One drained file's fate — what the batch stamp transaction records.
	private nonisolated enum ThumbnailOutcome: Sendable {
		case generated(Identifier<File>)
		case failed(Identifier<File>, reasonCode: String, message: String)
	}
	
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
