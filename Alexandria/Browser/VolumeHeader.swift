//
//  VolumeHeader.swift
//  Alexandria
//
//  The browser's volume header row: the LABEL of the volume's disclosure
//  group, componentized so structure and styling are edited here, in one
//  place, with the preview as the playground. It owns everything inside the
//  row; the List owns everything around it (selection, hover, chevron,
//  indentation) — sidebar row chrome breaks fast when fought (a custom
//  DisclosureGroupStyle kills context menus on macOS), so this view never
//  touches it. Fixed colors are the other trap: hierarchical foreground
//  styles remap when the row highlights, fixed colors don't.
//

import SwiftUI

/// What the row says about its volume's presence. Derived by the browser
/// from identity + VolumeMonitor; the header only renders it, so the
/// preview can show every state without a monitor.
enum VolumeAvailability {
	case mounted
	/// Identified, but nothing mounted answers to the identity — will come
	/// back when the drive does.
	case unmounted
	/// No identity recorded: can never be matched when drives mount.
	/// Distinct from offline on purpose.
	case unknown
}

struct VolumeHeader: View {
	let name: String
	let kind: VolumeKind
	let availability: VolumeAvailability

	var body: some View {
		HStack(spacing: 6) {
			// Identity zone: kind-differentiated icon + name.
			Image(systemName: icon)
				.overlay(alignment: .topTrailing) {
					Circle()
						.fill(availability == .mounted ? .green : .gray)
						.frame(width: 6, height: 6)
				}
			Text(name)
		}
		.foregroundStyle(
			availability == .mounted ? AnyShapeStyle(.primary) : AnyShapeStyle(.secondary)
		)
	}

	private var icon: String {
		switch kind {
		case .local: "internaldrive"
		case .external: "externaldrive"
		case .network: "externaldrive.connected.to.line.below"
		}
	}
}

/// The styling playground: every kind × availability that renders
/// differently, in the sidebar list style the real row lives in.
#Preview {
	List {
		Section("Volumes") {
			VolumeHeader(name: "Macintosh HD", kind: .local, availability: .mounted)
			VolumeHeader(name: "Scratch SSD", kind: .external, availability: .mounted)
			VolumeHeader(name: "Archive", kind: .external, availability: .unmounted)
			VolumeHeader(name: "NAS Photos", kind: .network, availability: .mounted)
			VolumeHeader(name: "NAS Photos", kind: .network, availability: .unmounted)
			VolumeHeader(name: "Old Import", kind: .external, availability: .unknown)
		}
	}
	.listStyle(.sidebar)
	.frame(width: 240, height: 260)
}
