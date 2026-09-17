//
//  VolumeMonitorTests.swift
//  AlexandriaTests
//
//  Monitor behavior over an injected, scripted table — the real enumeration
//  is MountTableTests' one integration touch.
//

import AppKit
import Foundation
import Synchronization
import Testing
@testable import Alexandria

@MainActor
struct VolumeMonitorTests {

	private nonisolated static let scratch = VolumeIdentity.filesystemUUID("SCRATCH")
	private nonisolated static let scratchURL = URL(fileURLWithPath: "/Volumes/Scratch")

	/// Ruling: construction triggers the seed. Pinned through the seam — the
	/// enumeration runs (and generation announces it) without anything ever
	/// reading the table, so this cannot pass on the table's read-time
	/// self-seed.
	@Test func constructionSeedsWithoutAnyRead() async throws {
		let calls = Mutex(0)
		let monitor = VolumeMonitor(table: MountTable {
			calls.withLock { $0 += 1 }
			return [Self.scratch: Self.scratchURL]
		})
		try await eventually("the seed lands") {
			monitor.generation >= 1 && calls.withLock { $0 } >= 1
		}
		#expect(monitor.mountURL(of: Self.scratch) == Self.scratchURL)
	}

	/// A mount-table event triggers a rebuild and bumps the observable
	/// generation — the mechanism the volume header re-renders through.
	/// Posted by hand: NSWorkspace's center is an ordinary NotificationCenter,
	/// so the test exercises the real subscription without ejecting anything.
	/// NOTE: the post is process-global — every live monitor in the parallel
	/// test run receives it. Rebuilds are idempotent so that's safe today,
	/// but it makes any future "generation never moves without events" test
	/// flaky by construction; don't write that one.
	@Test func mountEventsBumpTheGeneration() async throws {
		let monitor = VolumeMonitor(table: MountTable { [:] })
		try await eventually("the seed lands") { monitor.generation >= 1 }
		let before = monitor.generation
		NSWorkspace.shared.notificationCenter.post(
			name: NSWorkspace.didUnmountNotification, object: NSWorkspace.shared
		)
		try await eventually("the event's rebuild lands") { monitor.generation > before }
	}

	/// The UI read resolves through the injected table, and an identity the
	/// enumeration doesn't carry reads as not mounted.
	@Test func mountURLAnswersFromTheTable() async throws {
		let monitor = VolumeMonitor(table: MountTable { [Self.scratch: Self.scratchURL] })
		try await eventually("the seed lands") { monitor.generation >= 1 }
		#expect(monitor.mountURL(of: Self.scratch) == Self.scratchURL)
		#expect(monitor.mountURL(of: .filesystemUUID("NEVER-A-VOLUME")) == nil)
		#expect(monitor.mountURL(of: .remount("smb://nowhere.invalid/share")) == nil)
	}
}
