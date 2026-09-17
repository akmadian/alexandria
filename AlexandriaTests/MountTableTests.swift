//
//  MountTableTests.swift
//  AlexandriaTests
//
//  The table carries all the round's concurrency risk, so it gets direct
//  tests through the enumeration seam — scripted maps, no real filesystem.
//

import Foundation
import Synchronization
import Testing
@testable import Alexandria

nonisolated struct MountTableTests {

	private static let scratch = VolumeIdentity.filesystemUUID("SCRATCH")
	private static let scratchURL = URL(fileURLWithPath: "/Volumes/Scratch")

	/// The self-seed: the first read enumerates, under the lock, exactly
	/// once — concurrent later reads are pure lock-reads.
	@Test func firstReadSeedsExactlyOnce() throws {
		let calls = Mutex(0)
		let table = MountTable {
			calls.withLock { $0 += 1 }
			return [Self.scratch: Self.scratchURL]
		}
		#expect(table.mountURL(of: Self.scratch) == Self.scratchURL)
		#expect(table.mountURL(of: .filesystemUUID("OTHER")) == nil)
		#expect(calls.withLock { $0 } == 1)
	}

	/// Rebuilds replace wholesale: an identity absent from the newer
	/// enumeration is gone, not lingering from a patch.
	@Test func rebuildReplacesTheWholeMap() throws {
		let maps = Mutex<[[VolumeIdentity: URL]]>([
			[Self.scratch: Self.scratchURL],
			[.filesystemUUID("NEXT"): URL(fileURLWithPath: "/Volumes/Next")],
		])
		let table = MountTable {
			maps.withLock { $0.isEmpty ? [:] : $0.removeFirst() }
		}
		table.rebuild()
		#expect(table.mountURL(of: Self.scratch) == Self.scratchURL)
		table.rebuild()
		#expect(table.mountURL(of: Self.scratch) == nil)
		#expect(table.mountURL(of: .filesystemUUID("NEXT")) != nil)
	}

	/// The epoch guard: a rebuild that STARTED earlier but finished later
	/// never overwrites the newer one's map. Orchestrated with semaphores so
	/// the race is deterministic: the first rebuild blocks inside its
	/// enumeration while the second starts and finishes.
	@Test func supersededRebuildNeverOverwritesANewerOne() throws {
		let firstEntered = DispatchSemaphore(value: 0)
		let firstMayFinish = DispatchSemaphore(value: 0)
		let calls = Mutex(0)
		let stale: [VolumeIdentity: URL] = [.filesystemUUID("STALE"): URL(fileURLWithPath: "/stale")]
		let fresh: [VolumeIdentity: URL] = [.filesystemUUID("FRESH"): URL(fileURLWithPath: "/fresh")]
		let table = MountTable {
			let call = calls.withLock { count in
				count += 1
				return count
			}
			guard call == 1 else { return fresh }
			firstEntered.signal()
			firstMayFinish.wait()
			return stale
		}

		let firstDone = DispatchSemaphore(value: 0)
		DispatchQueue.global().async {
			table.rebuild()
			firstDone.signal()
		}
		firstEntered.wait()
		table.rebuild() // Started later; finishes first.
		firstMayFinish.signal()
		firstDone.wait()

		#expect(table.mountURL(of: .filesystemUUID("FRESH")) != nil)
		#expect(table.mountURL(of: .filesystemUUID("STALE")) == nil)
	}

	/// One integration touch of the real enumeration: the boot volume —
	/// always mounted — resolves through the production path.
	@Test func realEnumerationResolvesTheBootVolume() throws {
		let bootUUID = try #require(
			try URL(fileURLWithPath: "/")
				.resourceValues(forKeys: [.volumeUUIDStringKey]).volumeUUIDString
		)
		let mounts = MountTable.observeMounts()
		#expect(mounts[.filesystemUUID(bootUUID)] != nil)
	}
}
