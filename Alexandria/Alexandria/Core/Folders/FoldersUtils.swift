//
//  FoldersUtils.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation

/// The identity of the volume containing `url`; nil = unidentified.
func resolveParentVolume(url: URL) -> VolumeIdentity? {
	let values = try? url.resourceValues(forKeys: [.volumeUUIDStringKey])
	return VolumeIdentity(uuid: values?.volumeUUIDString)
}
