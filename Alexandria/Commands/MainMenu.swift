//
//  MainMenu.swift
//  Alexandria
//
//  The menu bar (keybind/targeting round, 2026-09-20): the app's ONE command
//  registry. Every keyboard-reachable command is a real NSMenuItem declared
//  here — chords live nowhere else — because the menu bar is the surface
//  macOS builds on: System Settings remapping matches menu titles, Help
//  search finds menu items, Full Keyboard Access reaches them.
//
//  AppKit, not SwiftUI Commands, by ruling (2026-09-20): enablement and
//  checkmarks are PULL — AppKit asks validateMenuItem at menu-open and
//  key-dispatch time and the answer reads the hub fresh, so a stale or
//  dead menu item is unrepresentable. The SwiftUI layer is push: every
//  item's enablement dependencies must be invalidation-wired, forever
//  (production apps that stay on it need custom plumbing and swizzles —
//  see CodeEdit's CommandsFixes), and it forfeits NSMenu API the roadmap
//  wants (item hiding, alternates, dynamic submenus).
//
//  Unmodified letter chords (p/x/u, 1–5, g/e — Lightroom's vocabulary, so a
//  photographer's fingers arrive trained) are safe beside text fields
//  because AppKit routes bare keys to the first responder BEFORE the menu:
//  a focused field consumes the letter as text, and only unclaimed keys
//  reach these items.
//

import AppKit
import Logging

/// Builder, action target, and validator in one object: the menu IS the
/// registry, so there is no separate command table to drift from it.
@MainActor
final class MainMenu: NSObject, NSMenuItemValidation {

	private let viewState: CatalogViewState
	private let catalog: Catalog
	private let importService: ImportService
	private let log = Logger(label: "menu")
	private var menu: NSMenu?

	init(viewState: CatalogViewState, catalog: Catalog, importService: ImportService) {
		self.viewState = viewState
		self.catalog = catalog
		self.importService = importService
		super.init()
	}

	/// Called once at launch by the app delegate — under the AppKit
	/// lifecycle this object is the menu's sole owner. The NSApp menu-role
	/// assignments live here, not in build(), so building the tree (as the
	/// registry tests do) never mutates the running app's menus (review
	/// finding, 2026-09-20).
	func install() {
		let menu = self.menu ?? build()
		self.menu = menu
		NSApp.mainMenu = menu
		NSApp.windowsMenu = menu.item(withTitle: "Window")?.submenu
		NSApp.helpMenu = menu.item(withTitle: "Help")?.submenu
		NSApp.servicesMenu = menu.item(withTitle: "Alexandria")?.submenu?
			.item(withTitle: "Services")?.submenu
		log.debug("main menu installed")
	}

	// MARK: The declaration

	/// Internal, not private, so the registry tests can walk the built tree
	/// without installing over the test host's menu.
	func build() -> NSMenu {
		let main = NSMenu()
		main.addItem(submenuItem(appMenu()))
		main.addItem(submenuItem(fileMenu()))
		main.addItem(submenuItem(editMenu()))
		main.addItem(submenuItem(assetMenu()))
		main.addItem(submenuItem(viewMenu()))
		main.addItem(submenuItem(windowMenu()))
		main.addItem(submenuItem(helpMenu()))
		return main
	}

	private func appMenu() -> NSMenu {
		let menu = NSMenu(title: "Alexandria")
		menu.addItem(chainItem("About Alexandria", #selector(NSApplication.orderFrontStandardAboutPanel(_:)), modifiers: []))
		menu.addItem(.separator())
		let services = NSMenuItem()
		services.title = "Services"
		services.submenu = NSMenu(title: "Services")
		menu.addItem(services)
		menu.addItem(.separator())
		menu.addItem(chainItem("Hide Alexandria", #selector(NSApplication.hide(_:)), key: "h"))
		menu.addItem(chainItem("Hide Others", #selector(NSApplication.hideOtherApplications(_:)), key: "h", modifiers: [.command, .option]))
		menu.addItem(chainItem("Show All", #selector(NSApplication.unhideAllApplications(_:)), modifiers: []))
		menu.addItem(.separator())
		menu.addItem(chainItem("Quit Alexandria", #selector(NSApplication.terminate(_:)), key: "q"))
		return menu
	}

	private func fileMenu() -> NSMenu {
		let menu = NSMenu(title: "File")
		menu.addItem(item("Import Folder…", #selector(importFolder(_:)), key: "i", modifiers: [.command, .shift]))
		menu.addItem(.separator())
		menu.addItem(chainItem("Close", #selector(NSWindow.performClose(_:)), key: "w"))
		return menu
	}

	/// Standard selectors, nil-targeted: the responder chain hands them to
	/// whatever has focus, so text fields get editing (and the undo round
	/// gets its menu items) without this file knowing who answers.
	private func editMenu() -> NSMenu {
		let menu = NSMenu(title: "Edit")
		menu.addItem(chainItem("Undo", Selector(("undo:")), key: "z"))
		menu.addItem(chainItem("Redo", Selector(("redo:")), key: "z", modifiers: [.command, .shift]))
		menu.addItem(.separator())
		menu.addItem(chainItem("Cut", #selector(NSText.cut(_:)), key: "x"))
		menu.addItem(chainItem("Copy", #selector(NSText.copy(_:)), key: "c"))
		menu.addItem(chainItem("Paste", #selector(NSText.paste(_:)), key: "v"))
		menu.addItem(.separator())
		menu.addItem(chainItem("Select All", #selector(NSText.selectAll(_:)), key: "a"))
		return menu
	}

	private func assetMenu() -> NSMenu {
		let menu = NSMenu(title: "Asset")
		let rating = NSMenu(title: "Set Rating")
		for stars in 1...5 {
			rating.addItem(item("Rate \(stars) Star\(stars == 1 ? "" : "s")",
			                    #selector(setRating(_:)), key: "\(stars)", tag: stars))
		}
		rating.addItem(item("Clear Rating", #selector(setRating(_:)), key: "0"))
		menu.addItem(submenuItem(rating))
		let flag = NSMenu(title: "Set Flag")
		flag.addItem(item("Flag as Pick", #selector(pick(_:)), key: "p"))
		flag.addItem(item("Flag as Reject", #selector(reject(_:)), key: "x"))
		flag.addItem(item("Clear Flag", #selector(unflag(_:)), key: "u"))
		menu.addItem(submenuItem(flag))
		return menu
	}

	private func viewMenu() -> NSMenu {
		let menu = NSMenu(title: "View")
		menu.addItem(item("Show Grid", #selector(showGrid(_:)), key: "g"))
		menu.addItem(item("Show Loupe", #selector(showLoupe(_:)), key: "e"))
		menu.addItem(.separator())
		// "=" is the key a US layout can actually press for zoom-in; "+"
		// would demand shift and never match (caught in review, 2026-09-20).
		menu.addItem(item("Zoom In", #selector(zoomIn(_:)), key: "=", modifiers: .command))
		menu.addItem(item("Zoom Out", #selector(zoomOut(_:)), key: "-", modifiers: .command))
		menu.addItem(.separator())
		// "\" is Lightroom's filter-bar toggle.
		menu.addItem(item("Show Filter Bar", #selector(toggleFilterBar(_:)), key: "\\"))
		menu.addItem(item("Show Inspector", #selector(toggleInspector(_:)), key: "i", modifiers: [.command, .option]))
		return menu
	}

	// The system appends the open-windows list, Help search, and Services
	// content once install() names these menus to NSApp.

	private func windowMenu() -> NSMenu {
		let menu = NSMenu(title: "Window")
		menu.addItem(chainItem("Minimize", #selector(NSWindow.performMiniaturize(_:)), key: "m"))
		menu.addItem(chainItem("Zoom", #selector(NSWindow.performZoom(_:)), modifiers: []))
		return menu
	}

	private func helpMenu() -> NSMenu {
		NSMenu(title: "Help")
	}

	private func submenuItem(_ menu: NSMenu) -> NSMenuItem {
		let item = NSMenuItem()
		item.title = menu.title
		item.submenu = menu
		return item
	}

	/// Ours: targeted at self, so validateMenuItem gates it and the chord
	/// dispatches here. An empty `key` means no chord.
	private func item(
		_ title: String, _ action: Selector, key: String = "",
		modifiers: NSEvent.ModifierFlags = [], tag: Int = 0
	) -> NSMenuItem {
		let item = NSMenuItem(title: title, action: action, keyEquivalent: key)
		item.keyEquivalentModifierMask = modifiers
		item.target = self
		item.tag = tag
		return item
	}

	/// Standard AppKit behavior: nil target, resolved down the responder
	/// chain.
	private func chainItem(
		_ title: String, _ action: Selector, key: String = "",
		modifiers: NSEvent.ModifierFlags = .command
	) -> NSMenuItem {
		let item = NSMenuItem(title: title, action: action, keyEquivalent: key)
		item.keyEquivalentModifierMask = modifiers
		return item
	}

	// MARK: Actions — hub intents and catalog verbs, nothing else

	/// The item's tag is the rating; 0 clears (0 is never a rating — the
	/// schema's CHECK backs this).
	@objc private func setRating(_ sender: NSMenuItem) {
		catalog.applyRating(viewState.judgmentTargets, sender.tag == 0 ? nil : sender.tag)
	}

	@objc private func pick(_ sender: NSMenuItem) {
		catalog.applyFlag(viewState.judgmentTargets, .pick)
	}

	@objc private func reject(_ sender: NSMenuItem) {
		catalog.applyFlag(viewState.judgmentTargets, .reject)
	}

	@objc private func unflag(_ sender: NSMenuItem) {
		catalog.applyFlag(viewState.judgmentTargets, nil)
	}

	@objc private func showGrid(_ sender: NSMenuItem) {
		viewState.setViewMode(.grid)
	}

	@objc private func showLoupe(_ sender: NSMenuItem) {
		viewState.setViewMode(.loupe)
	}

	/// Zoom follows the toolbar's sense: zooming IN means fewer, larger cells.
	@objc private func zoomIn(_ sender: NSMenuItem) {
		viewState.setGridColumns(viewState.gridColumns - 1)
	}

	@objc private func zoomOut(_ sender: NSMenuItem) {
		viewState.setGridColumns(viewState.gridColumns + 1)
	}

	@objc private func toggleFilterBar(_ sender: NSMenuItem) {
		viewState.setFilterBarPresented(!viewState.filterBarPresented)
	}

	@objc private func toggleInspector(_ sender: NSMenuItem) {
		viewState.setInspectorPresented(!viewState.inspectorPresented)
	}

	@objc private func importFolder(_ sender: NSMenuItem) {
		let panel = NSOpenPanel()
		panel.canChooseFiles = false
		panel.canChooseDirectories = true
		panel.allowsMultipleSelection = false
		panel.prompt = "Import Folder"
		guard panel.runModal() == .OK, let url = panel.url else { return }
		let importService = importService
		let log = log
		Task {
			do {
				try await importService.startImport(of: url)
			} catch ImportError.importAlreadyRunning {
				// The refusal must reach the user — a menu click that
				// silently does nothing is the silent-failure sin in
				// miniature.
				let alert = NSAlert()
				alert.alertStyle = .informational
				alert.messageText = "An import is already running"
				alert.informativeText = "Wait for it to finish before starting another."
				alert.runModal()
			} catch {
				log.error("Import failed to start", metadata: [
					"source": "\(url.path())",
					"error": "\(error)",
				])
			}
		}
	}

	// MARK: Validation — the pull

	func validateMenuItem(_ menuItem: NSMenuItem) -> Bool {
		switch menuItem.action {
		case #selector(setRating(_:)), #selector(pick(_:)), #selector(reject(_:)), #selector(unflag(_:)):
			// The judgment precondition: something to judge. The O(1) gate,
			// not judgmentTargets — validation runs per item per menu-open.
			return viewState.hasJudgmentTargets
		case #selector(showGrid(_:)):
			menuItem.state = viewState.viewMode == .grid ? .on : .off
			return true
		case #selector(showLoupe(_:)):
			menuItem.state = viewState.viewMode == .loupe ? .on : .off
			return true
		case #selector(zoomIn(_:)):
			// Grid density is meaningless in the loupe, which shows one asset.
			return viewState.viewMode == .grid
				&& viewState.gridColumns > CatalogViewState.gridColumnRange.lowerBound
		case #selector(zoomOut(_:)):
			return viewState.viewMode == .grid
				&& viewState.gridColumns < CatalogViewState.gridColumnRange.upperBound
		case #selector(toggleFilterBar(_:)):
			menuItem.state = viewState.filterBarPresented ? .on : .off
			return true
		case #selector(toggleInspector(_:)):
			menuItem.state = viewState.inspectorPresented ? .on : .off
			return true
		case #selector(importFolder(_:)):
			// Serialized imports (ruled 2026-09-18): the refusal the action
			// would alert about grays the item instead. The alert stays as
			// the backstop for the race where a run starts mid-validation.
			return !importService.isImporting
		default:
			return true
		}
	}
}

