//
//  Keymap.swift
//  Alexandria
//
//  The binding table (keybinding round, 2026-09-14).
//

import SwiftUI

/// The chord bound to each command, as menu items read it. Today it reads
/// Command.defaultShortcut; the rebinding round adds an overrides map here
/// and nowhere else.
///
/// Every chord — unmodified ones included — is a menu key equivalent, the
/// native mechanism (Photos shows "." beside Add to Favorites, Mail's Delete
/// key is a menu item). AppKit dispatches an unmodified key to the responder
/// chain FIRST and consults the menu bar only if nothing handled it, so a
/// focused text field keeps its characters and the grid's judgment keys
/// reach the menu (Cocoa Event Handling Guide, "Handling Key Equivalents";
/// WWDC 2010 session 145). Ruled 2026-09-14 after a monitor-based split was
/// built on a misread experiment and struck.
nonisolated struct Keymap: Sendable {
	func shortcut(for command: Command) -> KeyboardShortcut? {
		command.defaultShortcut
	}
}
