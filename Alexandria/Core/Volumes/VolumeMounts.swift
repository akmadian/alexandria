//
//  VolumeMounts.swift
//  Alexandria
//

import Foundation

/// The volume's current mount point, or nil when it isn't mounted — the one
/// live filesystem lookup behind file-URL resolution. Matches the stored
/// identity against the currently mounted volumes by filesystem UUID, the same
/// key ObservedVolume recorded the identity from, so the two ends agree.
// TODO: only the filesystemUUID rung resolves — network identities (smb/nfs)
// need their own remount lookup when those rungs land (VolumeIdentity). And
// this resolves once per call: no cached mount map, no mount/unmount
// observation, so availability can't update live. Promote to a cached,
// event-driven resolver when the UI needs live availability flips (the twin
// TODO on Catalog.currentURL).
nonisolated func currentMountURL(of identity: VolumeIdentity) -> URL? {
	guard case .filesystemUUID(let uuid) = identity else { return nil }
	guard let mounted = FileManager.default.mountedVolumeURLs(
		includingResourceValuesForKeys: [.volumeUUIDStringKey], options: []
	) else { return nil }
	for volumeURL in mounted {
		let values = try? volumeURL.resourceValues(forKeys: [.volumeUUIDStringKey])
		if values?.volumeUUIDString == uuid { return volumeURL }
	}
	return nil
}
