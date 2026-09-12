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
}
