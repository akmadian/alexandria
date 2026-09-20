//
//  GridCell.swift
//  Alexandria
//
//  The cell (recomposed 2026-09-20, contact-sheet anatomy): a zoned
//  container — header decoration, a content slot, prominence styled on the
//  cell surface and the content frame. The slot mounts a view the ITEM owns
//  and paints imperatively; the cell only places it and never learns what
//  it is — thumbnail today, any other content surface tomorrow. Decoration
//  reads one doorway, `CellState`, mutated in place across reuse — the cell
//  asks for nothing and holds no asset, file, or query.
//
//  Zone contents and styling values are a starting point, not a ratified
//  composition — Ari's playground (like VolumeHeader). The face switch is
//  wired pathways with placeholder glyphs; each kind's real face is filled
//  in as it's designed. The metadata-shown mode remains a documented seam:
//  a second body, same state, same slot, denser chrome.
//

import AppKit
import SwiftUI

/// The cell's four-state, minus hover (deferred until something needs it).
/// One value, resolved in one place, styled in one place — a restyle or a
/// new state touches `resolve` and the styling switches, nothing else.
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
	let content: NSView

	var body: some View {
		VStack(spacing: 0) {
			// Header — starting set: index + filename. Ari's playground.
			HStack {
				if let position = state.position {
					Text("\(position + 1)")
				}
				Spacer()
				if let name = state.file?.fileStem {
					Text(name).truncationMode(.middle)
				}
			}
			.padding(6)
			.background(cellHeaderColor)
			.font(.caption2)
			.lineLimit(1)

			// Content slot — placed here, owned by the item, painted elsewhere.
			ContentSlot(view: content)
				.padding(6)
				.overlay {
					// Face pathways; placeholder glyphs until each kind earns
					// a real face. `.none` = records not landed yet (quiet
					// ground); `.image` = the pixels speak for themselves.
					switch state.asset?.kind {
					case .none, .image: EmptyView()
					case .video: Image(systemName: "video")
					case .audio: Image(systemName: "waveform")
					case .document: Image(systemName: "document")
					case .vector: Image(systemName: "squareshape.controlhandles.on.squareshape.controlhandles")
					case .project: Image(systemName: "rectangle.stack")
					case .other: Image(systemName: "document")
					case .sidecar: Image(systemName: "info")
					}
				}
		}
		.background(cellBackgroundColor)
		.border(frame.color, width: frame.width)
	}

	// The four-state's entire styling — these two switches, values in
	// Theme.Grid. Prominence reads on the cell surface and the content
	// frame (the LrC construction), not an edge ring.
	private var cellBackgroundColor: Color {
		switch state.prominence {
		case .idle: Theme.Grid.cellBackground
		case .selected: Theme.Grid.cellBackground
		case .cursor: Theme.Grid.cellCursorBackground
		}
	}
	
	private var cellHeaderColor: Color {
		switch state.prominence {
		case .idle: Theme.Grid.cellHeader
		case .selected: Theme.Grid.cellBackground
		case .cursor: Theme.Grid.cellCursorBackground
		}
	}

	private var frame: (color: Color, width: CGFloat) {
		switch state.prominence {
		case .idle: (.clear, 0)
		case .selected:
			(
				Color(nsColor: .gray).opacity(Theme.Grid.selectedRingOpacity),
				Theme.Grid.selectedRingWidth
			)
		case .cursor: (Color(nsColor: .white), Theme.Grid.cursorRingWidth)
		}
	}
}

/// The AppKit↔SwiftUI seam, whole: mounts the item-owned content view into
/// the cell's layout. Placement only — the cell never paints it and never
/// knows its concrete type.
private struct ContentSlot: NSViewRepresentable {
	let view: NSView
	func makeNSView(context: Context) -> NSView { view }
	func updateNSView(_ nsView: NSView, context: Context) {}
}

// MARK: - Preview

@MainActor
private struct GridCellPreview: View {
	private struct PreviewCell {
		let state: CellState
		let content: NSView
	}

	private let cells: [PreviewCell]

	init() {
		func make(
			name: String, ext: String,
			kind: FileKind = .image,
			pos: Int,
			prominence: CellProminence = .idle,
			pixels: String? = nil
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
				rating: nil,
				flag: nil,
				representativeFileId: fileId
			)
			state.position = pos
			state.prominence = prominence
			let leaf = ThumbnailLeafView()
			if let pixels, let image = NSImage(contentsOf: fixture(pixels)) {
				leaf.show(image)
			}
			return PreviewCell(state: state, content: leaf)
		}

		cells = [
			make(
				name: "DSC_0482.NEF", ext: "NEF", pos: 0, prominence: .cursor,
				pixels: "real-jpg_6150009.JPG"),
			make(
				name: "DSC_0483.NEF", ext: "NEF", pos: 1, prominence: .selected,
				pixels: "_6160345-.jpg"),
			make(
				name: "A_Very_Long_Filename_That_Truncates.TIFF", ext: "TIFF", pos: 2,
				pixels: "_6160501-.jpg"),
			make(name: "Interview_Raw_Cut.MOV", ext: "MOV", kind: .video, pos: 3),
			make(name: "Ambient_Session.WAV", ext: "WAV", kind: .audio, pos: 4),
			make(name: "Shoot_Notes.PDF", ext: "PDF", kind: .document, pos: 5),
		]
	}

	var body: some View {
		let side: CGFloat = 160
		LazyVGrid(
			columns: Array(repeating: SwiftUI.GridItem(.fixed(side), spacing: 2), count: 3),
			spacing: 2
		) {
			ForEach(cells.indices, id: \.self) { i in
				GridCell(state: cells[i].state, content: cells[i].content)
					.frame(width: side, height: side)
			}
		}
		.padding(2)
		.background(Color(nsColor: .windowBackgroundColor))
	}
}

/// repo-root/TestData, resolved from this source file's location — same
/// trick LoupeView's preview uses. Dev-machine paths never ship.
private func fixture(_ name: String) -> URL {
	URL(fileURLWithPath: #filePath)
		.deletingLastPathComponent()   // Grid/
		.deletingLastPathComponent()   // Stage/
		.deletingLastPathComponent()   // Alexandria/
		.deletingLastPathComponent()   // repo root
		.appending(path: "TestData")
		.appending(path: name)
}

#Preview("Grid Cells") {
	GridCellPreview()
}
