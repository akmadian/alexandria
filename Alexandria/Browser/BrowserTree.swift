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
		/// For the header's availability lookup against VolumeMonitor;
		/// nil = unidentified, which the header renders as "unknown".
		let identity: VolumeIdentity?
		let kind: VolumeKind
		let roots: [FolderNode]
	}

	struct FolderNode: Identifiable, Equatable, Sendable {
		let id: Identifier<Folder>
		let name: String
		let children: [FolderNode]

		/// OutlineGroup's optional-children spelling: nil = leaf, no chevron.
		var outlineChildren: [FolderNode]? { children.isEmpty ? nil : children }
	}

	struct CollectionNode: Identifiable, Equatable, Sendable {
		let id: Identifier<Collection>
		let name: String
		let children: [CollectionNode]
	}

	var volumes: [VolumeNode]
	/// The collection roots (collections round, chunk 4), FinderOrder like
	/// everything else in the sidebar — and by construction the SAME
	/// sequence the union walk sections by (one comparator, ruling 5).
	var collections: [CollectionNode]
	/// Root folders whose LATEST import is unfinished — the resume badge's
	/// durable truth (it must survive relaunch, so it can't live in the
	/// service's session registry). Mirrors Catalog.unfinishedImport's
	/// predicate exactly (latest by started_at desc, id desc; outcome ≠
	/// completed); that method is the master, this is its set-valued twin
	/// for the tree observation.
	var unfinishedImports: Set<Identifier<Folder>>

	static let empty = BrowserTree(volumes: [], collections: [], unfinishedImports: [])

	/// Names an id-carrying source for the hub's title intent (the click
	/// site hands the name over; the hub reads no catalog content). Linear
	/// walk over a sidebar-sized tree at click frequency.
	func name(of source: Source) -> String? {
		switch source {
		case .folder(let id):
			found(id, in: volumes.flatMap(\.roots), children: \.children)?.name
		case .collection(let id):
			found(id, in: collections, children: \.children)?.name
		case .library, .latestImport, .import:
			nil
		}
	}

	private func found<Node: Identifiable>(
		_ id: Node.ID, in nodes: [Node], children: (Node) -> [Node]
	) -> Node? {
		for node in nodes {
			if node.id == id { return node }
			if let hit = found(id, in: children(node), children: children) { return hit }
		}
		return nil
	}

	/// Fetches and assembles the whole tree. Ordering is Finder-style
	/// (ruled 2026-09-11): FinderOrder — localizedStandardCompare on the
	/// display name, id as a total-order tiebreak.
	static func fetch(_ database: Database) throws -> BrowserTree {
		let volumes = try Volume.fetchAll(database)
		let folders = try Folder.fetchAll(database)
		let collections = try Collection.fetchAll(database)

		var childrenOf: [Identifier<Folder>: [Folder]] = [:]
		var rootsOf: [Identifier<Volume>: [Folder]] = [:]
		for folder in folders {
			if let parent = folder.parentId {
				childrenOf[parent, default: []].append(folder)
			} else {
				rootsOf[folder.volumeId, default: []].append(folder)
			}
		}

		func ordered(_ list: [Folder]) -> [Folder] {
			list.sorted {
				FinderOrder.ascending(
					(name: $0.name, id: $0.id.rawValue),
					(name: $1.name, id: $1.id.rawValue)
				)
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
				FinderOrder.ascending(
					(name: $0.name, id: $0.id.rawValue),
					(name: $1.name, id: $1.id.rawValue)
				)
			}
			.map { volume in
				VolumeNode(
					id: volume.id,
					name: volume.name,
					identity: volume.identity,
					kind: volume.kind,
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

		// The collections tree (collections round, chunk 4): same assembly
		// shape as folders — roots are parentId NULL, children FinderOrder.
		var collectionChildren: [Identifier<Collection>: [Collection]] = [:]
		var collectionRoots: [Collection] = []
		for collection in collections {
			if let parent = collection.parentId {
				collectionChildren[parent, default: []].append(collection)
			} else {
				collectionRoots.append(collection)
			}
		}
		func orderedCollections(_ list: [Collection]) -> [Collection] {
			list.sorted {
				FinderOrder.ascending(
					(name: $0.name, id: $0.id.rawValue),
					(name: $1.name, id: $1.id.rawValue)
				)
			}
		}
		var collectionsReached = 0
		func collectionNode(_ collection: Collection) -> CollectionNode {
			collectionsReached += 1
			return CollectionNode(
				id: collection.id,
				name: collection.name,
				children: orderedCollections(collectionChildren[collection.id] ?? []).map(collectionNode)
			)
		}
		let collectionNodes = orderedCollections(collectionRoots).map(collectionNode)
		if collectionsReached != collections.count {
			// Same fence as folders: FK RESTRICT makes a dangling parent
			// unreachable; loud if that ever stops being true.
			Logger(label: "browser").warning("tree assembly dropped collections", metadata: [
				"reached": "\(collectionsReached)",
				"total": "\(collections.count)",
			])
		}

		// Latest-per-folder unfinished imports (comment above names the
		// master predicate). One window pass; imports is bracket-sized.
		let unfinished = try Identifier<Folder>.fetchSet(database, sql: """
			SELECT folder_id FROM (
				SELECT folder_id, outcome,
					ROW_NUMBER() OVER (PARTITION BY folder_id
						ORDER BY started_at DESC, id DESC) AS rn
				FROM imports
			) WHERE rn = 1 AND (outcome IS NULL OR outcome <> 'completed')
			  AND folder_id IS NOT NULL
			""")

		return BrowserTree(
			volumes: volumeNodes,
			collections: collectionNodes,
			unfinishedImports: unfinished
		)
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
				: VolumeNode(
					id: volume.id, name: volume.name,
					identity: volume.identity, kind: volume.kind, roots: roots
				)
		}
		// The filter is the FOLDER filter (browser round: its prompt says
		// so); collections pass through untouched. Widening it is a future
		// call, not a silent behavior change. The unfinished set rides
		// along whole — a badge belongs to its folder, filtered or not.
		return BrowserTree(
			volumes: volumes, collections: collections,
			unfinishedImports: unfinishedImports
		)
	}
}
