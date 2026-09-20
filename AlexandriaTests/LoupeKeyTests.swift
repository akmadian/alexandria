//
//  LoupeKeyTests.swift
//  AlexandriaTests
//
//  The loupe's key surface (keybind round, 2026-09-20): arrows step the
//  cursor, printable keys go up the chain so the menu gets its native turn —
//  the same routing split the grid pins in GridCollectionViewTests.
//

import AppKit
import QuickLookUI
import Testing
@testable import Alexandria

@MainActor
struct LoupeKeyTests {

	private final class RecordingParent: NSView {
		var received: [String] = []
		override func keyDown(with event: NSEvent) {
			received.append(event.charactersIgnoringModifiers ?? "")
		}
	}

	private func keyDown(_ characters: String, keyCode: UInt16, flags: NSEvent.ModifierFlags = []) -> NSEvent {
		NSEvent.keyEvent(
			with: .keyDown, location: .zero, modifierFlags: flags,
			timestamp: ProcessInfo.processInfo.systemUptime, windowNumber: 0, context: nil,
			characters: characters, charactersIgnoringModifiers: characters,
			isARepeat: false, keyCode: keyCode
		)!
	}

	@Test func arrowSelectorsStepTheCursor() {
		let view = LoupeKeyView()
		var steps: [Int] = []
		view.onStep = { steps.append($0) }
		view.moveLeft(nil)
		view.moveRight(nil)
		view.moveUp(nil)
		view.moveDown(nil)
		#expect(steps == [-1, 1, -1, 1])
	}

	/// The main window's Quick Look veto (keybind round, 2026-09-20): views
	/// inside a QLPreviewView never take key focus — the loupe's key host
	/// owns the keyboard, the placeholder preview is display-only.
	@Test func quickLookViewsNeverTakeKeyFocus() throws {
		let window = MainWindow(
			contentRect: NSRect(x: 0, y: 0, width: 400, height: 400),
			styleMask: [.titled], backing: .buffered, defer: false)
		let parent = NSView(frame: NSRect(x: 0, y: 0, width: 400, height: 400))
		window.contentView = parent

		let preview = QLPreviewView(frame: parent.bounds, style: .normal)!
		parent.addSubview(preview)
		let inner = NSTextView(frame: parent.bounds)
		preview.addSubview(inner)
		#expect(window.makeFirstResponder(inner) == false)

		// A responder OUTSIDE the preview is untouched by the veto.
		let outside = NSTextView(frame: parent.bounds)
		parent.addSubview(outside)
		#expect(window.makeFirstResponder(outside) == true)
	}

	@Test func printableKeysGoUpTheChain() {
		let parent = RecordingParent(frame: NSRect(x: 0, y: 0, width: 100, height: 100))
		let view = LoupeKeyView(frame: parent.bounds)
		parent.addSubview(view)
		var steps: [Int] = []
		view.onStep = { steps.append($0) }

		view.keyDown(with: keyDown("p", keyCode: 35))
		#expect(parent.received == ["p"])
		#expect(steps.isEmpty)

		// Left arrow, function-flagged: interpreted here, not forwarded.
		view.keyDown(with: keyDown("\u{F702}", keyCode: 123, flags: .function))
		#expect(parent.received == ["p"])
		#expect(steps == [-1])
	}
}
