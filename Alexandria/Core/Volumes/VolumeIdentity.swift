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
	/// The network rung: the schema's ratified `scheme://host/share` form —
	/// case-folded, user stripped — observed from `volumeURLForRemountingKey`
	/// (Apple's cross-remount handle for network volumes). Known, accepted
	/// imperfection: the same share mounted by hostname vs IP folds to two
	/// identities; that fails toward a duplicate volume record, repaired like
	/// any misidentified volume.
	case remount(String)

	/// The heuristic: filesystem UUID, else remount URL, else nil.
	init?(uuid: String?, remountURL: URL?) {
		if let uuid {
			self = .filesystemUUID(uuid)
		} else if let location = Self.canonicalRemount(of: remountURL) {
			self = .remount(location)
		} else {
			return nil
		}
	}

	/// Normalizes an observed remounting URL into the ratified column form:
	/// `scheme://host/share`, case-folded. User and password strip — the
	/// share is the identity, not the account — and the port drops (the
	/// ratified form is portless; a share served on a nonstandard port
	/// aliases into the same accepted duplicate-record imperfection as
	/// hostname-vs-IP). The form is built from DECODED components so the
	/// input's percent-encoding can never reach the identity and non-ASCII
	/// names genuinely case-fold. A `file:` URL or one without a real host
	/// is no identity: this rung must never mint a mount-path-shaped
	/// identity, or a remount at a new path would re-mint the volume.
	private static func canonicalRemount(of url: URL?) -> String? {
		guard let url,
		      let components = URLComponents(url: url, resolvingAgainstBaseURL: true),
		      let scheme = components.scheme?.lowercased(), scheme != "file",
		      let host = components.host, !host.isEmpty
		else { return nil }
		var path = components.path
		while path.hasSuffix("/") {
			path.removeLast()
		}
		return "\(scheme)://\(host.lowercased())\(path.lowercased())"
	}
}

nonisolated extension VolumeIdentity: RawRepresentable, Codable, DatabaseValueConvertible {
	/// The ratified TEXT column form, self-describing by shape (the network
	/// rung carries a scheme prefix, a UUID never does). Codable and
	/// DatabaseValueConvertible ride this pair for free — stdlib and GRDB
	/// both honor RawRepresentable.
	var rawValue: String {
		switch self {
		case .filesystemUUID(let uuid): uuid
		case .remount(let location): location
		}
	}

	init?(rawValue: String) {
		self = rawValue.contains("://") ? .remount(rawValue) : .filesystemUUID(rawValue)
	}
}
