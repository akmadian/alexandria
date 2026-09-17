//
//  MountTable.swift
//  Alexandria
//

import Foundation
import Logging
import Synchronization

private nonisolated let log = Logger(label: "MountTable")

/// The live mirror of the mount table: every mounted volume's identity → its
/// volume root URL. Nonisolated, lock-backed storage because the resolution
/// seam (Catalog.fileURL) reads it synchronously from inside GRDB reads on
/// reader threads — a main-actor map can't serve that seam. Freshness is
/// event-driven: VolumeMonitor calls rebuild() on every mount, unmount, and
/// rename; the first read self-seeds, so no reader ever observes an unseeded
/// table (and tests and previews need no monitor).
nonisolated final class MountTable: Sendable {
	static let shared = MountTable()

	private struct State {
		/// nil = never seeded, distinct from "seeded and nothing identified".
		var mounts: [VolumeIdentity: URL]?
		/// Rebuild ordering (see rebuild()).
		var epoch = 0
	}
	private let state = Mutex(State())

	/// The enumeration seam: production mirrors the OS mount table; tests
	/// substitute a scripted enumeration to drive seed and rebuild races
	/// deterministically (round review: the type carrying the concurrency
	/// risk must be testable). The closure must never touch the table — the
	/// seed path runs it under the lock.
	private let enumerate: @Sendable () -> [VolumeIdentity: URL]

	init(enumerate: @escaping @Sendable () -> [VolumeIdentity: URL] = MountTable.observeMounts) {
		self.enumerate = enumerate
	}

	/// The volume's current mount point, or nil when it isn't mounted. The
	/// first-ever read seeds UNDER the lock, so exactly one thread enumerates
	/// and concurrent first readers wait for its answer — the stall ceiling
	/// of the pre-table code (where every read walked the mount table), paid
	/// at most once. Every later read is a pure lock-read; the enumeration a
	/// dead network mount can stall never runs on a steady-state reader.
	func mountURL(of identity: VolumeIdentity) -> URL? {
		state.withLock { state in
			if state.mounts == nil {
				state.epoch += 1
				state.mounts = enumerate()
			}
			return state.mounts?[identity]
		}
	}

	/// Re-mirrors the OS mount table wholesale. Enumeration runs outside the
	/// lock — probing a dead network mount can stall, and steady-state
	/// readers must not stall with it — and the epoch guard drops a swap
	/// that lost the race to a later-STARTED rebuild. Start order, not
	/// enumeration order: a later-started rebuild reflects the event that
	/// triggered it, so its map is the one to keep.
	func rebuild() {
		let epoch = state.withLock { state in
			state.epoch += 1
			return state.epoch
		}
		let mounts = enumerate()
		state.withLock { state in
			guard state.epoch == epoch else {
				// A silently dropped rebuild is the first suspect in a
				// staleness report; say it happened.
				log.debug("rebuild dropped, superseded", metadata: ["epoch": "\(epoch)"])
				return
			}
			state.mounts = mounts
			log.debug("mount table rebuilt", metadata: [
				"count": "\(mounts.count)",
				"identities": "\(mounts.keys.map(\.rawValue).sorted())",
			])
		}
	}

	/// One probe of the real mount table. First-wins on a duplicate identity
	/// (two mounts of one share, a cloned UUID): mountedVolumeURLs order is
	/// unspecified, so last-wins would let equal catalogs resolve different
	/// paths across launches — first-wins plus the warning makes the pick
	/// explicit and visible.
	static func observeMounts() -> [VolumeIdentity: URL] {
		guard let mounted = FileManager.default.mountedVolumeURLs(
			includingResourceValuesForKeys: nil, options: []
		) else { return [:] }
		var mounts: [VolumeIdentity: URL] = [:]
		for volumeURL in mounted {
			// One concept, one implementation: identity comes from the same
			// observation the import path records (ObservedVolume), so both
			// ends of the match apply the identical ladder.
			guard let observed = try? ObservedVolume(containing: volumeURL) else {
				log.warning("mounted volume could not be observed", metadata: [
					"url": "\(volumeURL.path)",
				])
				continue
			}
			guard let identity = observed.identity else {
				// The exact case behind a "why does this volume show
				// offline" report: mounted, but unmatchable.
				log.warning("mounted volume has no identity", metadata: [
					"url": "\(volumeURL.path)",
				])
				continue
			}
			if let standing = mounts[identity] {
				log.warning("duplicate volume identity; keeping the first mount", metadata: [
					"identity": "\(identity.rawValue)",
					"kept": "\(standing.path)",
					"dropped": "\(observed.volumeRootURL.path)",
				])
				continue
			}
			mounts[identity] = observed.volumeRootURL
		}
		return mounts
	}
}

/// The volume's current mount point, or nil when it isn't mounted — the one
/// live filesystem lookup behind file-URL resolution. A one-line forward on
/// purpose: it preserves the resolution seam's vocabulary, so Catalog.fileURL
/// reads the same as before the table existed.
// PERF: every resolution takes the table's lock; if a bulk pass (thumbnail
// regeneration over thousands of files) ever shows contention here, hand the
// pass one immutable snapshot of the map instead.
nonisolated func currentMountURL(of identity: VolumeIdentity) -> URL? {
	MountTable.shared.mountURL(of: identity)
}
