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
    }
}

extension EnvironmentValues {
	/// The open catalog. Defaults to an in-memory catalog so previews and
	/// uninjected views get a working, empty database.
	@Entry var catalog = try! Catalog(DatabaseQueue())
}
