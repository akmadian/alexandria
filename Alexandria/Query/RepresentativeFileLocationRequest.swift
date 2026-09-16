//
//  RepresentativeFileLocationRequest.swift
//  Alexandria
//
//  Created by ari on 9/16/26.
//

import GRDB
import GRDBQuery
import Logging

struct Location {
	let volume: Volume
	let folder: Folder
}

struct RepresentativeFileLocationRequest: ValueObservationQueryable {
	static var defaultValue: Location? { nil }

	var assetId: Identifier<Asset>
	/// Injected by the caller — this request logs the lookup failures that would
	/// otherwise vanish into the Query's error state, but it owns no logger.
	/// Excluded from equality below: the logger never changes what's fetched, so
	/// swapping it must not restart the observation.
	var log: Logger

	static func == (lhs: Self, rhs: Self) -> Bool { lhs.assetId == rhs.assetId }

	func fetch(_ db: Database) throws -> Location? {
		do {
			guard let asset = try Asset.fetchOne(db, key: assetId) else {
				log.warning("location: asset not found", metadata: ["asset": "\(assetId)"]); return nil
			}
			guard let fileId = asset.representativeFileId else {
				log.warning("location: no representative file", metadata: ["asset": "\(assetId)"]); return nil
			}
			guard let file = try File.fetchOne(db, key: fileId) else {
				log.error("location: file missing", metadata: ["file": "\(fileId)"]); return nil
			}
			guard let folder = try Folder.fetchOne(db, key: file.folderId) else {
				log.error("location: folder missing", metadata: ["folder": "\(file.folderId)"]); return nil
			}
			guard let volume = try Volume.fetchOne(db, key: folder.volumeId) else {
				log.error("location: volume missing", metadata: ["volume": "\(folder.volumeId)"]); return nil
			}
			return Location(volume: volume, folder: folder)
		} catch {
			// A decode/query throw would otherwise vanish into the Query's error state.
			log.error("location fetch failed", metadata: ["asset": "\(assetId)", "error": "\(error)"])
			throw error
		}
	}
}
