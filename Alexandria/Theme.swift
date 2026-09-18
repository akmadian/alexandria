//
//  Theme.swift
//  Alexandria
//
//  Created by ari on 9/11/26.
//

import AppKit
import SwiftUI

nonisolated enum Theme {
	/// The shell's pane metrics: the browser and inspector width bounds
	/// handed to the split view. The stage takes what remains.
	enum Pane {
		static let browserMinimumWidth: CGFloat = 180
		static let browserIdealWidth: CGFloat = 240
		static let browserMaximumWidth: CGFloat = 400
		
		static let inspectorMinimumWidth: CGFloat = 220
		static let inspectorIdealWidth: CGFloat = 280
		static let inspectorMaximumWidth: CGFloat = 420
	}

	/// Grid metrics and the cell round's styling tokens — all of them Ari's
	/// to dial. (Column bounds are behavior, not styling — they live with
	/// the hub's intent.)
	enum Grid {
		static let spacing: CGFloat = 2
		static let inset: CGFloat = 0
		static let minimumCellSide: CGFloat = 40
		// The four-state ring (cell round, 2026-09-18): cursor outranks plain
		// selection. Values are Ari's to dial; hover is deferred with the
		// state that would drive it.
		static let cursorRingWidth: CGFloat = 3
		static let selectedRingWidth: CGFloat = 3
		static let selectedRingOpacity: Double = 0.45
		/// The quiet ground a cell shows before (and beneath) its pixels, so
		/// geometry is fixed from first paint and a missing thumbnail reads
		/// as calm empty space, not a hole (grid.md invariant 3/4).
		static let placeholder = NSColor.quaternaryLabelColor
	}
	
	enum Icons {
		static let flagged = "flag.fill"
		static let unflagged = "flag"
		static let rejected = "flag.slash"
		static let zoomIn = "plus.magnifyingglass"
		static let zoomOut = "minus.magnifyingglass"
	}
}
