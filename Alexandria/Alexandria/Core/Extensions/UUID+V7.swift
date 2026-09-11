//
//  UUID+V7.swift
//  Alexandria
//
//  Created by ari on 9/9/26.
//

import Foundation

extension UUID {
	/// A UUIDv7: 48-bit big-endian Unix-millisecond timestamp, then version
	/// and variant bits over random tails — time-ordered, so ids minted
	/// together sort and insert together. `instant` is injectable for tests.
	static func v7(at instant: Date = .now) -> Self {
		let timestamp = UInt64(instant.timeIntervalSince1970 * 1000)
		let uuidBytes: (UInt8, UInt8, UInt8, UInt8, UInt8, UInt8, UInt8, UInt8, UInt8, UInt8, UInt8, UInt8, UInt8, UInt8, UInt8, UInt8) = (
			UInt8((timestamp >> 40) & 0xFF),
			UInt8((timestamp >> 32) & 0xFF),
			UInt8((timestamp >> 24) & 0xFF),
			UInt8((timestamp >> 16) & 0xFF),
			UInt8((timestamp >> 8) & 0xFF),
			UInt8(timestamp & 0xFF),
			UInt8.random(in: 0...255) & 0x0F | 0x70, // Version 7
			UInt8.random(in: 0...255),
			UInt8.random(in: 0...255) & 0x3F | 0x80, // Variant 1
			UInt8.random(in: 0...255),
			UInt8.random(in: 0...255),
			UInt8.random(in: 0...255),
			UInt8.random(in: 0...255),
			UInt8.random(in: 0...255),
			UInt8.random(in: 0...255),
			UInt8.random(in: 0...255)
		)
		return UUID(uuid: uuidBytes)
	}
}
