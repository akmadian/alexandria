//
//  FileKind.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

/// The coarse file classification recorded in `files.kind` — the registry
/// row's KIND column. Rawness is a format facet, never a kind: a RAF is an
/// image. Taxonomy carried from the old core's table, minus raw-as-a-kind.
nonisolated enum FileKind: String, Codable, Sendable, CaseIterable {
	case image
	case video
	case audio
	case vector
	case document
	case project
	case sidecar
	/// The universal floor: tracked and shown generically.
	case other
}
