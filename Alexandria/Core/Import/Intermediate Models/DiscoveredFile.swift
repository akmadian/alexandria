//
//  DiscoveredFile.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation

struct DiscoveredFile: Sendable {
	let url: URL
	let size: Int
	let modifiedAt: Date
	let format: FileFormat
}
