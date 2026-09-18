//
//  ImportServiceRegistryTests.swift
//  AlexandriaTests
//
//  The service's session registry (import status round, 2026-09-18): runs
//  with something left to show. Registered before start (so the resume
//  badge never flashes), serialized while executing, lingering when done,
//  removed on dismiss. One end-to-end pass over a real temp catalog +
//  TestData, same pattern as the resume e2e suite.
//

import Foundation
import GRDB
import Testing
@testable import Alexandria

/// repo-root/TestData, resolved from this source file's location.
private nonisolated var testData: URL {
	URL(fileURLWithPath: #filePath)
		.deletingLastPathComponent()   // AlexandriaTests/
		.deletingLastPathComponent()   // repo root
		.appending(path: "TestData")
}

struct ImportServiceRegistryTests {

	@MainActor
	@Test func registryLifecycleAndSerialization() async throws {
		let catalogDirectory = FileManager.default.temporaryDirectory
			.appending(path: "import-registry-\(UUID().uuidString)")
		try FileManager.default.createDirectory(at: catalogDirectory, withIntermediateDirectories: true)
		defer { try? FileManager.default.removeItem(at: catalogDirectory) }
		let catalog = try Catalog.open(at: catalogDirectory)
		let service = ImportService(catalog: catalog)

		// Serialization under reentrancy (review finding, 2026-09-18):
		// launch BOTH starts before awaiting either — startImport suspends
		// four times before its registry write, so this is the interleaving
		// the synchronous in-flight reservation exists to close. Exactly
		// one run may land; exactly one call must be refused.
		func attempt() async -> Result<ImportRun, any Error> {
			do { return .success(try await service.startImport(of: testData)) }
			catch { return .failure(error) }
		}
		async let first = attempt()
		async let second = attempt()
		let results = await [first, second]
		let started = results.compactMap { try? $0.get() }
		#expect(started.count == 1)
		#expect(results.contains { result in
			if case .failure(ImportError.importAlreadyRunning) = result { true } else { false }
		})
		let run = try #require(started.first)

		// Registered under the run's root folder before its bracket write.
		let rootFolderId = try #require(service.runs.first(where: { $0.value === run })?.key)

		// Completion: the run lingers in the registry until dismissed.
		let deadline = Date().addingTimeInterval(60)
		while !run.isFinished && Date() < deadline {
			try await Task.sleep(for: .milliseconds(100))
		}
		#expect(run.phase == .done(.completed))
		#expect(service.runs[rootFolderId] === run)

		// A lingering finished run doesn't block (scenario g): a fresh
		// start REPLACES the entry, never stacks a second one.
		let again = try await service.startImport(of: testData)
		#expect(service.runs.count == 1)
		#expect(service.runs[rootFolderId] === again)
		let againDeadline = Date().addingTimeInterval(60)
		while !again.isFinished && Date() < againDeadline {
			try await Task.sleep(for: .milliseconds(100))
		}

		// Dismiss is removal.
		service.dismiss(folder: rootFolderId)
		#expect(service.runs.isEmpty)
	}
}
