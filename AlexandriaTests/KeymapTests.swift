//
//  KeymapTests.swift
//  AlexandriaTests
//
//  The binding table's contract (keybinding round, 2026-09-14): every chord
//  is a menu key equivalent, and the registry's one invariant is that no two
//  commands claim the same one.
//

import SwiftUI
import Testing
@testable import Alexandria

struct KeymapTests {

	private let keymap = Keymap()

	/// Which chord each command ships with is the registry's business (and
	/// gets tuned); that every command ships with one is the contract.
	@Test func everyCommandShipsWithAChord() {
		for command in Command.allCases {
			#expect(keymap.shortcut(for: command) != nil, "\(command) is unbound")
		}
	}

	/// AppKit hands a key equivalent to exactly one menu item, so two
	/// commands sharing a chord means one of them is unreachable.
	@Test func chordsAreUniqueAcrossTheRegistry() {
		var owners: [KeyboardShortcut: [Command]] = [:]
		for command in Command.allCases {
			guard let shortcut = keymap.shortcut(for: command) else { continue }
			owners[shortcut, default: []].append(command)
		}
		for (shortcut, commands) in owners {
			#expect(commands.count == 1, "\(shortcut.key.character) is claimed by \(commands)")
		}
	}
}
