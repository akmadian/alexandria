//
//  AlexandriaApp.swift
//  Alexandria
//
//  Created by ari on 9/9/26.
//

import SwiftUI
import Foundation
import GRDB
import Logging

@main
struct AlexandriaApp: App {
	static let devCatalogDir = URL.applicationSupportDirectory
		.appendingPathComponent("Alexandria")
		.appendingPathComponent("devcat")

	// Dev scaffold: one hardcoded catalog until the open-catalog UI round.
	let catalog: Catalog
	let importService: ImportService

	init() {
		Log.bootstrap()
		catalog = try! Catalog.open(at: Self.devCatalogDir)
		importService = ImportService(catalog: catalog)
	}

    var body: some Scene {
		Window("Alexandria", id: "main") {
			ShellView().environment(\.catalog, catalog)
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
		}
    }
}

extension EnvironmentValues {
	/// The open catalog. Defaults to an in-memory catalog so previews and
	/// uninjected views get a working, empty database.
	@Entry var catalog = try! Catalog(DatabaseQueue())
}
