//
//  AssetFormation.swift
//  Alexandria
//

import Foundation
import Logging

/// The asset-formation engine (formation round, 2026-09-11 —
/// _design/grouping.md): decides how an import's committed-but-unformed
/// files group into assets. Pure decision logic: files in, a Resolution
/// out — no database access, no disk IO (evidence is the committed rows'
/// metadata column). The pipeline (ImportRun) invokes it per recorded
/// batch and once more as the final sweep; the catalog persists its
/// clusters (Catalog+Assets). Idempotent by construction: the population
/// is "files where asset_id IS NULL", so the database is the worklist and
/// re-running a pass is always safe.
///
/// The pass is the classic entity-resolution pipeline, staged explicitly:
///
///   generate    candidate relationships from the blocking keys
///   evaluate    each candidate against the ordered rules — pure pairwise
///               judgments; the first confirming rule stamps the edge
///   select      one subject per sidecar (the ratified tie-break)
///   union       admitting edges into clusters
///
/// Batch invariance (ratified 2026-09-11): batch boundaries have NO effect
/// on formation — the outcome equals one all-at-once pass over the same
/// files. Consequence: when a later pass's edge proves two same-import
/// scaffolding assets are one work, they MERGE — the elder asset survives,
/// the others are absorbed (files repointed, provenance untouched, empty
/// rows deleted). Safe today because absorbed assets are this import's own
/// judgment-free scaffolding; the user-state guard is flagged for the
/// judgments round.
///
/// Founding is a rule like any other: generation emits a reflexive
/// candidate per unformed file and the floor rule (registry-last) confirms
/// it for non-sidecars — so every admission is a named registry row, and
/// only orphan sidecars stay pending.
nonisolated enum AssetFormation {
	/// THE registry. Add a grouping behavior = write a rule, add a row.
	/// Order is precedence: the first confirming rule stamps the edge, and
	/// sorted edge processing gives earlier rules provenance priority —
	/// which is why the founding floor sits last.
	static let rules: [FormationRule] = [
		.sidecarAttach,
		.rawRenditionPair,
		.oneAssetPerFile,
	]

	/// A confirmed relationship: two files one rule judged to manifest the
	/// same work.
	struct Edge {
		let a: FormationFile
		let b: FormationFile
		let ruleId: String
	}

	/// Where a cluster's files land.
	enum Destination: Sendable {
		case mint(kind: String)
		case join(Identifier<Asset>)
	}

	/// A resolved cluster — the write instruction Catalog.recordFormedAssets
	/// applies: the destination asset, the admitted members with their
	/// admitting rules' provenance, and any same-import scaffolding assets
	/// the destination absorbs (batch-invariance merge).
	struct Cluster: Sendable {
		let destination: Destination
		let members: [(file: Identifier<File>, rule: String)]
		let absorbed: [Identifier<Asset>]
	}

	struct Resolution: Sendable {
		var clusters: [Cluster] = []
		var sidecarsPending = 0

		var assetsMinted: Int {
			clusters.count { if case .mint = $0.destination { true } else { false } }
		}
		var filesFormed: Int { clusters.reduce(0) { $0 + $1.members.count } }
		var assetsAbsorbed: Int { clusters.reduce(0) { $0 + $1.absorbed.count } }
	}

	/// One pass's decisions. Symmetric and order-free by construction:
	/// candidates are a set, evaluation is pairwise, and the only ordering
	/// is an explicit deterministic sort before union — a TOTAL order after
	/// dedup, which is load-bearing: the input fetch has no ORDER BY, so
	/// this sort is what makes identical inputs yield identical decisions.
	static func form(files: [File], log: Logger = Logger(label: "formation")) -> Resolution {
		var resolution = Resolution()
		let universe = files.map { record in
			let metadata = record.metadata.flatMap(FileMetadata.init(databaseJSON:))
			if record.metadata != nil && metadata == nil {
				// Undecodable must not be silently identical to absent —
				// evidence vanishing without trace is the ambiguity the
				// error-residue design forbids.
				log.warning("Undecodable metadata; treating as absent evidence", metadata: [
					"file": "\(record.name)",
					"fileId": "\(record.id.rawValue)",
				])
			}
			return FormationFile(record: record, metadata: metadata)
		}
		let population = universe.filter { !$0.isFormed }
		if population.isEmpty { return resolution }
		let byId = Dictionary(uniqueKeysWithValues: universe.map { ($0.id, $0) })

		let candidates = generateCandidates(population: population, universe: universe)

		var edges = candidates.compactMap { pair in
			rules.first { $0.confirms(pair.a, pair.b) }
				.map { Edge(a: pair.a, b: pair.b, ruleId: $0.id) }
		}
		// Only edges that admit someone drive anything downstream: an edge
		// whose sidecar is already formed admits nobody and must not union
		// (it would smuggle same-stem strangers into the sidecar's asset).
		edges = edges.filter { !admitted(by: $0).isEmpty }
		edges = selectSidecarSubjects(edges)

		// Deterministic union and provenance order: rule precedence, then ids.
		let precedence = Dictionary(uniqueKeysWithValues: rules.enumerated().map { ($1.id, $0) })
		edges.sort { lhs, rhs in
			(precedence[lhs.ruleId] ?? .max, sortKey(lhs)) < (precedence[rhs.ruleId] ?? .max, sortKey(rhs))
		}

		var clusters = UnionFind()
		var provenance: [Identifier<File>: String] = [:]
		for edge in edges {
			clusters.add(edge.a.id, anchor: edge.a.record.assetId)
			clusters.add(edge.b.id, anchor: edge.b.record.assetId)
			clusters.union(edge.a.id, edge.b.id)
			for member in admitted(by: edge) where provenance[member.id] == nil {
				provenance[member.id] = edge.ruleId
			}
		}

		let orphans = population.filter { $0.isSidecar && provenance[$0.id] == nil }
		resolution.sidecarsPending = orphans.count
		if !orphans.isEmpty {
			log.debug("Sidecars formation-pending, no subject found", metadata: [
				"count": "\(orphans.count)",
				"names": "\(orphans.prefix(20).map(\.record.name).joined(separator: ", "))",
			])
		}

		for group in clusters.groups() {
			let members = group.memberIds.compactMap { id in
				provenance[id].map { (file: id, rule: $0) }
			}
			// Elder-survives: anchors sorted by id (UUIDv7 is time-ordered,
			// so the lowest id is the earliest-minted asset).
			let anchors = group.anchors.sorted { $0.rawValue.uuidString < $1.rawValue.uuidString }
			if members.isEmpty && anchors.count <= 1 { continue }  // fully formed; nothing to write

			let destination: Destination
			var absorbed: [Identifier<Asset>] = []
			if let survivor = anchors.first {
				destination = .join(survivor)
				absorbed = Array(anchors.dropFirst())
				if !absorbed.isEmpty {
					log.debug("Merging same-import scaffolding assets", metadata: [
						"survivor": "\(survivor.rawValue)",
						"absorbed": "\(absorbed.map { "\($0.rawValue)" }.joined(separator: ", "))",
					])
				}
			} else {
				// Kind from the lowest-id non-sidecar member — deterministic,
				// and always present: a sidecar only ever clusters by a
				// confirmed edge to a non-sidecar subject.
				let founder = group.memberIds
					.compactMap { byId[$0] }
					.filter { !$0.isSidecar }
					.min { sortKey($0) < sortKey($1) }
				guard let founder else { continue }
				destination = .mint(kind: founder.record.kind.rawValue)
			}
			resolution.clusters.append(Cluster(
				destination: destination, members: members, absorbed: absorbed
			))
		}

		log.debug("Formation pass resolved", metadata: [
			"population": "\(population.count)",
			"candidates": "\(candidates.count)",
			"edges": "\(edges.count)",
			"minted": "\(resolution.assetsMinted)",
			"formed": "\(resolution.filesFormed)",
			"absorbed": "\(resolution.assetsAbsorbed)",
			"sidecarsPending": "\(resolution.sidecarsPending)",
		])
		return resolution
	}

	/// The blocking stage. Same-folder pairs come from (stem, folder)
	/// buckets; cross-folder pairs are generated only among evidence-bearing
	/// images — the pair rule cannot corroborate a cross-folder pair without
	/// evidence on both sides, so evidence-less cross-folder candidates are
	/// provably wasted (and a realistic colliding stem, cover.jpg in
	/// thousands of album folders, would otherwise go quadratic). Plus each
	/// unformed sidecar's exact-form reach into its stripped stem's
	/// same-folder bucket, and one reflexive founding candidate per
	/// unformed file — the floor rule's input. Only pairs that could admit
	/// someone (at least one unformed endpoint), each pair once.
	private static func generateCandidates(
		population: [FormationFile],
		universe: [FormationFile]
	) -> [(a: FormationFile, b: FormationFile)] {
		var candidates: [(a: FormationFile, b: FormationFile)] = population.map { ($0, $0) }
		var seen = Set<String>()
		func add(_ a: FormationFile, _ b: FormationFile) {
			guard a.id != b.id, !a.isFormed || !b.isFormed else { return }
			let key = min(sortKey(a), sortKey(b)) + "|" + max(sortKey(a), sortKey(b))
			guard seen.insert(key).inserted else { return }
			candidates.append((a, b))
		}
		func pairs(within bucket: [FormationFile]) {
			for i in bucket.indices {
				for j in bucket.indices where j > i {
					add(bucket[i], bucket[j])
				}
			}
		}

		let byStemFolder = Dictionary(grouping: universe) { stemFolderKey($0) }
		for bucket in byStemFolder.values where bucket.count > 1 {
			pairs(within: bucket)
		}

		let evidencedByStem = Dictionary(
			grouping: universe.filter {
				$0.record.kind == .image && ($0.capturedAt != nil || $0.cameraIdentity != nil)
			}
		) { $0.record.fileStem }
		for bucket in evidencedByStem.values where bucket.count > 1 {
			pairs(within: bucket)  // same-folder duplicates dedup via `seen`
		}

		for sidecar in population where sidecar.isSidecar {
			let stem = sidecar.record.fileStem
			guard let dot = stem.lastIndex(of: "."), dot != stem.startIndex else { continue }
			let strippedKey = String(stem[..<dot]) + "\u{1F}" + sidecar.record.folderId.rawValue.uuidString
			for subject in byStemFolder[strippedKey] ?? [] {
				add(sidecar, subject)
			}
		}
		return candidates
	}

	private static func stemFolderKey(_ file: FormationFile) -> String {
		file.record.fileStem + "\u{1F}" + file.record.folderId.rawValue.uuidString
	}

	/// The ratified sidecar tie-break: among a sidecar's confirmed subject
	/// edges keep exactly one — exact convention form first, else a raw
	/// capture, else the lowest subject id. This is also what keeps a
	/// sidecar from bridging a refuted pair.
	private static func selectSidecarSubjects(_ edges: [Edge]) -> [Edge] {
		var bySidecar: [Identifier<File>: [Edge]] = [:]
		var selected: [Edge] = []
		for edge in edges {
			if let pair = sidecarAndSubject(edge.a, edge.b) {
				bySidecar[pair.sidecar.id, default: []].append(edge)
			} else {
				selected.append(edge)
			}
		}
		for group in bySidecar.values {
			let best = group.min { lhs, rhs in
				subjectRank(lhs) < subjectRank(rhs)
			}
			if let best { selected.append(best) }
		}
		return selected
	}

	private static func subjectRank(_ edge: Edge) -> (Int, Int, String) {
		guard let pair = sidecarAndSubject(edge.a, edge.b) else { return (.max, .max, "") }
		return (
			sidecarDescribes(pair.sidecar, pair.subject) ? 0 : 1,
			pair.subject.isRawCapture ? 0 : 1,
			sortKey(pair.subject)
		)
	}

	/// Who an edge admits: its unformed endpoints — except a sidecar–subject
	/// edge admits only the sidecar. A sidecar rides its subject; it never
	/// founds it (the subject's own admission is its pair edge or the floor).
	private static func admitted(by edge: Edge) -> [FormationFile] {
		if edge.a.id == edge.b.id {  // a reflexive founding edge
			return edge.a.isFormed ? [] : [edge.a]
		}
		if let pair = sidecarAndSubject(edge.a, edge.b) {
			return pair.sidecar.isFormed ? [] : [pair.sidecar]
		}
		return [edge.a, edge.b].filter { !$0.isFormed }
	}

	private static func sortKey(_ file: FormationFile) -> String {
		file.id.rawValue.uuidString
	}

	private static func sortKey(_ edge: Edge) -> String {
		min(sortKey(edge.a), sortKey(edge.b)) + "|" + max(sortKey(edge.a), sortKey(edge.b))
	}

	// MARK: - Union-find

	/// Minimal disjoint-set over file ids, carrying the set of existing
	/// assets (anchors) each cluster touches.
	private struct UnionFind {
		private var parent: [Identifier<File>: Identifier<File>] = [:]
		private var anchors: [Identifier<File>: Set<Identifier<Asset>>] = [:]

		mutating func add(_ id: Identifier<File>, anchor: Identifier<Asset>?) {
			if parent[id] == nil { parent[id] = id }
			if let anchor { anchors[find(id), default: []].insert(anchor) }
		}

		mutating func find(_ id: Identifier<File>) -> Identifier<File> {
			var current = id
			while let up = parent[current], up != current {
				parent[current] = parent[up]  // path halving
				current = parent[current] ?? up
			}
			return current
		}

		mutating func union(_ a: Identifier<File>, _ b: Identifier<File>) {
			let rootA = find(a), rootB = find(b)
			guard rootA != rootB else { return }
			parent[rootB] = rootA
			anchors[rootA, default: []].formUnion(anchors[rootB] ?? [])
			anchors[rootB] = nil
		}

		mutating func groups() -> [(memberIds: [Identifier<File>], anchors: Set<Identifier<Asset>>)] {
			var byRoot: [Identifier<File>: [Identifier<File>]] = [:]
			for id in parent.keys {
				byRoot[find(id), default: []].append(id)
			}
			return byRoot.map { root, members in (members, anchors[root] ?? []) }
		}
	}
}
