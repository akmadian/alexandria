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
		// WITHIN CELLS
		static let cellBackground: Color = Color(white: 0.57)
		static let cellSelectedBackground: Color = Color(white: 0.79)
		static let cellCursorBackground: Color = Color(white: 0.92)
		
		static let cellHeaderColor: Color = Color(white: 0.63)
		
		static let cellContentPadding: CGFloat = 6
		
		// BETWEEN CELLS
		static let interCellSpacing: CGFloat = 2
		static let inset: CGFloat = 2
		// The four-state styling (cell round, 2026-09-18; recomposed
		// 2026-09-20): prominence reads on the cell surface + content frame.
		// Cursor outranks plain selection. Values are Ari's to dial; hover
		// is deferred with the state that would drive it.
		static let cursorRingWidth: CGFloat = 1
		static let selectedRingWidth: CGFloat = 1
		static let selectedRingOpacity: Double = 0.45
		static let surfaceIdle = Color.clear
		static let surfaceSelected = Color(nsColor: .unemphasizedSelectedContentBackgroundColor)
		static let surfaceCursor = Color(nsColor: .unemphasizedSelectedContentBackgroundColor)
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
