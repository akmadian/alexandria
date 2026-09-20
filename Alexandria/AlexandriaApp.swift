//
//  AlexandriaApp.swift
//  Alexandria
//
//  The AppKit-lifecycle entry point (keybind round, 2026-09-20). The shell —
//  application, window, menu — is AppKit's, matching the round's spine
//  ruling: under the SwiftUI lifecycle the framework co-owns NSApp.mainMenu
//  and reasserts its own menu on scene updates, so a hand-built menu can
//  never stick (measured here; corroborated in the wild). Everything inside
//  the window stays SwiftUI: the hosting controller bridges our toolbars and
//  navigation title onto the real window (sceneBridgingOptions defaults to
//  .all for a contentViewController, macOS 14+).
//

import AppKit
import SwiftUI
import Foundation
import GRDB
import GRDBQuery
import QuickLookUI

@main
enum AlexandriaMain {
	static func main() {
		let app = NSApplication.shared
		let delegate = AppDelegate()
		// NSApplication holds its delegate weakly; the extended lifetime is
		// the strong reference that keeps it alive for the whole run.
		app.delegate = delegate
		withExtendedLifetime(delegate) {
			app.run()
		}
	}
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
	static let devCatalogDir = URL.applicationSupportDirectory
		.appendingPathComponent("Alexandria")
		.appendingPathComponent("devcat")

	// Dev scaffold: one hardcoded catalog until the open-catalog UI round.
	let catalog: Catalog
	let importService: ImportService
	// One hub per open catalog. "Exactly one" is a fact about this
	// composition root, never about the design.
	let viewState: CatalogViewState
	// The menu bar IS the command registry (keybind round, 2026-09-20):
	// every chord is declared in MainMenu and nowhere else.
	let mainMenu: MainMenu
	// Keeps MountTable event-fresh for the app's lifetime; injected into the
	// environment for the browser's volume headers.
	let volumeMonitor: VolumeMonitor

	private var window: NSWindow?

	override init() {
		Log.bootstrap()
		volumeMonitor = VolumeMonitor()
		catalog = try! Catalog.open(at: Self.devCatalogDir)
		importService = ImportService(catalog: catalog)
		viewState = CatalogViewState(catalog: catalog)
		mainMenu = MainMenu(viewState: viewState, catalog: catalog, importService: importService)
		super.init()
	}

	func applicationDidFinishLaunching(_ notification: Notification) {
		// Sole owner: nothing else in the process writes NSApp.mainMenu.
		mainMenu.install()

		let root = ShellView()
			.environment(\.catalog, catalog)
			.databaseContext(.readOnly { catalog.reader })
			.environment(viewState)
			.environment(volumeMonitor)
			.environment(importService)
		let hosting = NSHostingController(rootView: root)
		// Only the min-size constraint: the default set includes an
		// intrinsic-size constraint that stops hosted SwiftUI from filling a
		// resizable window (the documented NSHostingView sizing trap).
		hosting.sizingOptions = [.minSize]

		// .fullSizeContentView is what lets NavigationSplitView's sidebar
		// run the full window height, as it does under a SwiftUI Window.
		let window = MainWindow(
			contentRect: NSRect(x: 0, y: 0, width: 1280, height: 800),
			styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
			backing: .buffered, defer: false)
		window.contentViewController = hosting
		// A programmatic NSWindow defaults to release-on-close — a manual
		// release ARC can't see, dangling our strong reference the first
		// time a close doesn't also terminate (review finding, 2026-09-20).
		window.isReleasedWhenClosed = false
		window.title = "Alexandria"
		if !window.setFrameUsingName("Main") {
			window.center()
		}
		window.setFrameAutosaveName("Main")
		window.makeKeyAndOrderFront(nil)
		self.window = window
	}

	func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
		true
	}

	func applicationSupportsSecureRestorableState(_ app: NSApplication) -> Bool {
		true
	}
}

/// The main window, with one focus policy: Quick Look's internal views grab
/// key focus when preview content loads (measured 2026-09-20; QLPreviewView
/// has no public knob to decline), which yanks the keyboard from the loupe's
/// key host mid-navigation. The loupe surface's key owner is its host by the
/// stage-surface contract; the placeholder preview is display-only. This
/// veto dies with QLPreviewView at the loupe round.
final class MainWindow: NSWindow {
	override func makeFirstResponder(_ responder: NSResponder?) -> Bool {
		if let view = responder as? NSView,
		   sequence(first: view, next: \.superview).contains(where: { $0 is QLPreviewView }) {
			return false
		}
		return super.makeFirstResponder(responder)
	}
}

extension EnvironmentValues {
	/// The open catalog. Defaults to an in-memory catalog so previews and
	/// uninjected views get a working, empty database.
	@Entry var catalog = try! Catalog(DatabaseQueue())
}
