//
//  FolderItem.swift
//  Alexandria
//
//  The folder row (import status round, 2026-09-18): VolumeHeader's
//  sibling and the same discipline — value-fed, catalog- and
//  service-blind, previewable in every state. The trailing slot is the
//  import status well: one home for the whole story (ring while running,
//  lingering check/alert when done, resume badge when the DB says an
//  import never finished), with the popover as the universal detail
//  anchor. Like VolumeHeader, this is Ari's styling playground.
//

import SwiftUI

struct FolderItem: View {
	let name: String
	let status: ImportStatus?
	var onDismiss: () -> Void = {}
	var onCancel: () -> Void = {}

	@State private var showingDetail = false

	var body: some View {
		HStack {
			Label(name, systemImage: "folder")
			Spacer(minLength: 0)
			statusControl
		}
	}

	@ViewBuilder private var statusControl: some View {
		switch status {
		case nil:
			EmptyView()
		case .walking:
			ProgressView()
				.controlSize(.mini)
				.help("Walking directory…")
				.accessibilityLabel("Import in progress, walking directory")
		case .finishing:
			ProgressView()
				.controlSize(.mini)
				.help("Finishing up…")
				.accessibilityLabel("Import in progress, finishing up")
		case .progress(let ready, let cataloged, let total):
			detailButton {
				// One arc, ready/total (ruled 2026-09-18): a filled circle
				// must mean the import is COMPLETE — cataloged-but-not-
				// thumbnailed never fills it (the batch loop outruns the
				// thumbnail drain, so a cataloged arc reads full early).
				// The cataloged count lives in the popover instead.
				SegmentedRing(segments: [
					.init(fraction: Double(ready) / Double(total), color: .blue),
				])
				.frame(width: 13, height: 13)
			}
			.help("Importing — \(ready) of \(total) ready")
			.accessibilityLabel("Import progress")
			.accessibilityValue("\(ready) of \(total) files ready")
		case .done(let outcome, _, let failures):
			if outcome == .completed && failures == 0 {
				detailButton {
					Image(systemName: "checkmark.circle")
						.foregroundStyle(.green)
				}
				.help("Import complete")
				.accessibilityLabel("Import complete")
			} else {
				detailButton {
					Image(systemName: "exclamationmark.triangle")
						.foregroundStyle(.yellow)
				}
				.help("Import finished with problems")
				.accessibilityLabel("Import finished with problems")
			}
		case .needsResume:
			detailButton {
				Image(systemName: "exclamationmark.triangle")
					.foregroundStyle(.yellow)
			}
			.help("Import didn't finish")
			.accessibilityLabel("Import didn't finish, reimport to complete")
		}
	}

	private func detailButton(@ViewBuilder label: () -> some View) -> some View {
		Button {
			showingDetail = true
		} label: {
			// contentShape: the ring is stroked circles with no fill, so
			// without it clicks fall through to the List row's selection.
			label()
				.contentShape(Rectangle())
		}
		.buttonStyle(.borderless)
		.popover(isPresented: $showingDetail, arrowEdge: .trailing) {
			detail.padding(12)
		}
	}

	/// Popover content, switching on the same status the control did.
	@ViewBuilder private var detail: some View {
		switch status {
		case .progress(let ready, let cataloged, let total):
			VStack(alignment: .leading, spacing: 8) {
				Text("Importing “\(name)”").font(.headline)
				Text("\(cataloged) of \(total) cataloged · \(ready) ready")
					.foregroundStyle(.secondary)
				Button("Cancel Import", role: .destructive) {
					showingDetail = false
					onCancel()
				}
			}
		case .done(let outcome, let imported, let failures):
			VStack(alignment: .leading, spacing: 8) {
				Text(doneTitle(outcome)).font(.headline)
				// Minimal bridge (ruled 2026-09-18): counts only, never
				// silence. TODO: residue round — per-file, per-reason
				// drill-down and a retry-failures affordance.
				Text(failures > 0
					? "\(imported) imported · \(failures) failed"
					: "\(imported) imported")
					.foregroundStyle(.secondary)
				if outcome != .completed {
					Text("Reimport the folder to complete it.")
						.foregroundStyle(.secondary)
				}
				Button("Dismiss") {
					showingDetail = false
					onDismiss()
				}
			}
		case .needsResume:
			VStack(alignment: .leading, spacing: 8) {
				Text("Import didn't finish").font(.headline)
				// TODO: actionable resume (ratified, deferred 2026-09-18) —
				// a Resume Import button here invoking ImportService.
				Text("Reimport “\(name)” to pick it back up where it left off.")
					.foregroundStyle(.secondary)
			}
		default:
			EmptyView()
		}
	}

	private func doneTitle(_ outcome: ImportOutcome) -> String {
		switch outcome {
		case .completed: "Import complete"
		case .canceled: "Import canceled"
		case .failed: "Import failed"
		}
	}
}

#Preview("States") {
	List {
		FolderItem(name: "Plain", status: nil)
		FolderItem(name: "Walking", status: .walking)
		FolderItem(name: "Importing", status: .progress(ready: 220, cataloged: 340, total: 1000))
		FolderItem(name: "Finishing", status: .finishing)
		FolderItem(name: "Clean", status: .done(outcome: .completed, imported: 1000, failures: 0))
		FolderItem(name: "Residue", status: .done(outcome: .completed, imported: 997, failures: 3))
		FolderItem(name: "Failed", status: .done(outcome: .failed, imported: 400, failures: 12))
		FolderItem(name: "Resume Me", status: .needsResume)
	}
	.frame(width: 240)
}
