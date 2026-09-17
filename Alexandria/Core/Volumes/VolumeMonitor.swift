//
//  VolumeMonitor.swift
//  Alexandria
//
//  Watches the OS for volumes coming and going and keeps MountTable current.
//  Catalog-blind by design: it mirrors the whole mount table, and "is this a
//  volume we track" is always the consumer's lookup — the monitor could be
//  lifted into any app unchanged.
//

import AppKit
import Logging
import Observation

// TODO: name refresh (ratified 2026-09-17, deferred): when a tracked volume
// mounts or renames with a label differing from volumes.name, update the
// record. A small catalog-aware listener beside this monitor, never inside
// it — the monitor stays catalog-blind.

/// The observable face of MountTable. `generation` bumps after every rebuild
/// lands, so a SwiftUI reader going through `mountURL(of:)` (the browser's
/// volume header) re-renders on mount flips with no subscription code.
/// Non-UI resolution stays on the free `currentMountURL(of:)`.
@MainActor @Observable
final class VolumeMonitor {
	/// Bumped after every rebuild the events (or the seed) trigger.
	private(set) var generation = 0

	/// nonisolated(unsafe) is sound because of REACHABILITY, not timing: no
	/// code path other than init and deinit touches this array — the
	/// observer blocks call volumesDidChange, which never reads it. (Self
	/// does escape during init, into those blocks; that isn't the argument.)
	/// Adding any other accessor invalidates this and needs real isolation.
	@ObservationIgnored private nonisolated(unsafe) var observers: [NSObjectProtocol] = []
	@ObservationIgnored private let table: MountTable
	@ObservationIgnored private let log = Logger(label: "VolumeMonitor")

	init(table: MountTable = .shared) {
		self.table = table
		// Subscribe before seeding, so a mount racing construction is caught
		// by the seed or by its event — the rebuild is idempotent either way.
		let center = NSWorkspace.shared.notificationCenter
		for name in [
			NSWorkspace.didMountNotification,
			NSWorkspace.didUnmountNotification,
			NSWorkspace.didRenameVolumeNotification,
		] {
			observers.append(center.addObserver(
				forName: name, object: nil, queue: .main
			) { [weak self] notification in
				// The name alone is read out for legibility; the handler
				// re-reads the mount table rather than trusting any event
				// payload. queue: .main runs the block on the main thread,
				// so the assumeIsolated assertion always holds.
				let name = notification.name
				MainActor.assumeIsolated {
					self?.volumesDidChange(name)
				}
			})
		}
		volumesDidChange(nil)
		log.debug("observers registered, seed dispatched")
	}

	deinit {
		for observer in observers {
			NSWorkspace.shared.notificationCenter.removeObserver(observer)
		}
	}

	/// The UI read: the volume's current mount point, or nil when it isn't
	/// mounted. Touching `generation` registers the observation dependency,
	/// so a view that read a volume's status re-renders when the table flips.
	/// A nil identity is the consumer's short-circuit ("unknown", never
	/// "unmounted") before the table is asked.
	func mountURL(of identity: VolumeIdentity) -> URL? {
		_ = generation
		return table.mountURL(of: identity)
	}

	/// One handler for every event and the seed: re-mirror the table
	/// wholesale. Rebuilding instead of patching per event deletes the
	/// per-event edge cases (unmount matching by stale URL, rename
	/// bookkeeping) — the mount table is a handful of entries and events are
	/// human-rate. Off the main actor because probing a dead network mount
	/// can stall.
	private func volumesDidChange(_ event: Notification.Name?) {
		if let event {
			log.debug("mount table changed", metadata: ["event": "\(event.rawValue)"])
		}
		let table = table
		Task.detached(priority: .utility) { [weak self] in
			table.rebuild()
			await self?.rebuildLanded()
		}
	}

	private func rebuildLanded() {
		generation += 1
		log.trace("rebuild landed", metadata: ["generation": "\(generation)"])
	}
}
