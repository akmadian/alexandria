//
//  AlexandriaApp.swift
//  Alexandria
//
//  Created by ari on 9/9/26.
//

import SwiftUI
import Foundation
import GRDB

@main
struct AlexandriaApp: App {

	static let devCatalogDir = URL.applicationSupportDirectory
		.appendingPathComponent("Alexandria")
		.appendingPathComponent("devcat")

	// Dev scaffold: one hardcoded catalog until the open-catalog UI round.
	let catalog = try! Catalog.open(at: Self.devCatalogDir)

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(\.catalog, catalog)
        }
		.commands {
			CommandGroup(after: .newItem) {
				Button("Import Folder") {
					print("CMD: Import Folder")
					let panel = NSOpenPanel()
					panel.canChooseFiles = false
					panel.canChooseDirectories = true
					panel.allowsMultipleSelection = false
					panel.prompt = "Import Folder"
					
					if panel.runModal() == .OK, let url = panel.url {
						print("CMD: Import Folder - Selected \(url)")
						let run = ImportRun(folderUrl: url, catalog: catalog)
						run.start()
					}
				}
				.keyboardShortcut("I", modifiers: [.command])
			}
		}
    }
}

extension EnvironmentValues {
	/// The open catalog. Defaults to an in-memory catalog so previews and
	/// uninjected views get a working, empty database.
	@Entry var catalog = try! Catalog(DatabaseQueue())
}
