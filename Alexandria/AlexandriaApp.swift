//
//  AlexandriaApp.swift
//  Alexandria
//
//  Created by ari on 9/9/26.
//

import SwiftUI
import Foundation
import GRDB
import GRDBQuery
import Logging

@main
struct AlexandriaApp: App {
	static let devCatalogDir = URL.applicationSupportDirectory
		.appendingPathComponent("Alexandria")
		.appendingPathComponent("devcat")

	// Dev scaffold: one hardcoded catalog until the open-catalog UI round.
	let catalog: Catalog
	let importService: ImportService
	// One hub per open catalog. "Exactly one" is a fact about this
	// composition root, never about the design.
	let viewState: CatalogViewState
	// The command system: one table, one door (menu items carry every chord).
	// The runner is deliberately NOT in the environment — views call verbs
	// and intents directly; it exists for the menus.
	let keymap = Keymap()
	let runner: CommandRunner
	// Keeps MountTable event-fresh for the app's lifetime; injected into the
	// environment for the browser's volume headers.
	let volumeMonitor: VolumeMonitor

	init() {
		Log.bootstrap()
		volumeMonitor = VolumeMonitor()
		catalog = try! Catalog.open(at: Self.devCatalogDir)
		importService = ImportService(catalog: catalog)
		viewState = CatalogViewState(catalog: catalog)
		runner = CommandRunner(catalog: catalog, viewState: viewState)
	}

    var body: some Scene {
		Window("Alexandria", id: "main") {
			ShellView()
				.environment(\.catalog, catalog)
				.databaseContext(.readOnly { catalog.reader })
				.environment(viewState)
				.environment(volumeMonitor)
				.environment(importService)
        }
		.commands {
			CommandGroup(after: .newItem) {
				Button("Import Folder") {
					let panel = NSOpenPanel()
					panel.canChooseFiles = false
					panel.canChooseDirectories = true
					panel.allowsMultipleSelection = false
					panel.prompt = "Import Folder"
					
					if panel.runModal() == .OK, let url = panel.url {
						print("CMD: Import Folder - Selected \(url)")
						Task {
							do {
								try await importService.startImport(of: url)
							} catch ImportError.importAlreadyRunning {
								// The refusal must reach the user — a menu
								// click that silently does nothing is the
								// silent-failure sin in miniature.
								let alert = NSAlert()
								alert.alertStyle = .informational
								alert.messageText = "An import is already running"
								alert.informativeText = "Wait for it to finish before starting another."
								alert.runModal()
							} catch {
								Logger(label: "app").error("Import failed to start", metadata: [
									"source": "\(url.path())",
									"error": "\(error)",
								])
							}
						}
					}
				}
				.keyboardShortcut("I", modifiers: [.command, .shift])
			}
			AppCommands(runner: runner, keymap: keymap)
		}
    }
}

extension EnvironmentValues {
	/// The open catalog. Defaults to an in-memory catalog so previews and
	/// uninjected views get a working, empty database.
	@Entry var catalog = try! Catalog(DatabaseQueue())
}
