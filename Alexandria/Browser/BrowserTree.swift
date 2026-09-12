//
//  BrowserTree.swift
//  Alexandria
//
//  The sidebar's pure value side (browser round, 2026-09-11): the volumes
//  and folders tables assembled into the tree the browser renders.
//  Nonisolated and view-free so the fetch-and-assemble runs inside a
//  ValueObservation's off-main work and pins under test without any UI.
//  Browser-private by design — not shared view state, so not the hub's.
//

import Foundation
import GRDB
import Logging

nonisolated struct BrowserTree: Equatable, Sendable {

	struct VolumeNode: Identifiable, Equatable, Sendable {
		let id: Identifier<Volume>
		let name: String
		let roots: [FolderNode]
	}

	struct FolderNode: Identifiable, Equatable, Sendable {
		let id: Identifier<Folder>
		let name: String
		let children: [FolderNode]

		/// OutlineGroup's optional-children spelling: nil = leaf, no chevron.
		var outlineChildren: [FolderNode]? { children.isEmpty ? nil : children }
	}

	var volumes: [VolumeNode]

	static let empty = BrowserTree(volumes: [])

	/// Fetches and assembles the whole tree. Ordering is Finder-style
	/// (ruled 2026-09-11): localizedStandardCompare on the display name —
	/// "Shoot 9" before "Shoot 10" — with id as a total-order tiebreak.
	/// Deterministic per locale, which is what a presentation ordering owes.
	static func fetch(_ database: Database) throws -> BrowserTree {
		let volumes = try Volume.fetchAll(database)
		let folders = try Folder.fetchAll(database)

		var childrenOf: [Identifier<Folder>: [Folder]] = [:]
		var rootsOf: [Identifier<Volume>: [Folder]] = [:]
		for folder in folders {
			if let parent = folder.parentId {
				childrenOf[parent, default: []].append(folder)
			} else {
				rootsOf[folder.volumeId, default: []].append(folder)
			}
		}

		func finderOrdered(_ names: (String, String), tiebreak: (UUID, UUID)) -> Bool {
			switch names.0.localizedStandardCompare(names.1) {
			case .orderedAscending: true
			case .orderedDescending: false
			case .orderedSame: tiebreak.0.uuidString < tiebreak.1.uuidString
			}
		}
		func ordered(_ list: [Folder]) -> [Folder] {
			list.sorted {
				finderOrdered(($0.name, $1.name), tiebreak: ($0.id.rawValue, $1.id.rawValue))
			}
		}
		var reached = 0
		func node(_ folder: Folder) -> FolderNode {
			reached += 1
			return FolderNode(
				id: folder.id,
				name: folder.name,
				children: ordered(childrenOf[folder.id] ?? []).map(node)
			)
		}

		let volumeNodes = volumes
			.sorted {
				finderOrdered(($0.name, $1.name), tiebreak: ($0.id.rawValue, $1.id.rawValue))
			}
			.map { volume in
				VolumeNode(
					id: volume.id,
					name: volume.name,
					roots: ordered(rootsOf[volume.id] ?? []).map(node)
				)
			}
		if reached != folders.count {
			// A folder whose parent isn't in the table would vanish with its
			// whole subtree. FK RESTRICT makes this unreachable today; loud
			// if that ever stops being true.
			Logger(label: "browser").warning("tree assembly dropped folders", metadata: [
				"reached": "\(reached)",
				"total": "\(folders.count)",
			])
		}
		return BrowserTree(volumes: volumeNodes)
	}

	/// The filter field's behavior: a folder matching on its OWN name keeps
	/// its whole subtree (ruled 2026-09-11 — the sidebar must not show as a
	/// leaf what the grid shows as a subtree); a folder kept only for a
	/// matching descendant keeps just the surviving path, so every match is
	/// reachable. Volumes with no surviving roots drop. Empty or whitespace
	/// text returns the tree untouched.
	func filtered(by text: String) -> BrowserTree {
		let needle = text.trimmingCharacters(in: .whitespacesAndNewlines)
		guard !needle.isEmpty else { return self }

		func surviving(_ node: FolderNode) -> FolderNode? {
			if node.name.range(of: needle, options: [.caseInsensitive]) != nil {
				return node
			}
			let children = node.children.compactMap(surviving)
			guard !children.isEmpty else { return nil }
			return FolderNode(id: node.id, name: node.name, children: children)
		}

		let volumes = self.volumes.compactMap { volume -> VolumeNode? in
			let roots = volume.roots.compactMap(surviving)
			return roots.isEmpty
				? nil
				: VolumeNode(id: volume.id, name: volume.name, roots: roots)
		}
		return BrowserTree(volumes: volumes)
	}
}
