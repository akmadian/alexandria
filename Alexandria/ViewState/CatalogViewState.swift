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
	private(set) var arrangement = Arrangement()
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
		restartObservation()
	}

	func setArrangement(_ newArrangement: Arrangement) {
		guard newArrangement != arrangement else { return }
		arrangement = newArrangement
		restartObservation()
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
		let query = WorkingSetQuery(lens: lens, source: source, arrangement: arrangement)
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
