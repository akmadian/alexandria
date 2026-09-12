//
//  GridDiff.swift
//  Alexandria
//
//  The pure half of the grid's delivery path (grid round, 2026-09-12):
//  translates a working-set replacement into NSCollectionView batch ops.
//  The stdlib computes the diff; this file's whole job is the coordinate
//  contract AppKit demands — deletes and move-origins in OLD positions,
//  inserts and move-destinations in NEW positions — pinned by tests with
//  no AppKit in the loop.
//

// AppKit only for IndexPath's (item:section:) spelling — no view types
// in this file; the logic stays pure and main-actor-free.
import AppKit
import Foundation

nonisolated enum GridDiff {

	struct Move: Hashable {
		var from: IndexPath
		var to: IndexPath
	}

	struct Ops: Equatable {
		var deletes: Set<IndexPath> = []
		var inserts: Set<IndexPath> = []
		var moves: [Move] = []
		var isEmpty: Bool { deletes.isEmpty && inserts.isEmpty && moves.isEmpty }
	}

	/// Animating a wholesale replacement is worse than useless — thousands
	/// of flying cells and a quadratic-ish diff to compute them. Past this
	/// many changes the caller reloads instead. Import growth stays far
	/// under it; a latestImport takeover sails past it by design.
	static let changeBudget = 512

	/// Ops for the batch update, or nil when the change is too large to be
	/// worth animating (or computing): membership churn beyond the budget
	/// means reload. The budget check is set arithmetic, O(n), and runs
	/// BEFORE the diff — the diff's worst case (two mostly-disjoint lists)
	/// is the exact case the budget exists to skip.
	static func compute(from old: [SubjectID], to new: [SubjectID]) -> Ops? {
		let shared = Set(old).intersection(Set(new)).count
		guard (old.count - shared) + (new.count - shared) <= changeBudget else { return nil }

		var ops = Ops()
		for change in new.difference(from: old).inferringMoves() {
			switch change {
			case .remove(let offset, _, let associatedWith):
				// A removal paired with an insertion is one move, recorded
				// once when its insertion half arrives.
				if associatedWith == nil {
					ops.deletes.insert(IndexPath(item: offset, section: 0))
				}
			case .insert(let offset, _, let associatedWith):
				if let from = associatedWith {
					ops.moves.append(Move(
						from: IndexPath(item: from, section: 0),
						to: IndexPath(item: offset, section: 0)
					))
				} else {
					ops.inserts.insert(IndexPath(item: offset, section: 0))
				}
			}
		}
		return ops
	}
}
