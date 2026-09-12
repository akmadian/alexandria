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
import Logging
import SwiftUI
import os

struct GridRepresentable: NSViewRepresentable {
	var workingSet: [SubjectID]
	var answeredQuery: WorkingSetQuery?
	var selection: Set<SubjectID>
	var cursor: SubjectID?
	var columns: Int
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

		let collectionView = NSCollectionView()
		collectionView.collectionViewLayout = layout
		collectionView.isSelectable = true
		collectionView.allowsMultipleSelection = true
		collectionView.allowsEmptySelection = true
		collectionView.register(GridItem.self, forItemWithIdentifier: GridItem.identifier)
		collectionView.dataSource = context.coordinator
		collectionView.delegate = context.coordinator
		context.coordinator.attach(collectionView)

		let scrollView = NSScrollView()
		scrollView.documentView = collectionView
		scrollView.hasVerticalScroller = true
		return scrollView
	}

	func updateNSView(_ scrollView: NSScrollView, context: Context) {
		let coordinator = context.coordinator
		coordinator.callbacks = Coordinator.Callbacks(
			selection: onSelectionChange, cursor: onCursorMove, activate: onActivate
		)
		coordinator.apply(columns: columns)
		coordinator.apply(workingSet: workingSet, answering: answeredQuery)
		coordinator.mirror(selection: selection, cursor: cursor)
	}

	func makeCoordinator() -> Coordinator {
		Coordinator()
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

		func attach(_ collectionView: NSCollectionView) {
			self.collectionView = collectionView
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
