//
//  ImportStatusSlot.swift
//  Alexandria
//
//  The invalidation fence (import status round, 2026-09-18): the one view
//  that reads a live ImportRun's counters, so per-batch ticks (~5,000
//  across a 40k import) re-evaluate this row alone, never the sidebar's
//  body. FolderItem below stays value-fed and previewable.
//

import SwiftUI

struct ImportStatusSlot: View {
	let name: String
	let run: ImportRun
	let onDismiss: () -> Void

	var body: some View {
		FolderItem(
			name: name,
			// unfinished: false is immaterial — the run outranks the flag,
			// and .current is the one place that precedence lives.
			status: ImportStatus.current(run: run, unfinished: false),
			onDismiss: onDismiss,
			onCancel: { run.cancel() }
		)
	}
}
