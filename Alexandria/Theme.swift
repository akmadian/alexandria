//
//  Theme.swift
//  Alexandria
//
//  Created by ari on 9/11/26.
//

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

	/// Grid metrics, deliberately minimal this round: spacing and clamps
	/// only. Cell styling belongs to the cell design round. (Column bounds
	/// are behavior, not styling — they live with the hub's intent.)
	enum Grid {
		static let spacing: CGFloat = 8
		static let inset: CGFloat = 12
		static let minimumCellSide: CGFloat = 40
		static let selectionRingWidth: CGFloat = 3
	}
}
