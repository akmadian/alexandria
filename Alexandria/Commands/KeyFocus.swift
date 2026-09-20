//
//  KeyFocus.swift
//  Alexandria
//
//  Focus ownership (keybind round, 2026-09-20): the stage's active renderer
//  claims key focus by default, so keyboard-only workflows never need an
//  arming click — the bug this round was opened on.
//

import AppKit
import Logging

private nonisolated let log = Logger(label: "focus")

extension NSEvent {
	/// The navigation-key discriminator both stage key splits share (grid
	/// and loupe): function-flagged keys — arrows, home/end, page up/down —
	/// are the surface's own; everything else goes up the chain so the menu
	/// gets its native turn.
	var isNavigationKey: Bool {
		modifierFlags.contains(.function)
	}
}

extension NSView {
	/// Claim first responder, but only from limbo — when the window itself
	/// holds key focus, AppKit's "nobody does". Never steals from a live
	/// responder (a sidebar the user clicked, a text field mid-edit), so
	/// full-screen and split-view rebuilds that re-attach views can't yank
	/// focus from one. Deferred a runloop turn: at viewDidMoveToWindow time
	/// inside an NSViewRepresentable attach, the window's responder plumbing
	/// isn't settled yet (documented ordering race).
	func claimKeyFocusFromLimbo() {
		DispatchQueue.main.async { [weak self] in
			guard let self, let window = self.window else { return }
			if window.firstResponder === window {
				window.makeFirstResponder(self)
				log.debug("key focus claimed from limbo", metadata: [
					"claimant": "\(type(of: self))",
				])
			} else {
				// The likely face of any future "keys are dead" report:
				// a live responder held focus and the claim stood down.
				log.debug("key focus claim declined — live responder", metadata: [
					"claimant": "\(type(of: self))",
					"holder": "\(window.firstResponder.map { String(describing: type(of: $0)) } ?? "nil")",
				])
			}
		}
	}
}
