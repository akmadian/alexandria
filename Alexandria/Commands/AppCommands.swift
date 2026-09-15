//
//  AppCommands.swift
//  Alexandria
//
//  The app's menus (keybinding round, 2026-09-14).
//

import SwiftUI

/// The menus, written out. Each item names its command so the chord comes
/// from Keymap (rebinding reaches the menu) and enablement from the runner.
/// Every item shows its real chord, unmodified ones included — see Keymap
/// for why that is safe next to text fields.
struct AppCommands: Commands {
	let runner: CommandRunner
	let keymap: Keymap

	// TODO: Contextual icons - LrC shows a check by the selected image's rating
	var body: some Commands {
		CommandMenu("Asset") {
			Section("Judgments") {
				Menu("Set Rating", systemImage: "star") {
					item("Rate 1 Star", .rate1)
					item("Rate 2 Stars", .rate2)
					item("Rate 3 Stars", .rate3)
					item("Rate 4 Stars", .rate4)
					item("Rate 5 Stars", .rate5)
					item("Clear Rating", .unrate)
				}
				Menu("Set Flag", systemImage: "flag") {
					item("Flag as Pick", .pick, "flag")
					item("Flag as Reject", .reject, "flag.slash")
					item("Clear Flag", .unflag)
				}
			}
			Divider()
		}
		// `after: .sidebar` joins the SYSTEM View menu rather than minting a
		// second one beside it.
		CommandGroup(after: .sidebar) {
			item("Show Grid", .showGrid, "square.grid.2x2")
			item("Show Loupe", .showLoupe, "loupe")
			Divider()
			item("Zoom In", .zoomIn, "plus.magnifyingglass")
			item("Zoom Out", .zoomOut, "minus.magnifyingglass")
		}
	}

	@ViewBuilder
	private func item(
		_ title: LocalizedStringKey,
		_ command: Command,
		_ icon: String? = nil
	) -> some View {
		if let icon {
			Button(title, systemImage: icon) { runner.perform(command) }
				.keyboardShortcut(keymap.shortcut(for: command))
				.disabled(!runner.canPerform(command))
		} else {
			Button(title) { runner.perform(command) }
				.keyboardShortcut(keymap.shortcut(for: command))
				.disabled(!runner.canPerform(command))
		}
	}
}
