//
//  GridCell.swift
//  Alexandria
//
//  The SwiftUI cell (cell round, 2026-09-18): content + decoration, and
//  nothing else. Content is the thumbnail slot — an AppKit leaf the ITEM
//  owns, laid out here but painted imperatively by the coordinator, so
//  pixels never enter SwiftUI's update cycle (the swap-flash fix from the
//  grid round survives inside the slot untouched). Decoration reads one
//  doorway, `CellState`, mutated in place across reuse — the cell asks for
//  nothing and holds no asset, file, or query.
//
//  This is the bare composition. The metadata-shown mode is a documented
//  seam: a second body beside `body`, same state, same slot, different
//  chrome — it must not reopen the container or the item. When faces
//  diverge by kind (audio glyph, document), the kind switch lands HERE,
//  once, compiler-exhaustive — never in the coordinator.
//

import AppKit
import SwiftUI

/// The cell's four-state, minus hover (deferred until something needs it).
/// One value, resolved in one place, styled in one place — a restyle or a
/// new state touches `resolve` and the ring, nothing else.
nonisolated enum CellProminence {
	case idle
	case selected
	/// The cursor outranks plain selection; keyboard focus reads at a glance.
	case cursor

	static func resolve(
		isCursor: Bool, isSelected: Bool, highlightState: NSCollectionViewItem.HighlightState
	) -> CellProminence {
		if isCursor { return .cursor }
		if isSelected || highlightState == .forSelection || highlightState == .asDropTarget {
			return .selected
		}
		return .idle
	}
}

/// The one doorway for non-pixel cell state. The item mutates it in place
/// on selection changes and reuse; SwiftUI observes. Never recreated for a
/// recycled cell — that in-place update is what keeps a reconfigure at one
/// cheap body evaluation.
///
/// Metadata crosses as canonical records (ruled 2026-09-18): the cell is a
/// schema consumer like the inspector, no translation layer. The
/// coordinator batch-fills these — the cell still asks for nothing.
@MainActor @Observable final class CellState {
	var prominence: CellProminence = .idle
	/// Zero-based slot in the working set; the coordinator refreshes it
	/// after a delivery, since a diff shifts positions without reconfigure.
	var position: Int?
	/// nil for file subjects, and for asset subjects until resolution lands.
	var asset: Asset?
	/// The representative file (or the file subject itself); nil = file-less.
	var file: File?

	func clear() {
		prominence = .idle
		position = nil
		asset = nil
		file = nil
	}
}

@MainActor struct GridCell: View {
	let state: CellState
	let thumbnail: ThumbnailLeafView

	var body: some View {
		ThumbnailSlot(view: thumbnail)
			.overlay(alignment: .bottom) { decoration }
			.overlay(ring)
	}

	/// The decoration row: what the pixels can't say. Composition and
	/// styling are Ari's playground; the DATA path behind each element is
	/// fixed (records via the coordinator, position via the id↔position
	/// table).
	@ViewBuilder private var decoration: some View {
		HStack(spacing: 4) {
			if let position = state.position {
				Text("\(position + 1)")
					.foregroundStyle(.secondary)
			}
			if let name = state.file?.name {
				Text(name)
					.truncationMode(.middle)
			}
			Spacer(minLength: 0)
			if let rating = state.asset?.rating, rating > 0 {
				Text(String(repeating: "★", count: rating))
			}
		}
		.font(.caption2)
		.lineLimit(1)
		.padding(.horizontal, 4)
		.padding(.vertical, 2)
		.background(.ultraThinMaterial)
	}

	/// The four-state's entire styling, in one spot.
	@ViewBuilder private var ring: some View {
		switch state.prominence {
		case .idle:
			EmptyView()
		case .selected:
			Rectangle()
				.strokeBorder(
					Color(nsColor: .controlAccentColor).opacity(Theme.Grid.selectedRingOpacity),
					lineWidth: Theme.Grid.selectedRingWidth
				)
		case .cursor:
			Rectangle()
				.strokeBorder(
					Color(nsColor: .controlAccentColor),
					lineWidth: Theme.Grid.cursorRingWidth
				)
		}
	}
}

/// The AppKit↔SwiftUI seam, whole: SwiftUI lays out a view it does not own.
/// The item creates the leaf and the coordinator paints it; this wrapper
/// only places it.
private struct ThumbnailSlot: NSViewRepresentable {
	let view: ThumbnailLeafView
	func makeNSView(context: Context) -> ThumbnailLeafView { view }
	func updateNSView(_ nsView: ThumbnailLeafView, context: Context) {}
}
