//
//  DragRulesTests.swift
//  AlexandriaTests
//
//  The drag round's ratified table, pinned cell by cell (2026-09-18): a
//  combination not written in DragRules refuses by construction, and these
//  tests are what keeps a future payload or target from silently changing
//  a ruled cell. Payload round-trips through a real pasteboard, ordinal
//  order included — AppKit does not document multi-item order, the ordinal
//  is the contract.
//

import AppKit
import Testing
@testable import Alexandria

@MainActor
struct DragRulesTests {

	private func asset() -> Identifier<Asset> { .mint() }
	private func collection() -> Identifier<Collection> { .mint() }

	// MARK: - Payload round trip

	@Test func assetPayloadRoundTripsInOrdinalOrderRegardlessOfItemOrder() {
		let first = asset()
		let second = asset()
		let third = asset()
		let pasteboard = NSPasteboard(name: .init("test.\(UUID().uuidString)"))
		pasteboard.clearContents()
		// Deliberately shuffled write order; ordinals carry the truth.
		pasteboard.writeObjects([
			DragPayload.assetItem(third, ordinal: 2),
			DragPayload.assetItem(first, ordinal: 0),
			DragPayload.assetItem(second, ordinal: 1),
		])
		#expect(DragPayload.read(pasteboard) == .assets([first, second, third]))
	}

	@Test func collectionPayloadRoundTrips() {
		let id = collection()
		let pasteboard = NSPasteboard(name: .init("test.\(UUID().uuidString)"))
		pasteboard.clearContents()
		pasteboard.writeObjects([DragPayload.collectionItem(id)])
		#expect(DragPayload.read(pasteboard) == .collection(id))
	}

	@Test func foreignPasteboardContentReadsAsNoPayload() {
		let pasteboard = NSPasteboard(name: .init("test.\(UUID().uuidString)"))
		pasteboard.clearContents()
		pasteboard.setString("not ours", forType: .string)
		#expect(DragPayload.read(pasteboard) == nil)
	}

	// MARK: - Assets × collection row (add, badge, zero-add refusal)

	@Test func assetsOverACollectionRowAddWithTheNewCount() {
		let target = collection()
		let dragged = [asset(), asset(), asset()]
		let context = DragContext(payload: .assets(dragged), alreadyHeld: [target: 1])
		#expect(DragRules.verdict(over: .collectionRow(target, smart: false), context: context) == .add(new: 2))
	}

	@Test func assetsStayOptimisticWhileTheMembershipSnapshotIsInFlight() {
		let context = DragContext(payload: .assets([asset()]), alreadyHeld: nil)
		#expect(DragRules.verdict(over: .collectionRow(collection(), smart: false), context: context) == .add(new: nil))
	}

	@Test func aDropThatWouldAddNothingStaysDark() {
		let target = collection()
		let dragged = [asset(), asset()]
		let context = DragContext(payload: .assets(dragged), alreadyHeld: [target: 2])
		#expect(DragRules.verdict(over: .collectionRow(target, smart: false), context: context) == nil)
	}

	@Test func assetsNeverTargetTheHeader() {
		let context = DragContext(payload: .assets([asset()]), alreadyHeld: [:])
		#expect(DragRules.verdict(over: .collectionsHeader, context: context) == nil)
	}

	/// Smart takes no manual adds (smart-collection round, 2026-09-18):
	/// dark, and dark even in the optimistic pre-snapshot window — the
	/// smartness rides the target, not the async context.
	@Test func aSmartCollectionRowStaysDarkForAssets() {
		let target = collection()
		let pending = DragContext(payload: .assets([asset()]), alreadyHeld: nil)
		#expect(DragRules.verdict(over: .collectionRow(target, smart: true), context: pending) == nil)
		let landed = DragContext(payload: .assets([asset()]), alreadyHeld: [:])
		#expect(DragRules.verdict(over: .collectionRow(target, smart: true), context: landed) == nil)
	}

	/// Re-parenting UNDER a smart collection stays legal — children are
	/// tree structure, not membership (ruled 2026-09-18).
	@Test func aCollectionStillReparentsOntoASmartRow() {
		let dragged = collection()
		let target = collection()
		let context = DragContext(
			payload: .collection(dragged), parentOfDraggedCollection: nil,
			draggedSubtree: [dragged]
		)
		#expect(DragRules.verdict(over: .collectionRow(target, smart: true), context: context)
			== .reparent(under: target))
	}

	// MARK: - Assets × grid gap (the ruled reorder gates)

	private func gapVerdict(
		manual: Bool, filter: Bool, union: Bool?, smart: Bool? = false
	) -> DropVerdict? {
		let context = DragContext(
			payload: .assets([asset()]),
			reorderFacts: .init(
				viewedCollection: collection(), isManual: manual,
				filterActive: filter, unionContributes: union,
				sourceIsSmart: smart
			)
		)
		return DragRules.verdict(over: .gridGap, context: context)
	}

	@Test func theGridIsNoTargetOutsideACollectionSource() {
		let context = DragContext(payload: .assets([asset()]))
		#expect(DragRules.verdict(over: .gridGap, context: context) == nil)
	}

	@Test func manualArrangementReordersLive() {
		#expect(gapVerdict(manual: true, filter: false, union: false) == .reorderLive)
	}

	@Test func anotherSortOffersTheConfirmedSwitch() {
		#expect(gapVerdict(manual: false, filter: false, union: false) == .reorderAdopt)
	}

	@Test func aUnionViewRefusesVisiblyNeverSilently() {
		guard case .refused(let message)? = gapVerdict(manual: false, filter: false, union: true) else {
			Issue.record("union reorder must refuse with a visible message")
			return
		}
		#expect(!message.isEmpty)
	}

	/// The round review's finding 1: the union gate outranks manual — a
	/// manual union view refuses exactly like any other union view, and a
	/// pending snapshot keeps even a manual gap dark.
	@Test func manualNeverBypassesTheUnionGate() {
		guard case .refused? = gapVerdict(manual: true, filter: false, union: true) else {
			Issue.record("a manual union view must refuse visibly")
			return
		}
		#expect(gapVerdict(manual: true, filter: false, union: nil) == nil)
	}

	@Test func aFilteredViewRefusesAdoptionVisibly() {
		guard case .refused? = gapVerdict(manual: false, filter: true, union: nil) else {
			Issue.record("filtered adoption must refuse with a visible message")
			return
		}
	}

	@Test func thePendingUnionSnapshotKeepsTheGapDark() {
		#expect(gapVerdict(manual: false, filter: false, union: nil) == nil)
	}

	/// A smart source refuses reorder whole — no manual order exists to
	/// write, on either path — and a pending smartness snapshot keeps the
	/// gap dark, the union snapshot's own stance.
	@Test func aSmartSourceRefusesReorderVisibly() {
		guard case .refused(let message)? = gapVerdict(
			manual: false, filter: false, union: false, smart: true
		) else {
			Issue.record("smart-source reorder must refuse with a visible message")
			return
		}
		#expect(!message.isEmpty)
		guard case .refused? = gapVerdict(manual: true, filter: false, union: false, smart: true) else {
			Issue.record("manual over a smart source must refuse too")
			return
		}
		#expect(gapVerdict(manual: true, filter: false, union: false, smart: nil) == nil)
	}

	/// Review finding 2: KNOWN smart names the real reason even with a
	/// filter active (never "clear the filter" followed by a second
	/// refusal) — while the filter refusal stays instant during the
	/// pending-smartness window for ordinary filtered collections.
	@Test func knownSmartOutranksTheFilterRefusal() {
		guard case .refused(let message)? = gapVerdict(
			manual: false, filter: true, union: false, smart: true
		) else {
			Issue.record("a filtered smart source must refuse as smart")
			return
		}
		#expect(message.contains("smart collection"))
		guard case .refused(let pending)? = gapVerdict(
			manual: false, filter: true, union: nil, smart: nil
		) else {
			Issue.record("a filtered view must keep its instant refusal while smartness is pending")
			return
		}
		#expect(pending.contains("filter"))
	}

	// MARK: - Collection × rows (re-parent and its dark refusals)

	@Test func aCollectionReparentsOntoAnUnrelatedRow() {
		let dragged = collection()
		let target = collection()
		let context = DragContext(
			payload: .collection(dragged), parentOfDraggedCollection: nil,
			draggedSubtree: [dragged]
		)
		#expect(DragRules.verdict(over: .collectionRow(target, smart: false), context: context)
			== .reparent(under: target))
	}

	@Test func ownSubtreeRowsStayDark() {
		let dragged = collection()
		let child = collection()
		let context = DragContext(
			payload: .collection(dragged), draggedSubtree: [dragged, child]
		)
		#expect(DragRules.verdict(over: .collectionRow(dragged, smart: false), context: context) == nil)
		#expect(DragRules.verdict(over: .collectionRow(child, smart: false), context: context) == nil)
	}

	@Test func theCurrentParentStaysDark() {
		let dragged = collection()
		let parent = collection()
		let context = DragContext(
			payload: .collection(dragged), parentOfDraggedCollection: parent,
			draggedSubtree: [dragged]
		)
		#expect(DragRules.verdict(over: .collectionRow(parent, smart: false), context: context) == nil)
	}

	@Test func theHeaderMovesANestedCollectionToRootAndIgnoresRoots() {
		let nested = DragContext(
			payload: .collection(collection()),
			parentOfDraggedCollection: collection(), draggedSubtree: []
		)
		#expect(DragRules.verdict(over: .collectionsHeader, context: nested)
			== .reparent(under: nil))
		let root = DragContext(payload: .collection(collection()))
		#expect(DragRules.verdict(over: .collectionsHeader, context: root) == nil)
	}

	@Test func aCollectionNeverTargetsTheGrid() {
		let context = DragContext(payload: .collection(collection()))
		#expect(DragRules.verdict(over: .gridGap, context: context) == nil)
	}

	// MARK: - The reorder plan (the pure math both drop paths share)

	@Test func planAnchorsOnTheGapAndAdoptsTheOnScreenOrder() {
		let a = asset(), b = asset(), c = asset(), d = asset()
		let ids = [a, b, c, d].map(SubjectID.asset)
		// Drag d before b (gap index 1).
		let plan = DragRules.reorderPlan(ids: ids, dragged: [d], dropIndex: 1)
		#expect(plan.anchor == b)
		#expect(plan.adopted == [a, d, b, c])
	}

	@Test func planDegeneratesPastDraggedCellsToTheirSuccessor() {
		let a = asset(), b = asset(), c = asset(), d = asset()
		let ids = [a, b, c, d].map(SubjectID.asset)
		// Gap sits ON dragged b: the anchor is its first non-dragged
		// successor, and the result equals dropping in the same visual spot.
		let plan = DragRules.reorderPlan(ids: ids, dragged: [b, c], dropIndex: 1)
		#expect(plan.anchor == d)
		#expect(plan.adopted == [a, b, c, d])
	}

	@Test func planAppendsAtTheEndWithANilAnchor() {
		let a = asset(), b = asset(), c = asset()
		let ids = [a, b, c].map(SubjectID.asset)
		let plan = DragRules.reorderPlan(ids: ids, dragged: [a], dropIndex: 3)
		#expect(plan.anchor == nil)
		#expect(plan.adopted == [b, c, a])
	}

	@Test func planInsertsBeforeFirst() {
		let a = asset(), b = asset(), c = asset()
		let ids = [a, b, c].map(SubjectID.asset)
		let plan = DragRules.reorderPlan(ids: ids, dragged: [c], dropIndex: 0)
		#expect(plan.anchor == a)
		#expect(plan.adopted == [c, a, b])
	}

	@Test func planSurvivesTheWholeSelectionBeingDragged() {
		let a = asset(), b = asset()
		let ids = [a, b].map(SubjectID.asset)
		let plan = DragRules.reorderPlan(ids: ids, dragged: [b, a], dropIndex: 0)
		#expect(plan.anchor == nil)
		// The dragged order IS the order: the selection's internal
		// sequence is preserved, not the previous on-screen one.
		#expect(plan.adopted == [b, a])
	}

	@Test func planClampsAnOutOfRangeGapInsteadOfTrapping() {
		let a = asset(), b = asset()
		let ids = [a, b].map(SubjectID.asset)
		#expect(DragRules.reorderPlan(ids: ids, dragged: [a], dropIndex: 99).adopted == [b, a])
		#expect(DragRules.reorderPlan(ids: ids, dragged: [a], dropIndex: -1).adopted == [a, b])
	}

	@Test func assetPayloadDeduplicatesCellsOfOneAsset() {
		let shared = asset()
		let other = asset()
		let pasteboard = NSPasteboard(name: .init("test.\(UUID().uuidString)"))
		pasteboard.clearContents()
		// A RAW and its JPEG in the files lens: two cells, one asset.
		pasteboard.writeObjects([
			DragPayload.assetItem(shared, ordinal: 0),
			DragPayload.assetItem(shared, ordinal: 1),
			DragPayload.assetItem(other, ordinal: 2),
		])
		#expect(DragPayload.read(pasteboard) == .assets([shared, other]))
	}
}
