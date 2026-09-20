//
//  MainMenuTests.swift
//  AlexandriaTests
//
//  The registry invariants (keybind round, 2026-09-20). The menu IS the
//  registry, so the invariants walk the built NSMenu tree: chords are
//  unambiguous, and enablement/checkmarks answer the hub's live posture
//  through pull validation — chosen because pull cannot latch (the SwiftUI
//  bridge's push was measured latching a starts-disabled judgment key dead).
//

import AppKit
import Testing
@testable import Alexandria

@MainActor
struct MainMenuTests {

	private func makeMenu() async throws -> (MainMenu, NSMenu, CatalogViewState, ImportContext) {
		let context = try await ImportContext.make()
		let hub = CatalogViewState(catalog: context.catalog)
		let main = MainMenu(
			viewState: hub, catalog: context.catalog,
			importService: ImportService(catalog: context.catalog))
		return (main, main.build(), hub, context)
	}

	private func items(in menu: NSMenu) -> [NSMenuItem] {
		menu.items.flatMap { item in
			[item] + (item.submenu.map(items(in:)) ?? [])
		}
	}

	private func item(_ title: String, in menu: NSMenu) throws -> NSMenuItem {
		try #require(items(in: menu).first { $0.title == title })
	}

	/// No two items declare the same chord: AppKit's key-equivalent search is
	/// first-match-wins and a disabled match swallows the key (measured
	/// 2026-09-20), so a duplicate is never "the other one fires" — it is a
	/// dead or misrouted key.
	@Test func chordsAreUnambiguous() async throws {
		let (_, menu, _, _) = try await makeMenu()
		let chords = items(in: menu)
			.filter { !$0.keyEquivalent.isEmpty }
			.map { "\($0.keyEquivalentModifierMask.rawValue)+\($0.keyEquivalent)" }
		#expect(chords.count == Set(chords).count)
		#expect(!chords.isEmpty)
	}

	/// The judgment precondition, live through validation: disabled with
	/// nothing to judge, enabled the moment the working set delivers a
	/// cursor — the exact wake-up the SwiftUI bridge failed.
	@Test func judgmentItemsFollowTheTargets() async throws {
		let (main, menu, hub, context) = try await makeMenu()
		let pickItem = try item("Flag as Pick", in: menu)
		#expect(main.validateMenuItem(pickItem) == false)

		try await context.record([context.prepared("/Volumes/Test/Shoot/a.jpg")])
		try await context.form()
		try await eventually("delivery") { hub.workingSet.count == 1 }
		#expect(main.validateMenuItem(pickItem) == true)
	}

	/// Mode items carry a fresh checkmark on every validation.
	@Test func modeItemsCarryTheCheckmark() async throws {
		let (main, menu, hub, _) = try await makeMenu()
		let gridItem = try item("Show Grid", in: menu)
		let loupeItem = try item("Show Loupe", in: menu)

		#expect(main.validateMenuItem(gridItem))
		#expect(gridItem.state == .on)
		#expect(main.validateMenuItem(loupeItem))
		#expect(loupeItem.state == .off)

		hub.setViewMode(.loupe)
		_ = main.validateMenuItem(gridItem)
		_ = main.validateMenuItem(loupeItem)
		#expect(gridItem.state == .off)
		#expect(loupeItem.state == .on)
	}

	/// Zoom gates on the column bounds and on the grid being the mode —
	/// grid density is meaningless in the loupe.
	@Test func zoomGatesOnColumnsAndMode() async throws {
		let (main, menu, hub, _) = try await makeMenu()
		let zoomIn = try item("Zoom In", in: menu)
		let zoomOut = try item("Zoom Out", in: menu)

		hub.setGridColumns(CatalogViewState.gridColumnRange.lowerBound)
		#expect(main.validateMenuItem(zoomIn) == false)
		#expect(main.validateMenuItem(zoomOut) == true)

		hub.setGridColumns(CatalogViewState.gridColumnRange.upperBound)
		#expect(main.validateMenuItem(zoomIn) == true)
		#expect(main.validateMenuItem(zoomOut) == false)

		hub.setViewMode(.loupe)
		#expect(main.validateMenuItem(zoomIn) == false)
		#expect(main.validateMenuItem(zoomOut) == false)
	}

	/// Chrome toggles read the hub and answer with a checkmark, not
	/// enablement.
	@Test func chromeTogglesMirrorTheHub() async throws {
		let (main, menu, hub, _) = try await makeMenu()
		let filterItem = try item("Show Filter Bar", in: menu)
		let inspectorItem = try item("Show Inspector", in: menu)

		_ = main.validateMenuItem(filterItem)
		_ = main.validateMenuItem(inspectorItem)
		#expect(filterItem.state == .off)
		#expect(inspectorItem.state == .on)

		hub.setFilterBarPresented(true)
		hub.setInspectorPresented(false)
		_ = main.validateMenuItem(filterItem)
		_ = main.validateMenuItem(inspectorItem)
		#expect(filterItem.state == .on)
		#expect(inspectorItem.state == .off)
	}
}
