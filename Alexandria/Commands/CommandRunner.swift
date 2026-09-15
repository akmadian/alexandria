//
//  CommandRunner.swift
//  Alexandria
//
//  Command dispatch (keybinding round, 2026-09-14).
//

import Foundation
import SwiftUI
import Logging

/// Maps a command to the hub intents and catalog verbs that already exist.
/// The runner invents no behavior and holds no state of its own: it is the
/// door menu items (and so their key equivalents) come through, and every
/// other surface keeps calling the verbs and intents directly.
///
/// canPerform is the ONE place context and preconditions are evaluated; the
/// menu asks it to enable or disable an item.
@MainActor @Observable
final class CommandRunner {

	@ObservationIgnored private let catalog: Catalog
	// The runner holds no state of its own; what a menu item's enablement
	// tracks is the HUB's observable posture, read through canPerform.
	@ObservationIgnored private let viewState: CatalogViewState
	@ObservationIgnored private let log = Logger(label: "commands")

	init(catalog: Catalog, viewState: CatalogViewState) {
		self.catalog = catalog
		self.viewState = viewState
	}

	/// Context first, then the command's own precondition. Exhaustive, so a
	/// new command states what it needs instead of inheriting "always".
	func canPerform(_ command: Command) -> Bool {
		guard viewState.satisfies(command.context) else { return false }
		switch command {
		case .rate1, .rate2, .rate3, .rate4, .rate5, .unrate, .pick, .reject, .unflag:
			// The judgment precondition: something to judge.
			return !viewState.judgmentTargets.isEmpty
		case .showGrid, .showLoupe:
			// A renderer swap changes no question and needs no working set.
			return true
		case .zoomIn:
			return viewState.gridColumns > CatalogViewState.gridColumnRange.lowerBound
		case .zoomOut:
			return viewState.gridColumns < CatalogViewState.gridColumnRange.upperBound
		}
	}

	/// A flat switch, one line per case. Gating lives in canPerform, which
	/// the menu consults before it gets here; the intents and verbs behind
	/// these lines are total anyway (an empty target set judges nothing, and
	/// setGridColumns clamps), so an ungated call is a no-op, never damage.
	///
	/// Zoom follows the toolbar's sense (ShellView): zooming IN means fewer,
	/// larger cells.
	func perform(_ command: Command) {
		switch command {
		case .rate1: rate(1)
		case .rate2: rate(2)
		case .rate3: rate(3)
		case .rate4: rate(4)
		case .rate5: rate(5)
		case .unrate: rate(nil)
		case .pick: flag(.pick)
		case .reject: flag(.reject)
		case .unflag: flag(nil)
		case .showGrid: viewState.setViewMode(.grid)
		case .showLoupe: viewState.setViewMode(.loupe)
		case .zoomIn: viewState.setGridColumns(viewState.gridColumns - 1)
		case .zoomOut: viewState.setGridColumns(viewState.gridColumns + 1)
		}
	}


	// Both verbs return the prior values for undo; undo is a later chunk, so
	// they are discarded here rather than half-kept.

	private func rate(_ rating: Int?) {
		let targets = viewState.judgmentTargets
		Task {
			do {
				_ = try await catalog.setRating(targets, to: rating)
			} catch {
				log.error("rating failed", metadata: [
					"assets": "\(targets.count)",
					"error": "\(error)",
				])
			}
		}
	}

	private func flag(_ flag: Asset.Flag?) {
		let targets = viewState.judgmentTargets
		Task {
			do {
				_ = try await catalog.setFlag(targets, to: flag)
			} catch {
				log.error("flagging failed", metadata: [
					"assets": "\(targets.count)",
					"error": "\(error)",
				])
			}
		}
	}
}
