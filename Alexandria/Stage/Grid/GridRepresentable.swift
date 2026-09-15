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
//  Image loading is deliberately absent: cells render the placeholder
//  ground only. The loading engine is prescribed by
//  _design/technical/grid.md and lands in its own build.
//

import AppKit
import GRDB
import Logging
import Nuke
import SwiftUI
import os

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

	func makeNSView(context: Context) -> NSScrollView {
		let layout = GridFlowLayout()
		layout.minimumInteritemSpacing = Theme.Grid.spacing
		layout.minimumLineSpacing = Theme.Grid.spacing
		layout.sectionInset = NSEdgeInsets(
			top: Theme.Grid.inset, left: Theme.Grid.inset,
			bottom: Theme.Grid.inset, right: Theme.Grid.inset
		)

		let collectionView = GridCollectionView()
		collectionView.collectionViewLayout = layout
		collectionView.isSelectable = true
		collectionView.allowsMultipleSelection = true
		collectionView.allowsEmptySelection = true
		collectionView.register(GridItem.self, forItemWithIdentifier: GridItem.identifier)
		collectionView.dataSource = context.coordinator
		collectionView.delegate = context.coordinator
		collectionView.prefetchDataSource = context.coordinator

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
			selection: onSelectionChange, cursor: onCursorMove, activate: onActivate
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
		}

		var callbacks = Callbacks()

		// The one id ↔ position table.
		private(set) var ids: [SubjectID] = []
		private var indexOf: [SubjectID: Int] = [:]

		private var renderedQuery: WorkingSetQuery?
		private var targetColumns = CatalogViewState.gridColumnRange.lowerBound
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

		/// The id→representative-file table: a per-session resolution cache,
		/// filled lazily in visible/prefetch batches (never eagerly over the
		/// whole working set — at the 1M scale target that read would be the
		/// hitch we're avoiding). File-less assets never land here, so they
		/// resolve to the placeholder ground.
		/// PERF: unbounded for the session; if a full 1M scroll makes it heavy,
		/// an LRU keyed on visited ids is the named upgrade.
		private var fileOf: [SubjectID: Identifier<File>] = [:]

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
			// latestImport takeover. Diff within budget animates; past
			// budget reloads. Either way the anchor rule holds the viewport
			// still (contract: a delivery never moves you).
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
					collectionView.performBatchUpdates {
						collectionView.deleteItems(at: ops.deletes)
						collectionView.insertItems(at: ops.inserts)
						for move in ops.moves {
							collectionView.moveItem(at: move.from, to: move.to)
						}
					}
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
			lastMirroredCursor = cursor
			needsCursorReveal = false
		}

		// MARK: Layout

		func apply(columns: Int) {
			guard columns != targetColumns else { return }
			let anchor = captureAnchor()
			targetColumns = columns
			collectionView?.collectionViewLayout?.invalidateLayout()
			collectionView?.layoutSubtreeIfNeeded()
			restoreAnchor(anchor)
			// The bucket re-evaluation (driven from updateNSView) picks up the
			// new cell size and re-requests visible cells if the tier changed.
		}

		private func cellSize(in collectionView: NSCollectionView) -> NSSize {
			let columns = CGFloat(targetColumns)
			let available = collectionView.bounds.width
				- Theme.Grid.inset * 2
				- Theme.Grid.spacing * (columns - 1)
			let side = max(Theme.Grid.minimumCellSide, floor(available / columns))
			return NSSize(width: side, height: side)
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
		// A recycled slot must not show its previous id's pixels. Paint from
		// cache immediately if we have anything (so a reload/scope-change of
		// already-seen content never blanks), otherwise the quiet ground.
		// willDisplay then decodes the target. This is why a reload doesn't
		// strobe: cached cells repaint in place, uncached ones show the ground.
		paintFromCacheOrPlaceholder(id, into: gridItem)
		return item
	}
}

extension GridRepresentable.Coordinator: NSCollectionViewDelegateFlowLayout {
	func collectionView(
		_ collectionView: NSCollectionView, layout collectionViewLayout: NSCollectionViewLayout,
		sizeForItemAt indexPath: IndexPath
	) -> NSSize {
		cellSize(in: collectionView)
	}

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
		if let file = fileOf[id] {
			startContent(id: id, file: file)
			return
		}
		Task { [weak self] in
			guard let self else { return }
			await self.resolve([id])
			guard let file = self.fileOf[id], self.isVisible(id) else { return }
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
		guard let imaging, let thumbnailStore, let file = fileOf[id] else {
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

	// MARK: Resolution — SubjectID → representative file, batched

	private func resolve(_ ids: [SubjectID]) async {
		var assetIds: [Identifier<Asset>] = []
		for id in ids where fileOf[id] == nil {
			switch id {
			case .file(let file): fileOf[id] = file
			case .asset(let asset): assetIds.append(asset)
			}
		}
		guard !assetIds.isEmpty, let catalog else { return }
		let signpost = Signposts.grid.beginInterval("resolve batch")
		defer { Signposts.grid.endInterval("resolve batch", signpost) }
		do {
			let map = try await catalog.representativeFileIds(for: assetIds)
			for (asset, file) in map { fileOf[.asset(asset)] = file }
			// Assets absent from the map are file-less — they stay unresolved
			// and keep the placeholder ground (invariant 4).
		} catch {
			log.error("representative resolution failed", metadata: ["error": "\(error)"])
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
			guard let file = fileOf[id] else { return nil }
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
		currentBucket = bucket
		for path in collectionView.indexPathsForVisibleItems() {
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
		for path in collectionView.indexPathsForVisibleItems() {
			guard let id = id(at: path), let file = fileOf[id] else { continue }
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
		if offered > 0 {
			log.debug("healed visible placeholders", metadata: ["count": "\(offered)"])
		}
	}

	// MARK: Bucket selection from live geometry

	private func currentBucketValue() -> DecodeBucket {
		GridImaging.bucket(
			forCellSide: currentCellSide(), scale: currentScale(), ladder: ThumbnailStore.decodeLadder
		)
	}

	private func currentCellSide() -> CGFloat {
		guard let collectionView else { return Theme.Grid.minimumCellSide }
		return cellSize(in: collectionView).width
	}

	private func currentScale() -> CGFloat {
		collectionView?.window?.backingScaleFactor ?? 2
	}

	private func isVisible(_ id: SubjectID) -> Bool {
		guard let collectionView, let path = indexPath(of: id) else { return false }
		return collectionView.indexPathsForVisibleItems().contains(path)
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
}

// MARK: - Layout

/// Flow layout that reflows when the viewport's width changes — a window
/// resize re-asks the delegate for cell sizes, so columns track the pane.
final class GridFlowLayout: NSCollectionViewFlowLayout {
	override func shouldInvalidateLayout(forBoundsChange newBounds: NSRect) -> Bool {
		newBounds.width != collectionView?.bounds.width
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
