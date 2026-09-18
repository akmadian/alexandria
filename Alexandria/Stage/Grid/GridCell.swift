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
		VStack() {
			HStack() {
				if let position = state.position {
					Text("\(position + 1)")
						.foregroundStyle(.secondary)
				}
				Spacer()
				switch (state.asset?.kind) {
				case .image: Image(systemName: "photo")
				case .video: Image(systemName: "video")
				case .audio: Image(systemName: "waveform")
				case .document: Image(systemName: "document")
				case .vector: Image(systemName: "squareshape.controlhandles.on.squareshape.controlhandles")
				case .project: Image(systemName: "rectangle.stack")
				case .other: Image(systemName: "document")
				case .sidecar: Image(systemName: "info")
				case .none: Image(systemName: "questionmark")
				}
			}
			Spacer()
			HStack(spacing: 4) {
				if let name = state.file?.fileStem {
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
		.padding(2)
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

// MARK: - Preview

@MainActor
private struct GridCellPreview: View {
	private struct PreviewCell {
		let state: CellState
		let thumbnail: ThumbnailLeafView
	}

	private let cells: [PreviewCell]

	init() {
		func make(
			name: String, ext: String,
			kind: FileKind = .image,
			pos: Int,
			rating: Int? = nil,
			flag: Asset.Flag? = nil,
			prominence: CellProminence = .idle
		) -> PreviewCell {
			let state = CellState()
			let fileId = Identifier<File>(rawValue: UUID())
			let assetId = Identifier<Asset>(rawValue: UUID())
			state.file = File(
				id: fileId,
				folderId: Identifier(rawValue: UUID()),
				assetId: assetId,
				importId: Identifier(rawValue: UUID()),
				name: name,
				nameKey: name.lowercased(),
				fileStem: String(name.dropLast(ext.count + 1)),
				fileExtension: ext,
				kind: kind,
				sizeBytes: 24_385_024,
				modifiedAt: Date(timeIntervalSinceReferenceDate: 0),
				contentHash: nil,
				missing: false,
				metadata: nil,
				metadataVersion: 0,
				thumbnailAt: nil,
				formationRule: nil
			)
			state.asset = Asset(
				id: assetId,
				kind: kind,
				rating: rating,
				flag: flag,
				representativeFileId: fileId
			)
			state.position = pos
			state.prominence = prominence
			return PreviewCell(state: state, thumbnail: ThumbnailLeafView())
		}

		cells = [
			make(name: "DSC_0482.NEF", ext: "NEF", pos: 0, rating: 5, flag: .pick, prominence: .cursor),
			make(name: "DSC_0483.NEF", ext: "NEF", pos: 1, rating: 3, prominence: .selected),
			make(name: "A_Very_Long_Filename_That_Truncates_In_The_Bar.TIFF", ext: "TIFF", pos: 2),
			make(name: "Interview_Raw_Cut.MOV", ext: "MOV", kind: .video, pos: 3, rating: 2),
			make(name: "Ambient_Session.WAV", ext: "WAV", kind: .audio, pos: 4),
			make(name: "Shoot_Notes.PDF", ext: "PDF", kind: .document, pos: 5, flag: .reject),
		]
	}

	var body: some View {
		let side: CGFloat = 160
		LazyVGrid(
			columns: Array(repeating: SwiftUI.GridItem(.fixed(side), spacing: 2), count: 3),
			spacing: 2
		) {
			ForEach(cells.indices, id: \.self) { i in
				GridCell(state: cells[i].state, thumbnail: cells[i].thumbnail)
					.frame(width: side, height: side)
			}
		}
		.padding(2)
		.background(Color(nsColor: .windowBackgroundColor))
	}
}

#Preview("Grid Cells") {
	GridCellPreview()
}

