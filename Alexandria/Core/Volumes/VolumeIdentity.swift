//
//  VolumeIdentity.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//
// 	Who a volume is to the filesystem
//

import Foundation
import GRDB

/// The identity resolution ladder, typed: best evidence wins. Absence stays
/// Optional at the property level — never a case — so unidentified volumes
/// keep the schema's NULL-distinct never-collide semantics.
nonisolated enum VolumeIdentity: Hashable, Sendable {
	case filesystemUUID(String)
	// TODO: network rungs — remount URL as case-folded smb://host/share, nfs://host/export

	/// The heuristic: filesystem UUID, else (future) remount URL, else nil.
	init?(uuid: String?) {
		guard let uuid else { return nil }
		self = .filesystemUUID(uuid)
	}
}

nonisolated extension VolumeIdentity: RawRepresentable, Codable, DatabaseValueConvertible {
	/// The ratified TEXT column form, self-describing by shape (the network
	/// rungs will carry a scheme prefix). Codable and DatabaseValueConvertible
	/// ride this pair for free — stdlib and GRDB both honor RawRepresentable.
	var rawValue: String {
		switch self {
		case .filesystemUUID(let uuid): uuid
		}
	}

	init?(rawValue: String) {
		self = .filesystemUUID(rawValue)
	}
}
