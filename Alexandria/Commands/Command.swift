//
//  Command.swift
//  Alexandria
//
//  The command registry (keybinding round, 2026-09-14).
//

import SwiftUI

/// Every keyboard- or menu-reachable action: a stable identity, the context
/// it is meaningful in, and the chord it ships with. Behavior lives in
/// CommandRunner and the menus are written out in AppCommands; this enum
/// exists so rebinding (a later round) has an identity to key its overrides
/// on, and so a menu item reads its chord through Keymap rather than
/// hardcoding it.
nonisolated enum Command: String, CaseIterable {
	case rate1, rate2, rate3, rate4, rate5, unrate
	case pick, reject, unflag
	case showGrid, showLoupe
	case zoomIn, zoomOut

	/// Where a command is meaningful — app posture, answered by the hub. A
	/// closed enum (the Qt/GTK shape), not a predicate language: three
	/// contexts is the whole vocabulary today; compare mode adds a case here
	/// and a line in CatalogViewState.satisfies.
	///
	/// Nested, not top-level: a bare `Context` shadows the `Context`
	/// typealias every NSViewRepresentable's methods are written against
	/// (GridRepresentable) and breaks them at a distance.
	enum Context {
		case any, grid, loupe
	}

	/// Exhaustive, like every other column of the registry: a new command
	/// states its context rather than inheriting `.any` from a `default`.
	var context: Context {
		switch self {
		case .rate1, .rate2, .rate3, .rate4, .rate5, .unrate, .pick, .reject, .unflag:
			.any
		case .showGrid, .showLoupe:
			.any
		// Grid density is meaningless in the loupe, which shows one asset.
		case .zoomIn, .zoomOut:
			.grid
		}
	}

	/// The chord the command ships with. Optional because a rebinding round
	/// must be able to leave a command unbound; every command carries one
	/// today. Letters follow Lightroom's vocabulary (p/x/u, g/e), so a
	/// photographer's fingers arrive already trained.
	// TODO: f - full screen selected asset
	// TODO: l - lights out state cycle
	// TODO: cmd+[ cmd+] rotate selected asset
	var defaultShortcut: KeyboardShortcut? {
		switch self {
		case .rate1: KeyboardShortcut("1", modifiers: [])
		case .rate2: KeyboardShortcut("2", modifiers: [])
		case .rate3: KeyboardShortcut("3", modifiers: [])
		case .rate4: KeyboardShortcut("4", modifiers: [])
		case .rate5: KeyboardShortcut("5", modifiers: [])
		case .unrate: KeyboardShortcut("0", modifiers: [])
		case .pick: KeyboardShortcut("p", modifiers: [])
		case .reject: KeyboardShortcut("x", modifiers: [])
		case .unflag: KeyboardShortcut("u", modifiers: [])
		case .showGrid: KeyboardShortcut("g", modifiers: [])
		case .showLoupe: KeyboardShortcut("e", modifiers: [])
		case .zoomIn: KeyboardShortcut("+", modifiers: .command)
		case .zoomOut: KeyboardShortcut("-", modifiers: .command)
		}
	}
}

extension CatalogViewState {
	/// The hub's answer to a command's context question. The one legitimate
	/// place the posture is compared against a context — canPerform asks it,
	/// nothing else does.
	func satisfies(_ context: Command.Context) -> Bool {
		switch context {
		case .any: true
		case .grid: viewMode == .grid
		case .loupe: viewMode == .loupe
		}
	}
}
