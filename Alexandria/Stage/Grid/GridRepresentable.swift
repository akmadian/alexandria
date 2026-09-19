//
//  GridRepresentable.swift
//  Alexandria
//
//  The AppKit bridge (grid round, 2026-09-12): NSCollectionView wrapped for
//  SwiftUI, with the Coordinator as the ONE place ids and index paths
//  translate — grouping, when its round comes, changes this table and
//  nothing else. Input flows one way around a loop: the collection view
//  interprets gestures natively, the delegate reports the result to the
//  hub, and the hub's state mirrors back guarded by inequality — so an echo
//  dies in one bounce.
//
//  The loading engine prescribed by _design/technical/grid.md lives here:
//  batched resolution (ids → records, cell round 2026-09-18), per-cell
//  decode requests, prefetch, the stamp and judgment watches, and the
//  paint routine with its recycling guard.
//

import AppKit
import GRDB
import Logging
import Nuke
import SwiftUI
import os

/// GridCollectionView's channel; the Coordinator's identically-labeled
/// instance logger shadows this inside the class, so both write "grid".
private nonisolated let log = Logger(label: "grid")

struct GridRepresentable: NSViewRepresentable {
	var workingSet: [SubjectID]
	var answeredQuery: WorkingSetQuery?
	var selection: Set<SubjectID>
	var cursor: SubjectID?
	var columns: Int
	var catalog: Catalog
	var imaging: StageImaging
	var onSelectionChange: (Set<SubjectID>) -> Void
	var onCursorMove: (SubjectID) -> Void
	var onActivate: () -> Void
	/// A reorder drop arrived under a non-manual sort (drag round): the
	/// switch is never silent — GridView presents the confirmation and, on
	/// yes, runs the adoption verb then the arrangement intent.
	var onReorderProposal: (PendingReorder) -> Void

	func makeNSView(context: Context) -> NSScrollView {
		let layout = GridLayout()

		let collectionView = GridCollectionView()
		collectionView.collectionViewLayout = layout
		collectionView.isSelectable = true
		collectionView.allowsMultipleSelection = true
		collectionView.allowsEmptySelection = true
		collectionView.register(GridItem.self, forItemWithIdentifier: GridItem.identifier)
		collectionView.dataSource = context.coordinator
		collectionView.delegate = context.coordinator
		collectionView.prefetchDataSource = context.coordinator
		// Drag round (2026-09-18): source and reorder destination. The
		// non-local mask keeps its .none default — intra-app only, by ruling.
		collectionView.registerForDraggedTypes([DragPayload.assetsType])
		collectionView.setDraggingSourceOperationMask([.move, .copy], forLocal: true)

		let scrollView = NSScrollView()
		scrollView.documentView = collectionView
		scrollView.hasVerticalScroller = true
		context.coordinator.attach(collectionView)
		return scrollView
	}

	func updateNSView(_ scrollView: NSScrollView, context: Context) {
		let coordinator = context.coordinator
		coordinator.configure(catalog: catalog, imaging: imaging)
		coordinator.callbacks = Coordinator.Callbacks(
			selection: onSelectionChange, cursor: onCursorMove, activate: onActivate,
			reorderProposal: onReorderProposal
		)
		coordinator.apply(columns: columns)
		coordinator.apply(workingSet: workingSet, answering: answeredQuery)
		coordinator.mirror(selection: selection, cursor: cursor)
		// A zoom (columns) or a window resize (width) can change the one bucket
		// every cell wants; if it did, visible cells re-request at the new size.
		coordinator.reevaluateBucket()
	}

	func makeCoordinator() -> Coordinator {
		Coordinator()
	}

	/// Deterministic teardown when the renderer is removed (e.g. a grid→loupe
	/// switch): cancel in-flight decodes and the stamp watch now, rather than
	/// leaning on deinit timing and weak-self no-ops to clean up after us.
	static func dismantleNSView(_ nsView: NSScrollView, coordinator: Coordinator) {
		coordinator.teardown()
	}

	@MainActor final class Coordinator: NSObject {
		struct Callbacks {
			var selection: (Set<SubjectID>) -> Void = { _ in }
			var cursor: (SubjectID) -> Void = { _ in }
			var activate: () -> Void = {}
			var reorderProposal: (PendingReorder) -> Void = { _ in }
		}

		var callbacks = Callbacks()

		// The one id ↔ position table.
		private(set) var ids: [SubjectID] = []
		private var indexOf: [SubjectID: Int] = [:]

		private var renderedQuery: WorkingSetQuery?
		private var lastMirroredCursor: SubjectID?
		/// Set when the answer was replaced wholesale (new question, rebuild
		/// after a mode switch): the next mirror reveals the cursor so the
		/// user lands where they left off.
		private var needsCursorReveal = false

		private weak var collectionView: NSCollectionView?
		private let log = Logger(label: "grid")

		// MARK: Image machinery (grid round, 2026-09-12)

		/// The engine and the catalog, injected once from the stage. Optional
		/// only because an in-memory catalog (previews/tests) has no thumbnail
		/// store — then every cell stays on the placeholder ground.
		private var imaging: StageImaging?
		private var catalog: Catalog?
		private var thumbnailStore: ThumbnailStore?
		private var configured = false

		/// The id→records table (asset + representative file; widened from
		/// bare file ids at the cell round, 2026-09-18 — cells render
		/// judgments and file fields off the canonical records): a
		/// per-session resolution cache, filled lazily in visible/prefetch
		/// batches (never eagerly over the whole working set — at the 1M
		/// scale target that read would be the hitch we're avoiding). A
		/// file-less asset resolves with a nil file, so it keeps the
		/// placeholder ground while its judgments still show.
		/// PERF: unbounded for the session; if a full 1M scroll makes it heavy,
		/// an LRU keyed on visited ids is the named upgrade.
		private var recordsOf: [SubjectID: (asset: Asset?, file: File?)] = [:]

		/// Bumped by every invalidation of the table (the judgment refresh);
		/// a resolve that began under an older generation discards its batch
		/// so stale records can't land over a forced re-read's fresher ones.
		private var recordsGeneration = 0

		/// The imaging path's view of the table: pixels want only the elected
		/// file's id. A file subject IS its file, so pixels never wait on the
		/// record read — the record rides later, for decoration alone.
		private func representativeFileId(of id: SubjectID) -> Identifier<File>? {
			switch id {
			case .file(let file): file
			case .asset: recordsOf[id]?.file?.id
			}
		}

		/// The decode size currently on screen for a live cell, so an instant
		/// paint from cache is never overwritten by nothing and a late arrival
		/// can't downgrade a sharper image. Cleared when the cell stops
		/// displaying the id.
		private var shownBucket: [SubjectID: DecodeBucket] = [:]

		/// In-flight content decodes, one per visible cell. There is no second
		/// lane: one size per cell, so nothing to upgrade and nothing to cancel
		/// on behalf of a cosmetic pass.
		private var contentTasks: [SubjectID: ImageTask] = [:]

		/// Heal offers are one-shot per subject per session: a corrupt
		/// thumbnail can't turn its ring into a retry loop.
		private var healed: Set<SubjectID> = []

		/// The one bucket every visible cell currently wants (cells are uniform
		/// at a given zoom, so it's grid-wide). Re-evaluated on a zoom or resize
		/// that changes it; when it changes, visible cells re-request. nil until
		/// first evaluated.
		private var currentBucket: DecodeBucket?

		private var stampObservation: AnyDatabaseCancellable?
		private var judgmentObservation: AnyDatabaseCancellable?

		func attach(_ collectionView: NSCollectionView) {
			self.collectionView = collectionView
		}

		/// Injected once from the stage. The store is derived from the
		/// catalog's directory; the stamp watch starts here so the heal is live
		/// for the session.
		func configure(catalog: Catalog, imaging: StageImaging) {
			guard !configured else { return }
			configured = true
			self.catalog = catalog
			self.imaging = imaging
			self.thumbnailStore = catalog.directory.map { ThumbnailStore(catalogDirectory: $0) }
			startStampObservation()
			startJudgmentObservation()
		}

		/// Cancel everything this coordinator owns. Nuke `ImageTask`s do NOT
		/// self-cancel when their reference is dropped (unlike the DB
		/// cancellable), so without this a mode switch leaves the grid's
		/// in-flight decodes running to completion; cancelling on the main
		/// actor here stops them cleanly (Nuke 12.9: no callback after a
		/// main-thread cancel).
		func teardown() {
			for task in contentTasks.values { task.cancel() }
			contentTasks.removeAll()
			imaging?.prefetcher.stopPrefetching()
			stampObservation?.cancel()
			stampObservation = nil
			judgmentObservation?.cancel()
			judgmentObservation = nil
		}

		// MARK: Translation

		private func id(at indexPath: IndexPath) -> SubjectID? {
			ids.indices.contains(indexPath.item) ? ids[indexPath.item] : nil
		}

		private func indexPath(of id: SubjectID) -> IndexPath? {
			indexOf[id].map { IndexPath(item: $0, section: 0) }
		}

		private func reindex(_ new: [SubjectID]) {
			ids = new
			indexOf = Dictionary(uniqueKeysWithValues: new.enumerated().map { ($1, $0) })
		}

		// MARK: Deliveries → screen

		func apply(workingSet new: [SubjectID], answering query: WorkingSetQuery?) {
			guard let collectionView else { return }
			guard query == renderedQuery else {
				// A new question (or first answer after attach): clean
				// replace; the next mirror reveals the cursor — the hub has
				// already reconciled it, so this lands on first-of-answer
				// for a fresh question and on the surviving cursor for a
				// rebuild.
				renderedQuery = query
				// An unconsumed reflow anchor points into the OLD answer;
				// applying it to this one would open at an arbitrary depth.
				(collectionView.collectionViewLayout as? GridLayout)?.cancelPendingReflow()
				resetLoading()
				reindex(new)
				collectionView.reloadData()
				needsCursorReveal = true
				log.debug("delivery applied as question replace", metadata: [
					"count": "\(new.count)",
				])
				return
			}
			guard new != ids else { return }
			// Same question, new answer: import growth, deletion, a
			// latestImport takeover. Diff within budget applies as targeted
			// batch ops; past budget reloads. Either way the anchor rule
			// holds the viewport still (contract: a delivery never moves
			// you).
			// PERF: deliveries apply unthrottled; if a real import measures
			// as UI churn, a trailing-edge delivery ceiling in the hub is
			// the named upgrade (designed 2026-09-12, deliberately unbuilt).
			// PERF: the diff runs on the main actor; GridDiff is nonisolated
			// and its inputs Sendable, so it can hop off if measured hot.
			let interval = Signposts.grid.beginInterval("apply delivery")
			let clock = ContinuousClock()
			let started = clock.now
			let anchor = captureAnchor()
			var mode = "diff"
			if let ops = GridDiff.compute(from: ids, to: new) {
				reindex(new)
				if !ops.isEmpty {
					collectionView.performBatchUpdates({
						collectionView.deleteItems(at: ops.deletes)
						collectionView.insertItems(at: ops.inserts)
						for move in ops.moves {
							collectionView.moveItem(at: move.from, to: move.to)
						}
					}, completionHandler: { [weak self] _ in
						// The settled moment: inserts/deletes/moves shift
						// later cells' positions WITHOUT reconfiguring them,
						// and only here are the visible paths post-animation.
						self?.refreshVisiblePositions()
					})
				}
			} else {
				mode = "reload, past budget"
				reindex(new)
				collectionView.reloadData()
			}
			restoreAnchor(anchor)
			Signposts.grid.endInterval("apply delivery", interval)
			log.debug("delivery applied", metadata: [
				"mode": "\(mode)",
				"count": "\(new.count)",
				"ms": "\((clock.now - started).milliseconds)",
			])
		}

		// MARK: Anchor — the viewport holds still under it

		private struct Anchor {
			var id: SubjectID
			/// The anchor item's offset from the top of the viewport, so
			/// restore puts it back at the same height, not just on screen.
			var offsetInViewport: CGFloat
		}

		private func captureAnchor() -> Anchor? {
			guard let collectionView else { return nil }
			let viewport = collectionView.visibleRect
			let topmost = collectionView.indexPathsForVisibleItems()
				.compactMap { path -> (SubjectID, NSRect)? in
					guard let id = id(at: path),
						let frame = collectionView.layoutAttributesForItem(at: path)?.frame
					else { return nil }
					return (id, frame)
				}
				.min { ($0.1.minY, $0.1.minX) < ($1.1.minY, $1.1.minX) }
			guard let topmost else { return nil }
			return Anchor(id: topmost.0, offsetInViewport: topmost.1.minY - viewport.minY)
		}

		private func restoreAnchor(_ anchor: Anchor?) {
			guard let anchor, let collectionView,
				let clipView = collectionView.enclosingScrollView?.contentView,
				let path = indexPath(of: anchor.id),
				let frame = collectionView.layoutAttributesForItem(at: path)?.frame
			else { return }  // anchor gone with its record: the offset stands
			collectionView.layoutSubtreeIfNeeded()
			let target = NSPoint(
				x: clipView.bounds.origin.x,
				y: max(0, frame.minY - anchor.offsetInViewport)
			)
			guard abs(target.y - clipView.bounds.origin.y) > 0.5 else { return }
			clipView.scroll(to: target)
			collectionView.enclosingScrollView?.reflectScrolledClipView(clipView)
		}

		// MARK: Hub → view (the echo-guarded mirror)

		func mirror(selection: Set<SubjectID>, cursor: SubjectID?) {
			guard let collectionView else { return }
			let current = Set(collectionView.selectionIndexPaths.compactMap { id(at: $0) })
			if current != selection {
				// Programmatic selection fires no delegate callbacks, and
				// the inequality guard stops the mirror when the view was
				// the author — the echo dies here.
				collectionView.selectionIndexPaths = Set(selection.compactMap { indexPath(of: $0) })
			}
			// Reveal only a cursor that MOVED (or survived a wholesale
			// replace) — never yank a user who scrolled away back to a
			// cursor that didn't go anywhere.
			if let cursor, let path = indexPath(of: cursor),
				needsCursorReveal || cursor != lastMirroredCursor {
				if !collectionView.indexPathsForVisibleItems().contains(path) {
					collectionView.scrollToItems(at: [path], scrollPosition: .nearestHorizontalEdge)
				}
			}
			// Cursor ring on live cells: retire the old holder, crown the new.
			// Cells not on screen catch up at configure time from
			// `lastMirroredCursor`, so this only touches the two movers.
			if cursor != lastMirroredCursor {
				for id in [lastMirroredCursor, cursor] {
					// representedID check: around a lazy reloadData the table
					// and the mounted items briefly disagree — never crown a
					// stale-mounted cell.
					guard let id, let path = indexPath(of: id),
						let item = collectionView.item(at: path) as? GridItem,
						item.representedID == id else { continue }
					item.isCursor = id == cursor
				}
			}
			lastMirroredCursor = cursor
			needsCursorReveal = false
		}

		private func refreshVisiblePositions() {
			guard let collectionView else { return }
			for path in collectionView.indexPathsForVisibleItems() {
				guard let id = id(at: path),
					let item = collectionView.item(at: path) as? GridItem,
					item.representedID == id else { continue }
				item.display(position: path.item)
			}
		}

		// MARK: Layout

		/// The layout owns the column count (one store; round review,
		/// finding 8). A zoom is just a geometry change: the anchor and the
		/// bucket re-evaluation ride the layout's geometry-change mint,
		/// exactly like a width reflow — the scroll-after-build shape this
		/// path used to run was the demolished one (finding 3).
		func apply(columns: Int) {
			guard let layout = collectionView?.collectionViewLayout as? GridLayout,
				layout.columns != columns else { return }
			layout.columns = columns
		}

	}
}

// MARK: - Data source and delegate

extension GridRepresentable.Coordinator: NSCollectionViewDataSource {
	func collectionView(_ collectionView: NSCollectionView, numberOfItemsInSection section: Int) -> Int {
		ids.count
	}

	func collectionView(
		_ collectionView: NSCollectionView, itemForRepresentedObjectAt indexPath: IndexPath
	) -> NSCollectionViewItem {
		let item = collectionView.makeItem(withIdentifier: GridItem.identifier, for: indexPath)
		guard let gridItem = item as? GridItem, let id = id(at: indexPath) else { return item }
		gridItem.onDoubleClick = { [weak self] in self?.callbacks.activate() }
		gridItem.represent(id)
		// A fresh or recycled cell learns cursor identity here; a cursor MOVE
		// between live cells is the mirror's job.
		gridItem.isCursor = id == lastMirroredCursor
		// Decoration data: position from the table, records from the cache
		// (nil until resolution lands — the resolve push catches the cell up).
		gridItem.display(position: indexPath.item)
		let records = recordsOf[id]
		gridItem.display(asset: records?.asset, file: records?.file)
		// A recycled slot must not show its previous id's pixels. Paint from
		// cache immediately if we have anything (so a reload/scope-change of
		// already-seen content never blanks), otherwise the quiet ground.
		// willDisplay then decodes the target. This is why a reload doesn't
		// strobe: cached cells repaint in place, uncached ones show the ground.
		paintFromCacheOrPlaceholder(id, into: gridItem)
		return item
	}
}

extension GridRepresentable.Coordinator: NSCollectionViewDelegate {
	func collectionView(
		_ collectionView: NSCollectionView, willDisplay item: NSCollectionViewItem,
		forRepresentedObjectAt indexPath: IndexPath
	) {
		guard let id = id(at: indexPath) else { return }
		requestContent(for: id)
	}

	func collectionView(
		_ collectionView: NSCollectionView, didEndDisplaying item: NSCollectionViewItem,
		forRepresentedObjectAt indexPath: IndexPath
	) {
		// The indexPath can be stale after a delete; the item knows its id.
		guard let id = (item as? GridItem)?.representedID else { return }
		stopLoading(id)
	}

	func collectionView(_ collectionView: NSCollectionView, didSelectItemsAt indexPaths: Set<IndexPath>) {
		reportSelection(touched: indexPaths)
	}

	func collectionView(_ collectionView: NSCollectionView, didDeselectItemsAt indexPaths: Set<IndexPath>) {
		reportSelection(touched: [])
	}

	/// View → hub: report the RESULTING selection (AppKit already applied
	/// its native click/keyboard semantics); the cursor follows the most
	/// recently touched item. Shift-range cursor nuance is the interaction
	/// round's — "last touched" is the ruled-good-enough default.
	private func reportSelection(touched: Set<IndexPath>) {
		guard let collectionView else { return }
		callbacks.selection(Set(collectionView.selectionIndexPaths.compactMap { id(at: $0) }))
		if let last = touched.max(by: { $0.item < $1.item }), let id = id(at: last) {
			callbacks.cursor(id)
		}
	}
}

// MARK: - Prefetching (speculation ahead of the viewport)

extension GridRepresentable.Coordinator: NSCollectionViewPrefetching {
	func collectionView(_ collectionView: NSCollectionView, prefetchItemsAt indexPaths: [IndexPath]) {
		let ids = indexPaths.compactMap { id(at: $0) }
		guard !ids.isEmpty else { return }
		Task { [weak self] in
			guard let self else { return }
			await self.resolve(ids)
			self.startPrefetch(ids)
		}
	}

	func collectionView(_ collectionView: NSCollectionView, cancelPrefetchingForItemsAt indexPaths: [IndexPath]) {
		stopPrefetch(indexPaths.compactMap { id(at: $0) })
	}
}

// MARK: - The load path (grid round, 2026-09-12; rebuilt clean)

extension GridRepresentable.Coordinator {

	/// A visible cell's demand. One size per cell: paint the best cached size
	/// instantly (progressive, never a blank), then decode to the cell's one
	/// bucket if the cache doesn't already cover it. Resolution is a cache hit
	/// on the common path (prefetch resolved it); a miss resolves this one id,
	/// then re-checks the cell is still on screen before spending a decode.
	func requestContent(for id: SubjectID) {
		guard imaging != nil, thumbnailStore != nil else { return }
		if let file = representativeFileId(of: id) {
			startContent(id: id, file: file)
			// Pixels didn't wait on the record (file subjects answer from
			// their own id) — decoration still wants it if it's missing.
			if recordsOf[id] == nil {
				Task { [weak self] in await self?.resolve([id]) }
			}
			return
		}
		Task { [weak self] in
			guard let self else { return }
			await self.resolve([id])
			guard let file = self.representativeFileId(of: id), self.isVisible(id) else { return }
			self.startContent(id: id, file: file)
		}
	}

	private func startContent(id: SubjectID, file: Identifier<File>) {
		guard let imaging, let thumbnailStore else { return }
		let url = thumbnailStore.url(for: file)
		if let cached = imaging.cachedImage(file: file, fileURL: url, ladder: ThumbnailStore.decodeLadder) {
			paint(cached.image, bucket: cached.bucket, for: id)
		}
		let bucket = currentBucketValue()
		// The cache already covers this cell's size — nothing to decode.
		if let shown = shownBucket[id], shown >= bucket { return }
		let signpost = Signposts.grid.beginInterval("decode content")
		// We only reach loadImage on a cache miss (a hit returned above), so
		// this completion is asynchronous. A cancel — on didEndDisplaying,
		// re-request, or teardown — happens on the main actor, and Nuke 12.9
		// guarantees no callback after a main-thread cancel, so a superseded
		// task never lands here to clobber a newer one. (A Nuke 13 upgrade
		// delivers cancellation as .failure(.cancelled) and reopens this —
		// guard task identity then.)
		contentTasks[id]?.cancel()
		let request = imaging.request(file: file, fileURL: url, bucket: bucket, urgency: .content)
		contentTasks[id] = imaging.pipeline.loadImage(with: request) { [weak self] result in
			MainActor.assumeIsolated {
				Signposts.grid.endInterval("decode content", signpost)
				guard let self else { return }
				self.contentTasks[id] = nil
				switch result {
				case .success(let response):
					self.paint(response.image, bucket: bucket, for: id)
				case .failure(let error):
					// Genuine failure (missing/corrupt bytes) — cancels don't
					// arrive here on 12.9, so this is never cancellation noise.
					self.log.debug("thumbnail decode failed", metadata: [
						"error": "\(error)",
					])
				}
			}
		}
	}

	/// Paint from cache if we hold any size for this id, else the quiet ground.
	/// Used at cell configure so a recycled slot never shows its previous id's
	/// pixels and a reload of already-seen content never blanks.
	func paintFromCacheOrPlaceholder(_ id: SubjectID, into item: GridItem) {
		guard let imaging, let thumbnailStore, let file = representativeFileId(of: id) else {
			shownBucket[id] = nil
			item.showPlaceholder()
			return
		}
		let url = thumbnailStore.url(for: file)
		if let cached = imaging.cachedImage(file: file, fileURL: url, ladder: ThumbnailStore.decodeLadder) {
			shownBucket[id] = cached.bucket
			item.show(cached.image)
		} else {
			shownBucket[id] = nil
			item.showPlaceholder()
		}
	}

	/// Set pixels on whatever cell is at the id's position NOW — the recycling
	/// guard lives here, once, against the id↔position table, not in the cell.
	/// Larger-or-equal pixels only, so a late small decode can't downgrade
	/// what's shown and the instant cache paint isn't clobbered.
	private func paint(_ image: NSImage, bucket: DecodeBucket, for id: SubjectID) {
		guard GridImaging.shouldPaint(incoming: bucket, over: shownBucket[id]) else { return }
		shownBucket[id] = bucket
		guard let collectionView, let path = indexPath(of: id),
			let item = collectionView.item(at: path) as? GridItem else { return }
		item.show(image)
	}

	private func stopLoading(_ id: SubjectID) {
		contentTasks[id]?.cancel()
		contentTasks[id] = nil
		shownBucket[id] = nil
	}

	/// Cancels everything in flight for the outgoing question and clears the
	/// per-cell paint state; the resolution cache and heal offers outlive it.
	private func resetLoading() {
		for task in contentTasks.values { task.cancel() }
		contentTasks.removeAll()
		shownBucket.removeAll()
		imaging?.prefetcher.stopPrefetching()
	}

	// MARK: Resolution — SubjectID → records, batched

	/// Batched records read. `force` re-reads entries already cached (the
	/// judgment refresh); otherwise cached entries are skipped — with one
	/// deliberate exception: an asset's nil-file election is SOFT, re-asked
	/// every time, so a file that joins the asset after its first resolution
	/// can still elect (a hard negative here was the review-caught heal
	/// regression: nothing would ever re-ask, and the subject stayed on the
	/// placeholder for the session).
	///
	/// Writes are generation-guarded: an invalidation bumps the generation,
	/// and a read that began before it discards its batch instead of
	/// republishing pre-invalidation records over fresher ones. A discarded
	/// batch retries once against the current generation so the asking cell
	/// still gets an answer.
	private func resolve(_ ids: [SubjectID], force: Bool = false, isRetry: Bool = false) async {
		var fileIds: [Identifier<File>] = []
		var assetIds: [Identifier<Asset>] = []
		for id in ids {
			switch id {
			case .file(let file):
				if force || recordsOf[id] == nil { fileIds.append(file) }
			case .asset(let asset):
				if force || recordsOf[id] == nil || recordsOf[id]?.file == nil {
					assetIds.append(asset)
				}
			}
		}
		guard !fileIds.isEmpty || !assetIds.isEmpty, let catalog else { return }
		let generation = recordsGeneration
		let signpost = Signposts.grid.beginInterval("resolve batch")
		defer { Signposts.grid.endInterval("resolve batch", signpost) }
		do {
			var fresh: [SubjectID: (asset: Asset?, file: File?)] = [:]
			if !assetIds.isEmpty {
				for (assetId, records) in try await catalog.representativeRecords(for: assetIds) {
					fresh[.asset(assetId)] = (records.asset, records.file)
				}
			}
			if !fileIds.isEmpty {
				for file in try await catalog.files(ids: fileIds) {
					fresh[.file(file.id)] = (nil, file)
				}
			}
			guard generation == recordsGeneration else {
				if !isRetry { await resolve(ids, force: force, isRetry: true) }
				return
			}
			// Assign over standing entries, never through a cleared hole:
			// the pixel path keys off the elected file id and must always
			// find the last known one.
			for (id, records) in fresh { recordsOf[id] = records }
		} catch {
			log.error("record resolution failed", metadata: [
				"error": "\(error)",
				"assets": "\(assetIds.count)",
				"files": "\(fileIds.count)",
			])
		}
		// Cells configured before their records landed catch up here;
		// item(at:) is nil for unmaterialized slots, so this only touches
		// live cells.
		pushRecords(ids)
	}

	/// Freshly resolved records onto whatever live cells display them. The
	/// representedID check keeps the guarantee local: the id↔position table
	/// and the mounted item can briefly disagree around a lazy reloadData.
	private func pushRecords(_ ids: [SubjectID]) {
		guard let collectionView else { return }
		for id in ids {
			guard let records = recordsOf[id], let path = indexPath(of: id),
				let item = collectionView.item(at: path) as? GridItem,
				item.representedID == id else { continue }
			item.display(asset: records.asset, file: records.file)
		}
	}

	// MARK: Prefetch — same bucket as content, lowest urgency

	/// Warms the memory cache ahead of the viewport at the SAME bucket a cell
	/// will request and the lowest urgency: so a prefetched image is a real
	/// cache hit when the cell displays (not a near-miss under a different
	/// key), and speculation can never outrank a visible cell's demand.
	private func startPrefetch(_ ids: [SubjectID]) {
		guard let requests = prefetchRequests(ids) else { return }
		imaging?.prefetcher.startPrefetching(with: requests)
	}

	private func stopPrefetch(_ ids: [SubjectID]) {
		guard let requests = prefetchRequests(ids) else { return }
		imaging?.prefetcher.stopPrefetching(with: requests)
	}

	private func prefetchRequests(_ ids: [SubjectID]) -> [ImageRequest]? {
		guard let imaging, let thumbnailStore else { return nil }
		let bucket = currentBucketValue()
		let requests = ids.compactMap { id -> ImageRequest? in
			guard let file = representativeFileId(of: id) else { return nil }
			return imaging.request(
				file: file, fileURL: thumbnailStore.url(for: file),
				bucket: bucket, urgency: .preheat
			)
		}
		return requests.isEmpty ? nil : requests
	}

	// MARK: Zoom / resize — the one legitimate re-request

	/// The one bucket every visible cell wants can change on a zoom (columns)
	/// or a window resize (width). When it does, re-request visible cells at
	/// the new size — the old image holds until the new one swaps in
	/// atomically, so a zoom sharpens without a blank. This is a deliberate
	/// user action, not scroll churn — the only re-request outside display.
	func reevaluateBucket() {
		guard thumbnailStore != nil, let collectionView else { return }
		let bucket = currentBucketValue()
		guard bucket != currentBucket else { return }
		let visible = collectionView.indexPathsForVisibleItems()
		// A tier crossing re-decodes every visible cell — the one resize
		// event with real decode cost, so it leaves a trace.
		log.debug("decode bucket changed", metadata: [
			"from": "\(currentBucket.map { "\($0.pixels)" } ?? "none")",
			"to": "\(bucket.pixels)",
			"visible": "\(visible.count)",
		])
		currentBucket = bucket
		for path in visible {
			guard let id = id(at: path) else { continue }
			requestContent(for: id)
		}
	}

	// MARK: Heal — the stamp watch and its bounded re-ask

	private func startStampObservation() {
		guard let catalog else { return }
		// One valueless event per commit that touches the thumbnail stamp
		// (write-before-stamp means a stamp implies bytes). Ignored payload;
		// the heal re-asks the bounded question itself.
		stampObservation = DatabaseRegionObservation(tracking: File.select(File.Columns.thumbnailAt))
			.start(in: catalog.databaseWriter) { [weak self] error in
				self?.log.error("stamp observation stopped", metadata: ["error": "\(error)"])
			} onChange: { [weak self] _ in
				Task { @MainActor in self?.heal() }
			}
	}

	/// A stamp landed. The event is valueless, so re-request the visible
	/// placeholders — but only those whose bytes exist NOW (a `fileExists`
	/// check), so a subject still awaiting its own write stays eligible for a
	/// later stamp instead of burning its one-shot on a decode that must fail.
	/// See `GridImaging.shouldOfferHeal` for the rule and why.
	private func heal() {
		guard let collectionView, let thumbnailStore else { return }
		var offered = 0
		var electionless: [SubjectID] = []
		for path in collectionView.indexPathsForVisibleItems() {
			guard let id = id(at: path) else { continue }
			guard let file = representativeFileId(of: id) else {
				// A nil election is soft — the stamp that woke us may belong
				// to a file that has JOINED this asset since it resolved.
				// Re-ask, then run the normal display request for winners.
				if case .asset = id, recordsOf[id] != nil { electionless.append(id) }
				continue
			}
			let exists = FileManager.default.fileExists(atPath: thumbnailStore.url(for: file).path)
			guard GridImaging.shouldOfferHeal(
				placeholder: shownBucket[id] == nil,
				inFlight: contentTasks[id] != nil,
				alreadyOffered: healed.contains(id),
				fileExists: exists
			) else { continue }
			healed.insert(id)
			requestContent(for: id)
			offered += 1
		}
		if !electionless.isEmpty {
			Task { [weak self] in
				guard let self else { return }
				await self.resolve(electionless)
				for id in electionless
				where self.isVisible(id) && self.representativeFileId(of: id) != nil {
					self.requestContent(for: id)
				}
			}
		}
		if offered > 0 {
			log.debug("healed visible placeholders", metadata: ["count": "\(offered)"])
		}
	}

	// MARK: Records watch — judgments stay live on visible cells

	/// One valueless event per commit that touches the assets table (a
	/// rating or flag set from the loupe or inspector, formation during an
	/// import). The refresh re-asks the bounded visible question itself —
	/// same shape as the stamp watch.
	/// PERF: fires per asset-table commit, including import formation; the
	/// work is one batched read over visible ids. If an import measures as
	/// churn, narrowing the tracked region to the judgment columns is the
	/// named upgrade.
	private func startJudgmentObservation() {
		guard let catalog else { return }
		judgmentObservation = DatabaseRegionObservation(tracking: Asset.all())
			.start(in: catalog.databaseWriter) { [weak self] error in
				self?.log.error("judgment observation stopped", metadata: ["error": "\(error)"])
			} onChange: { [weak self] _ in
				Task { @MainActor in self?.refreshVisibleRecords() }
			}
	}

	/// Force-re-resolve the visible asset subjects' records, so a judgment
	/// made anywhere repaints the cells showing it. File subjects carry no
	/// judgments and keep their cached record. Entries are never cleared:
	/// the standing records stay whole for the pixel path (a clear here
	/// blanked reconfiguring cells mid-refresh), and the generation bump
	/// makes any in-flight resolve drop its now-stale batch instead.
	private func refreshVisibleRecords() {
		guard let collectionView else { return }
		let assetSubjects = collectionView.indexPathsForVisibleItems()
			.compactMap { id(at: $0) }
			.filter { if case .asset = $0 { true } else { false } }
		guard !assetSubjects.isEmpty else { return }
		recordsGeneration += 1
		log.debug("judgment refresh", metadata: ["count": "\(assetSubjects.count)"])
		Task { [weak self] in
			await self?.resolve(assetSubjects, force: true)
		}
	}

	// MARK: Bucket selection from live geometry

	private func currentBucketValue() -> DecodeBucket {
		GridImaging.bucket(
			forCellSide: currentCellSide(), scale: currentScale(), ladder: ThumbnailStore.decodeLadder
		)
	}

	private func currentCellSide() -> CGFloat {
		(collectionView?.collectionViewLayout as? GridLayout)?.liveCellSide ?? 0
	}

	private func currentScale() -> CGFloat {
		collectionView?.window?.backingScaleFactor ?? 2
	}

	private func isVisible(_ id: SubjectID) -> Bool {
		guard let collectionView, let path = indexPath(of: id) else { return false }
		return collectionView.indexPathsForVisibleItems().contains(path)
	}
}

// MARK: - Drag and drop (drag round, 2026-09-18)

/// The switch confirmation's payload: outlives the drag session so the
/// dialog can act after the drop is gone. `orderedAssets` is the on-screen
/// order with the drop applied — what the collection's manual order becomes
/// on yes (the adoption verb refuses anything but an exact member cover).
nonisolated struct PendingReorder: Equatable, Sendable {
	var collection: Identifier<Collection>
	var orderedAssets: [Identifier<Asset>]
}

extension GridRepresentable.Coordinator {

	/// The payload mapping (ruled: the payload is ASSETS): an asset subject
	/// is itself; a file subject is its owning asset, read from the records
	/// cache. nil — a formation-pending file, or a record not yet resolved —
	/// refuses that one item's drag (ruled item-level).
	private func assetId(of subject: SubjectID) -> Identifier<Asset>? {
		switch subject {
		case .asset(let id): id
		case .file: recordsOf[subject]?.file?.assetId
		}
	}

	private func assetId(at indexPath: IndexPath) -> Identifier<Asset>? {
		guard let subject = id(at: indexPath) else { return nil }
		return assetId(of: subject)
	}

	/// Working-set-ordered asset ids for a dragged path set, deduplicated —
	/// two file cells of one asset are ONE dragged asset (first occurrence
	/// keeps its place, `reorderMembers`' canonical rule), so the badge and
	/// the zero-add refusal count assets, never cells (round review,
	/// finding 4).
	private func orderedAssets(at indexPaths: Set<IndexPath>) -> [Identifier<Asset>] {
		var seen: Set<Identifier<Asset>> = []
		return indexPaths.sorted { $0.item < $1.item }
			.compactMap { assetId(at: $0) }
			.filter { seen.insert($0).inserted }
	}

	// MARK: Source

	func collectionView(
		_ collectionView: NSCollectionView, canDragItemsAt indexPaths: Set<IndexPath>,
		with event: NSEvent
	) -> Bool {
		// The whole cell is the handle (ruled) — that part is native. The
		// drag starts if ANY dragged cell can name an asset; nameless ones
		// drop item-level in the writer.
		indexPaths.contains { assetId(at: $0) != nil }
	}

	// PERF: a select-all drag mints one NSPasteboardItem + NSDraggingItem
	// per selected cell, synchronously at mouse-down — at the 40k library
	// target that is a visible hang before the gesture starts, and the
	// payload read repeats the cost at drop. Trigger: select-all drags
	// measurably stall. Upgrade: one list-carrying pasteboard item plus
	// image-only dragging items (round review, finding 7).
	func collectionView(
		_ collectionView: NSCollectionView, pasteboardWriterForItemAt indexPath: IndexPath
	) -> (any NSPasteboardWriting)? {
		guard let asset = assetId(at: indexPath) else { return nil }
		return DragPayload.assetItem(asset, ordinal: indexPath.item)
	}

	func collectionView(
		_ collectionView: NSCollectionView, draggingSession session: NSDraggingSession,
		willBeginAt screenPoint: NSPoint, forItemsAt indexPaths: Set<IndexPath>
	) {
		session.draggingFormation = .stack
		var facts: DragContext.ReorderFacts?
		if let query = renderedQuery, query.lens == .assets,
			case .collection(let viewed) = query.source {
			facts = DragContext.ReorderFacts(
				viewedCollection: viewed,
				isManual: query.arrangement.sortKey == .manual,
				filterActive: query.filter != nil
			)
		}
		guard let catalog else { return }
		DragContext.beginAssets(orderedAssets(at: indexPaths), catalog: catalog, reorderFacts: facts)
	}

	func collectionView(
		_ collectionView: NSCollectionView, draggingSession session: NSDraggingSession,
		endedAt screenPoint: NSPoint, dragOperation operation: NSDragOperation
	) {
		DragContext.end()
	}

	// MARK: Destination (reorder)

	private var gapVerdict: DropVerdict? {
		guard let context = DragContext.current else { return nil }
		return DragRules.verdict(over: .gridGap, context: context)
	}

	func collectionView(
		_ collectionView: NSCollectionView, validateDrop draggingInfo: any NSDraggingInfo,
		proposedIndexPath: AutoreleasingUnsafeMutablePointer<NSIndexPath>,
		dropOperation proposedDropOperation: UnsafeMutablePointer<NSCollectionView.DropOperation>
	) -> NSDragOperation {
		switch gapVerdict {
		case .reorderLive, .reorderAdopt:
			// Always the gap, never ON a cell (ruled; the insertion line is
			// the one grid drop affordance). AppKit proposes .on over a
			// cell's middle — retarget to the NEARER gap by cursor position
			// so the whole surface maps to insertions and no cell half
			// silently means "insert to my left" (round review, finding 6;
			// refusing .on outright would leave sliver-thin targets).
			if proposedDropOperation.pointee == .on {
				let path = proposedIndexPath.pointee as IndexPath
				let location = collectionView.convert(draggingInfo.draggingLocation, from: nil)
				if let frame = collectionView.layoutAttributesForItem(at: path)?.frame,
					location.x > frame.midX {
					proposedIndexPath.pointee =
						NSIndexPath(forItem: path.item + 1, inSection: path.section)
				}
				proposedDropOperation.pointee = .before
			}
			return .move
		case .add, .reparent, .refused, nil:
			return []
		}
	}

	func collectionView(
		_ collectionView: NSCollectionView, acceptDrop draggingInfo: any NSDraggingInfo,
		indexPath: IndexPath, dropOperation: NSCollectionView.DropOperation
	) -> Bool {
		guard case .assets(let dragged)? = DragPayload.read(draggingInfo),
			!dragged.isEmpty,
			let query = renderedQuery, case .collection(let collection) = query.source,
			let facts = DragContext.current?.reorderFacts
		else { return false }
		// The verdict was judged against the drag-start facts; the write
		// must land on the same collection they described. A mid-drag
		// source change can't happen through the UI today — this fence
		// keeps that a fact rather than an assumption (round review,
		// finding 11).
		guard facts.viewedCollection == collection else {
			log.error("reorder drop refused: viewed collection changed mid-drag", metadata: [
				"judged": "\(facts.viewedCollection.rawValue.uuidString)",
				"current": "\(collection.rawValue.uuidString)",
			])
			return false
		}
		// The pure math, shared with the adopt path and pinned by tests.
		let plan = DragRules.reorderPlan(ids: ids, dragged: dragged, dropIndex: indexPath.item)
		switch gapVerdict {
		case .reorderLive:
			guard let catalog else { return false }
			return DragVerbs.perform(
				.assets(dragged), verdict: .reorderLive, over: .gridGap,
				reorderingIn: collection, before: plan.anchor, catalog: catalog
			)
		case .reorderAdopt:
			// Assets lens over a non-union, unfiltered collection, so the
			// adopted order IS the member set. Nothing writes here: the
			// confirmation owns the pen (a judgment is never silently
			// overwritten, ruled).
			callbacks.reorderProposal(
				PendingReorder(collection: collection, orderedAssets: plan.adopted)
			)
			return true
		default:
			return false
		}
	}

	// MARK: The cursor label channel (offer hints, ruled-visible refusals)

	/// Called by GridCollectionView's destination overrides on enter AND
	/// every update (the union snapshot can land mid-hover and change the
	/// verdict): the hint or refusal rides the drag image while it applies,
	/// and any other verdict restores — attach and clear are symmetric in
	/// both directions.
	func dragHovered(_ info: any NSDraggingInfo, in view: NSView) {
		guard let context = DragContext.current else { return }
		switch gapVerdict {
		case .reorderAdopt:
			context.attachLabel("Switch to Manual Order…", to: info, in: view)
		case .refused(let message):
			context.attachLabel(message, to: info, in: view)
		case .reorderLive, .add, .reparent, nil:
			context.restoreLabel(info, in: view)
		}
	}

	func dragExited(_ info: any NSDraggingInfo, in view: NSView) {
		DragContext.current?.restoreLabel(info, in: view)
	}
}

// MARK: - Collection view

/// NSCollectionView runs every keystroke through the text key-binding
/// machinery to get arrow navigation, so a printable character ends in an
/// `insertText` it has no use for: it beeps and the event stops there. The
/// window never sees it, and the window is where AppKit fires unmodified menu
/// key equivalents for keys no view handled (measured 2026-09-14; there is no
/// type-select switch on NSCollectionView to turn off). This restores the
/// responder-chain contract every other view keeps: keys the collection view
/// actually handles — the function-flagged ones (arrows, home/end, page
/// up/down) — stay here; everything else goes up the chain and the menu gets
/// its native turn. The only thing given up is type-select, which a grid of
/// thumbnails with no visible text never had a use for.
final class GridCollectionView: NSCollectionView {
	override func keyDown(with event: NSEvent) {
		if event.modifierFlags.contains(.function) {
			super.keyDown(with: event)
		} else {
			nextResponder?.keyDown(with: event)
		}
	}

	// Resize fix (2026-09-19, reordered same day): the settle half of the
	// width-reflow anchor. The scroll lands BEFORE super, so the pass
	// materializes cells for where the viewport is GOING, not where it was.
	// The first shape scrolled after super and repaired the staleness with
	// reconcile passes; deep in a scroll the per-tick jump (rows above ×
	// row-height delta) exceeds the whole viewport, so "one pass behind"
	// meant a blank grid for the entire drag (measured 2026-09-19,
	// coverage probe: 54% viewport coverage at mid-library depth). Scroll-
	// before-build has no staleness to repair — the reconcile machinery
	// (per-tick needsLayout, the live-resize gate, viewDidEndLiveResize)
	// died with it.
	override func layout() {
		let gridLayout = collectionViewLayout as? GridLayout
		// A bounds-change invalidation runs prepare() before this pass, but
		// a plain one (zoom, backing change) defers it into super.layout()
		// — after the pre-build scroll would need its mint. Preparing here
		// is idempotent (an unchanged geometry mints nothing), so every
		// kind of geometry change has its anchor in hand before the build.
		gridLayout?.prepare()
		let target = gridLayout?.takePendingReflowOrigin()
		let signpost = target.map { _ in Signposts.grid.beginInterval("reflow") }
		if let target { scrollClip(to: target) }
		super.layout()
		if let target, let clipView = enclosingScrollView?.contentView,
			abs(clipView.bounds.origin.y - target) > 0.5 {
			// Widening deep in the library: wider cells make taller rows,
			// so the pre-super scroll can clamp against the OUTGOING
			// (shorter) document. The frame has grown by now, so land the
			// rest; the strip this pass built stale is bounded by the
			// clamp and the next pass covers it.
			log.debug("reflow re-landed after clamp", metadata: [
				"target": "\(target)",
				"clamped": "\(clipView.bounds.origin.y)",
			])
			scrollClip(to: target)
		}
		if let signpost { Signposts.grid.endInterval("reflow", signpost) }
		// The one bucket every cell wants can change with the cell size —
		// keyed to the geometry swap itself, not to whether an anchor was
		// placed (round review, finding 7).
		if gridLayout?.takeGeometryChanged() == true {
			(delegate as? GridRepresentable.Coordinator)?.reevaluateBucket()
		}
	}

	/// A 1×↔2× display move changes the pixel grid every edge aligns to
	/// (and the decode tier) with no width change to invalidate for
	/// (round review, finding 6). The geometry-change mint then anchors
	/// and re-buckets like any other reflow.
	override func viewDidChangeBackingProperties() {
		super.viewDidChangeBackingProperties()
		collectionViewLayout?.invalidateLayout()
	}

	private func scrollClip(to y: CGFloat) {
		guard let clipView = enclosingScrollView?.contentView else { return }
		clipView.scroll(to: NSPoint(x: clipView.bounds.origin.x, y: y))
		enclosingScrollView?.reflectScrolledClipView(clipView)
	}

	// Drag round: the delegate protocol has no enter/exit hooks, but the
	// cursor label (switch hint, ruled-visible refusal) needs them — these
	// forward to the coordinator around NSCollectionView's own handling.
	// `updated` re-forwards because the union snapshot can land mid-hover
	// and upgrade a dark gap into an offer (attach is idempotent per text).

	override func draggingEntered(_ sender: any NSDraggingInfo) -> NSDragOperation {
		let operation = super.draggingEntered(sender)
		(delegate as? GridRepresentable.Coordinator)?.dragHovered(sender, in: self)
		return operation
	}

	override func draggingUpdated(_ sender: any NSDraggingInfo) -> NSDragOperation {
		let operation = super.draggingUpdated(sender)
		(delegate as? GridRepresentable.Coordinator)?.dragHovered(sender, in: self)
		return operation
	}

	override func draggingExited(_ sender: (any NSDraggingInfo)?) {
		super.draggingExited(sender)
		if let sender {
			(delegate as? GridRepresentable.Coordinator)?.dragExited(sender, in: self)
		}
	}
}

// MARK: - Instrumentation

/// The stage's signpost lanes: intervals Instruments draws on the same
/// timeline as CPU samples and animation hitches, so "which suspect owns
/// this hitch" is read off a trace instead of inferred. Free when no trace
/// records — they stay in production code.
enum Signposts {
	static let grid = OSSignposter(subsystem: Log.subsystem, category: "grid")
}

extension Duration {
	var milliseconds: Int64 {
		components.seconds * 1_000 + components.attoseconds / 1_000_000_000_000_000
	}
}
