//
//  ObservedVolume.swift
//  Alexandria
//

import Foundation

/// A snapshot of disk truth about one volume: everything root minting needs,
/// captured in one probe before any transaction opens. Inert by design —
/// no method re-touches the disk after construction.
nonisolated struct ObservedVolume: Sendable {
	let identity: VolumeIdentity?
	let name: String
	let kind: VolumeKind
	let volumeRootURL: URL

	/// Volume-relative path of `url` — the `root_path` form. Pure derivation
	/// from the snapshot.
	// Firmlink dragon: on the boot volume macOS splits system and data
	// (/System/Volumes/Data), so literal and canonical paths can disagree.
	// Revisit when internal-volume imports get real testing.
	func relativePath(of url: URL) -> String {
		let rootPath = volumeRootURL.standardizedFileURL.path(percentEncoded: false)
		let fullPath = url.standardizedFileURL.path(percentEncoded: false)
		guard fullPath.hasPrefix(rootPath) else { return fullPath }
		var relative = String(fullPath.dropFirst(rootPath.count))
		if relative.hasPrefix("/") { relative.removeFirst() }
		return relative
	}
}

extension ObservedVolume {
	/// One probe, one snapshot: constructing the observation is observing.
	init(containing url: URL) throws {
		let values = try url.resourceValues(forKeys: [
			.volumeUUIDStringKey, .volumeNameKey, .volumeIsLocalKey,
			.volumeIsInternalKey, .volumeURLKey,
		])
		guard let volumeRootURL = values.volume else {
			throw VolumeObservationError.noContainingVolume(url)
		}
		self.volumeRootURL = volumeRootURL
		identity = VolumeIdentity(uuid: values.volumeUUIDString)
		name = values.volumeName ?? volumeRootURL.lastPathComponent
		let isLocal = values.volumeIsLocal ?? true
		let isInternal = values.volumeIsInternal ?? false
		kind = !isLocal ? .network : (isInternal ? .local : .external)
	}
}

nonisolated enum VolumeObservationError: Error {
	case noContainingVolume(URL)
}
