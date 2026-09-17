//
//  CatalogViewState.swift
//  Alexandria
//
//  The hub (UI-layer design round, 2026-09-11): the one truth for the
//  question the user is asking of the catalog and the position they hold in
//  its answer. Every surface — browser, grid, loupe, inspector, filter bar,
//  menu commands — reads its properties and calls named intents; renderers
//  are pure functions over this object and own no copies. Everything that is
//  NOT shared question/position (the browser's tree content, a cell's
//  record, the inspector's subject detail) observes the catalog directly and
//  stays out of here.
//
//  Writes never pass through the hub: judgments flow UI → named catalog
//  write → commit, and return through the observation like any other change.
//

import Foundation
import GRDB
import Logging
import Observation

/// Which renderer paints the working set. Pure posture: the hub never
/// branches on it — the one legitimate switch is the shell's, picking a
/// body. Provisional shape: compare (a future mode) will pressure the
/// position model and re-open this enum.
nonisolated enum ViewMode: Sendable {
	case grid
	case loupe
}

/// Explicitly MainActor, not just by the app target's default isolation:
/// the observation-swap safety argument (see restartObservation) depends on
/// every intent and delivery sharing the main actor.
@MainActor @Observable
final class CatalogViewState {

	// MARK: Posture — the question and the position

	private(set) var lens: Lens = .assets
	private(set) var source: Source = .library
	// TODO: (collections round, 2026-09-12, ratified) per-source arrangement
	// memory — returning to a collection should restore the arrangement it
	// was left in. Its own round; this stays single-valued until then.
	private(set) var arrangement = Arrangement()
	/// The filter's clause (filter round, 2026-09-16): held as its own
	/// posture field beside source (scope ⊂ filter, ratified 2026-09-11 —
	/// both compile to WHERE, but the browser authors source without
	/// read-modify-writing the filter's tokens, and each clears alone).
	/// Always normalized: never an empty group, so "no filter" has exactly
	/// one representation (nil). DELIBERATELY UNSETTLED: whether the active
	/// filter survives relaunch — unlike gridColumns' settled should-persist
	/// note, the viewpoint-persistence round decides this one.
	private(set) var filter: FilterGroup?
	private(set) var viewMode: ViewMode = .grid
	private(set) var selection: Set<SubjectID> = []
	private(set) var cursor: SubjectID?

	/// Grid density as target columns (ruled 2026-09-12): UI state that
	/// should survive between launches lives HERE — the hub is the surface
	/// the viewpoint round's persistence machinery will read and restore.
	/// Renderer posture like viewMode: changes no question, reloads nothing.
	private(set) var gridColumns = 5
	static let gridColumnRange = 2...12

	// MARK: Answer — what the observation last delivered

	/// Every id the current question yields, in arrangement order. Replaced
	/// atomically by each delivery — the previous answer stands until the
	/// next one lands, so a renderer never clears while a question is
	/// re-asked. Its count IS the working-set count.
	private(set) var workingSet: [SubjectID] = []

	/// The question the standing working set answers. Nil until the first
	/// delivery — an unanswered question is distinguishable from an empty
	/// answer — and comparing it against the posture fields tells a
	/// consumer whether the answer is the current question's or a
	/// superseded one still standing while the swap settles.
	private(set) var answeredQuery: WorkingSetQuery?

	// MARK: Internals

	@ObservationIgnored private let catalog: Catalog
	@ObservationIgnored private var observation: AnyDatabaseCancellable?
	/// The working set's membership, kept beside it for the intents'
	/// contains-checks; rebuilt by each delivery.
	@ObservationIgnored private var members: Set<SubjectID> = []
	@ObservationIgnored private let log = Logger(label: "viewstate")

	init(catalog: Catalog) {
		self.catalog = catalog
		restartObservation()
	}

	// MARK: Intents — question (each swaps the observation)

	func setLens(_ newLens: Lens) {
		guard newLens != lens else { return }
		lens = newLens
		restartObservation()
	}

	func setSource(_ newSource: Source) {
		guard newSource != source else { return }
		source = newSource
		// normalized(for:) only ever changes the sort key, so any
		// difference here IS the manual fallback.
		let normalized = arrangement.normalized(for: newSource)
		if normalized != arrangement {
			logManualFellBack(intent: "setSource", kept: normalized.direction)
			arrangement = normalized
		}
		restartObservation()
	}

	/// The posture rule itself is Arrangement.normalized(for:) — pure, and
	/// pinned synchronously over every cell. The hub owns only the effects:
	/// apply it, tell the story in the log, and — by this guard — never
	/// restart the observation for a question that didn't change.
	func setArrangement(_ newArrangement: Arrangement) {
		let normalized = newArrangement.normalized(for: source)
		guard normalized != arrangement else { return }
		if normalized.sortKey != newArrangement.sortKey {
			logManualFellBack(intent: "setArrangement", kept: normalized.direction)
		}
		arrangement = normalized
		restartObservation()
	}

	/// Replaces the filter, or clears it with nil. The posture rule is
	/// FilterGroup.normalized() — empty groups prune, an empty root IS nil —
	/// so removing the last pill and "clear filter" converge on the same
	/// state and the compiler's identity-element backstop stays unreachable.
	/// Validation is a debug assertion, not a refusal: the P0 UI builds
	/// tokens from the vocabulary enums, so an invalid token here is a
	/// programmer error — the loud, typed refusal guards the persistence
	/// fence, where blobs arrive untrusted.
	func setFilter(_ newFilter: FilterGroup?) {
		let normalized = newFilter?.normalized()
		assert((try? normalized?.validate()) != nil || normalized == nil,
		       "setFilter received an invalid token; the vocabulary enums can't express this")
		guard normalized != filter else { return }
		filter = normalized
		if let normalized {
			// The wire form is the compact canonical rendering; the Swift
			// dump is the fallback if encoding ever fails.
			log.info("filter set", metadata: [
				"filter": "\((try? normalized.serialized()) ?? String(describing: normalized))",
			])
		} else {
			log.info("filter cleared")
		}
		restartObservation()
	}

	/// Authoring manual where it can't apply vs leaving a collection while
	/// in it are different UX stories to a human reading a real run — the
	/// intent says which. Fires only when the standing posture actually
	/// changes; a request normalized into the status quo is silent.
	private func logManualFellBack(
		intent: StaticString, kept direction: Arrangement.Direction
	) {
		log.info("manual arrangement fell back to added", metadata: [
			"intent": "\(intent)",
			"direction": "\(direction)",
			"source": "\(source)",
		])
	}

	// MARK: Intents — posture that changes no question

	/// A renderer swap changes nothing about the question, so nothing
	/// reloads.
	func setViewMode(_ mode: ViewMode) {
		viewMode = mode
	}

	/// Clamps to the sane range so every author (slider, future keys,
	/// restored persistence) shares one rule.
	func setGridColumns(_ count: Int) {
		let clamped = min(max(count, Self.gridColumnRange.lowerBound), Self.gridColumnRange.upperBound)
		guard clamped != gridColumns else { return }
		gridColumns = clamped
	}

	/// Re-asks the current question unconditionally. The recovery lever for
	/// a dead observation: an observation that errors never notifies again,
	/// and the question intents no-op on unchanged posture, so without this
	/// no gesture could revive it.
	func refresh() {
		restartObservation()
	}

	/// Ruling 14 (collections round): deleting the subtree the user is
	/// viewing retargets the source to the library — the working set never
	/// stands on a question whose subject is gone. The delete flow hands in
	/// the verb's returned subtree ids; any other source is untouched.
	func collectionsWereDeleted(_ ids: Set<Identifier<Collection>>) {
		guard case .collection(let viewed) = source, ids.contains(viewed) else { return }
		log.info("viewed collection deleted; source falls back to library", metadata: [
			"collection": "\(viewed.rawValue.uuidString)",
		])
		setSource(.library)
	}

	// MARK: Intents — position (pure memory, no database)

	/// Replaces the selection; ids outside the working set drop (a renderer
	/// racing a delivery can only select what the set holds). Gesture
	/// semantics — ranges, toggles — live with renderers; this is the
	/// primitive they compose.
	func setSelection(_ ids: Set<SubjectID>) {
		selection = ids.intersection(members)
	}

	func moveCursor(to id: SubjectID) {
		guard members.contains(id) else {
			log.debug("cursor move to unknown id ignored", metadata: [
				"id": "\(id)",
				"workingSetCount": "\(workingSet.count)",
			])
			return
		}
		cursor = id
	}

	// MARK: Position — derived

	/// The assets a judgment lands on: the selected assets, or the cursor
	/// asset when nothing is selected (ruled 2026-09-14). File-lens members
	/// are skipped — judgments attach to assets.
	///
	/// Derived from position, so it lives beside it: every door into a
	/// judgment — the inspector's controls, the command runner's keys and
	/// menu items — reads this one answer rather than re-deriving the rule.
	var judgmentTargets: [Identifier<Asset>] {
		let selected = selection.compactMap { subject -> Identifier<Asset>? in
			if case .asset(let id) = subject { return id }
			return nil
		}
		if !selected.isEmpty { return selected }
		if case .asset(let id) = cursor { return [id] }
		return []
	}

	// MARK: The observation

	/// Cancels the current observation and asks the new question. A
	/// superseded answer cannot land: cancellation nils the observer's
	/// callbacks synchronously, deliveries hop the main queue and re-check
	/// them, and this method runs on the main actor — so cancel always
	/// happens-before any queued delivery executes (verified against GRDB's
	/// observer implementation; pinned by
	/// rapidQuestionSwapsLandOnTheLastQuestion).
	private func restartObservation() {
		observation?.cancel()
		let query = WorkingSetQuery(lens: lens, source: source, arrangement: arrangement, filter: filter)
		log.debug("observation swap", metadata: ["query": "\(query)"])
		observation = ValueObservation
			.tracking { try query.fetchIdentifiers($0) }
			.removeDuplicates()
			.start(in: catalog.reader) { [weak self] error in
				// A failed observation never notifies again: the standing
				// answer is stale until a question intent or refresh().
				self?.log.error("working-set observation stopped (dead until refresh)", metadata: [
					"query": "\(query)",
					"error": "\(error)",
				])
			} onChange: { [weak self] ids in
				self?.deliver(ids, answering: query)
			}
	}

	/// Every working-set replacement funnels here, so reconciliation is
	/// written exactly once — for posture swaps and external mutation alike.
	/// Minimal defaults: vanished selection drops, a vanished cursor falls
	/// to first (the ratified survival policies arrive with the interaction
	/// rounds and slot in here). Position writes are guarded so an
	/// unchanged selection or cursor fires no mutation.
	private func deliver(_ ids: [SubjectID], answering query: WorkingSetQuery) {
		log.trace("working set delivered", metadata: ["count": "\(ids.count)"])
		workingSet = ids
		members = Set(ids)
		answeredQuery = query
		let reconciled = selection.intersection(members)
		if reconciled != selection {
			selection = reconciled
		}
		if let cursor, members.contains(cursor) {
			// The cursor's subject survived the replacement; it stays.
		} else if cursor != ids.first {
			cursor = ids.first
		}
	}
}
