//
//  VolumeIdentity.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//
// 	Who a volume is to the filesystem
//

import Foundation

enum VolumeIdentity {
	// TODO: Add network volume identity resolution
	static func resolve(uuid: String?, isLocal: Bool, remountUrl: URL?) -> String? {
		return uuid ?? nil
	}
}
