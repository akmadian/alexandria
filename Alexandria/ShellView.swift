//
//  Shell.swift
//  Alexandria
//
//  Created by ari on 9/11/26.
//

import SwiftUI

struct ShellView: View {
	@State private var inspectorPresented = true
	
	var body: some View {
		NavigationSplitView {
			BrowserView()
		} detail: {
			StageView()
		}
		.inspector(isPresented: $inspectorPresented) {
			InspectorView()
		}
	}
}
