//
//  GridCollectionViewTests.swift
//  AlexandriaTests
//
//  The grid's key contract (keybinding round, 2026-09-14): a printable key
//  the collection view has no use for goes up the responder chain — so the
//  window can fire the menu's unmodified key equivalents — while navigation
//  keys stay with the collection view. The fix for judgment keys beeping and
//  never reaching the Judgments menu; this is the test that would have
//  caught it.
//

import AppKit
import Testing
@testable import Alexandria

@MainActor
struct GridCollectionViewTests {

	/// A parent view that records what reached it — the collection view's
	/// next responder, standing in for the clip view / window chain.
	private final class RecordingParent: NSView {
		var received: [String] = []
		override func keyDown(with event: NSEvent) {
			received.append(event.charactersIgnoringModifiers ?? "")
		}
	}

	/// Three items in a window, so the collection view has something to
	/// navigate and its own arrow handling is exercised rather than a fall
	/// back on empty content (an empty collection view forwards arrows too).
	private final class ThreeItems: NSObject, NSCollectionViewDataSource {
		func collectionView(_ collectionView: NSCollectionView, numberOfItemsInSection section: Int) -> Int { 3 }
		func collectionView(_ collectionView: NSCollectionView, itemForRepresentedObjectAt indexPath: IndexPath) -> NSCollectionViewItem {
			collectionView.makeItem(withIdentifier: GridItem.identifier, for: indexPath)
		}
	}

	private let dataSource = ThreeItems()

	private func makeChain() -> (RecordingParent, GridCollectionView) {
		let parent = RecordingParent(frame: NSRect(x: 0, y: 0, width: 400, height: 400))
		let grid = GridCollectionView(frame: parent.bounds)
		let layout = GridLayout()
		layout.columns = 3
		grid.collectionViewLayout = layout
		grid.isSelectable = true
		grid.register(GridItem.self, forItemWithIdentifier: GridItem.identifier)
		grid.dataSource = dataSource
		parent.addSubview(grid)
		let window = NSWindow(contentRect: parent.frame, styleMask: [.titled], backing: .buffered, defer: false)
		window.contentView = parent
		grid.reloadData()
		grid.layoutSubtreeIfNeeded()
		grid.selectionIndexPaths = [IndexPath(item: 0, section: 0)]
		return (parent, grid)
	}

	private func keyDown(_ characters: String, keyCode: UInt16, flags: NSEvent.ModifierFlags = []) -> NSEvent {
		NSEvent.keyEvent(
			with: .keyDown, location: .zero, modifierFlags: flags,
			timestamp: ProcessInfo.processInfo.systemUptime, windowNumber: 0, context: nil,
			characters: characters, charactersIgnoringModifiers: characters,
			isARepeat: false, keyCode: keyCode
		)!
	}

	@Test func printableKeysGoUpTheChain() {
		let (parent, grid) = makeChain()
		grid.keyDown(with: keyDown("3", keyCode: 20))
		grid.keyDown(with: keyDown("p", keyCode: 35))
		#expect(parent.received == ["3", "p"])
	}

	@Test func navigationKeysStayWithTheCollectionView() {
		let (parent, grid) = makeChain()
		// Right arrow: the function flag is what marks a navigation key.
		grid.keyDown(with: keyDown("\u{F703}", keyCode: 124, flags: .function))
		// The collection view kept it (whether it could move the selection
		// depends on layout, which an unshown window never performs — the
		// routing is the contract here).
		#expect(parent.received.isEmpty)
	}
}
