//
//  ImportError.swift
//  Alexandria
//
//  Created by ari on 9/10/26.
//

import Foundation

enum ImportError: Error {
	case sourceUnreadable
	/// A folder row's parent vanished mid-chain-mint: catalog corruption, not
	/// a disk condition.
	case folderChainBroken(pathKey: String)
	/// Imports need the thumbnail store (ruled 2026-09-11), and the store
	/// needs a catalog directory — an in-memory catalog cannot run imports.
	case catalogHasNoDirectory
}

/// One pre-identity walk casualty: a path the walk saw but couldn't read, so
/// it never became a file row. Bound for the import_errors DLQ.
nonisolated struct WalkFailure: Sendable {
	let url: URL
	let reasonCode: String
	let message: String
}
