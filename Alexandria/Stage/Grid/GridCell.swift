//
//  GridCell.swift
//  Alexandria
//
//  The cell (value architecture, 2026-09-20): a pure function of one
//  immutable value. `CellModel` is everything the cell shows — records,
//  decoded pixels, prominence — and data flows one way: the coordinator
//  computes it, the item assigns it, SwiftUI diffs it. There is no
//  observable doorway, no paint seam, no reuse contract beyond "assign
//  the empty value".
//
//  The model carries DATA, never views: NSImage is the cacheable decoded-
//  pixel container (the pipeline's product), wrapped by `Image(nsImage:)`
//  at render time. Content per kind is a view-builder branch — a future
//  in-cell experience (video playback) is a model field plus a branch,
//  measured precedent: the leaf-vs-SwiftUI spike (2026-09-20) showed no
//  frame cost to whole-value diffing at fast-scroll scale.
//
//  Zone contents and styling values are a starting point, not a ratified
//  composition — Ari's playground (like VolumeHeader). The metadata-shown
//  mode remains a documented seam: a second body, same model, denser
//  chrome.
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

/// Everything the cell shows, as one value. Metadata crosses as canonical
/// records (ruled 2026-09-18): the cell is a schema consumer like the
/// inspector, no translation layer.
nonisolated struct CellModel: Equatable {
	/// Zero-based slot in the working set; the coordinator refreshes it
	/// after a delivery, since a diff shifts positions without reconfigure.
	var position: Int? = nil
	/// nil for file subjects, and for asset subjects until resolution lands.
	var asset: Asset? = nil
	/// The representative file (or the file subject itself); nil = file-less.
	var file: File? = nil
	/// Decoded pixels from the imaging pipeline; nil = quiet ground.
	/// Compared by reference — same object, same pixels.
	var thumbnail: NSImage? = nil
	var prominence: CellProminence = .idle

	static let empty = CellModel()

	static func == (lhs: CellModel, rhs: CellModel) -> Bool {
		lhs.position == rhs.position
			&& lhs.asset == rhs.asset
			&& lhs.file == rhs.file
			&& lhs.thumbnail === rhs.thumbnail
			&& lhs.prominence == rhs.prominence
	}
}

@MainActor struct GridCell: View {
	let model: CellModel

	var body: some View {
		VStack(spacing: 0) {
			VStack(spacing: 0) { // HEADER
				HStack {
					if let position = model.position {
						Text("\(position + 1)")
					}
					Spacer()
					if let name = model.file?.fileStem {
						Text(name)
							.truncationMode(.middle)
					}
				}
				.foregroundStyle(.black)
				.font(.footnote)
				.padding(0)
				HStack {
					Text("BL")
					Spacer()
					Text("BR")
				}
				.padding(0)
				.foregroundStyle(.black)
				.font(.footnote)
			}
			.padding(.horizontal, Theme.Grid.cellContentPadding)
			.padding(.vertical, Theme.Grid.cellContentPadding / 2)
			.background(headerColor)
			.lineLimit(1)
			
			Divider()

			content
				.frame(maxWidth: .infinity, maxHeight: .infinity)
				.padding(Theme.Grid.cellContentPadding)
		}
		.background(backgroundColor)
		.border(frame.color, width: frame.width)
	}

	/// Content per kind: pixels for visual kinds, placeholder glyphs for
	/// the rest until each earns a real presentation. `nil` kind = records
	/// not landed yet (quiet ground — the cell background shows through).
	@ViewBuilder private var content: some View {
		switch model.asset?.kind {
		case .none, .image, .video: thumbnail
		case .audio: glyph("waveform")
		case .document: glyph("document")
		case .vector: glyph("squareshape.controlhandles.on.squareshape.controlhandles")
		case .project: glyph("rectangle.stack")
		case .other: glyph("document")
		case .sidecar: glyph("info")
		}
	}

	@ViewBuilder private var thumbnail: some View {
		if let pixels = model.thumbnail {
			Image(nsImage: pixels)
				.resizable()
				.aspectRatio(contentMode: .fit)
				.border(.black, width: 1)
		} else {
			// Quiet ground, in the cell's own surface color. MUST be spatial:
			// an EmptyView here ignores the frame modifier entirely and the
			// cell background collapses to the header strip (fast-scroll bug,
			// 2026-09-20) — geometry is fixed from first paint, with or
			// without pixels (grid.md invariant).
			backgroundColor
		}
	}

	private func glyph(_ name: String) -> some View {
		Image(systemName: name)
	}

	// The four-state's entire styling — these switches, values in
	// Theme.Grid. Prominence reads on the cell surface and the content
	// frame (the LrC construction), not an edge ring.
	private var backgroundColor: Color {
		switch model.prominence {
		case .idle: Theme.Grid.cellBackground
		case .selected: Theme.Grid.cellHeaderColor
		case .cursor: Theme.Grid.cellCursorBackground
		}
	}

	private var headerColor: Color {
		switch model.prominence {
		case .idle: Theme.Grid.cellHeaderColor
		case .selected: Theme.Grid.cellHeaderColor
		case .cursor: Theme.Grid.cellCursorBackground
		}
	}

	private var frame: (color: Color, width: CGFloat) {
		switch model.prominence {
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

// MARK: - Preview

@MainActor
private struct GridCellPreview: View {
	private let models: [CellModel]

	init() {
		func make(
			name: String, ext: String,
			kind: FileKind = .image,
			pos: Int,
			prominence: CellProminence = .idle,
			pixels: String? = nil
		) -> CellModel {
			let fileId = Identifier<File>(rawValue: UUID())
			let assetId = Identifier<Asset>(rawValue: UUID())
			return CellModel(
				position: pos,
				asset: Asset(
					id: assetId,
					kind: kind,
					rating: nil,
					flag: nil,
					representativeFileId: fileId
				),
				file: File(
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
				),
				thumbnail: pixels.flatMap { NSImage(contentsOf: fixture($0)) },
				prominence: prominence
			)
		}

		models = [
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
			// Pixels not landed yet (the fast-scroll state): the ground must
			// hold the cell's full extent — this is the collapse-bug canary.
			make(name: "DSC_0484.NEF", ext: "NEF", pos: 6),
		]
	}

	var body: some View {
		let side: CGFloat = 160
		LazyVGrid(
			columns: Array(repeating: SwiftUI.GridItem(.fixed(side), spacing: 2), count: 3),
			spacing: Theme.Grid.interCellSpacing
		) {
			ForEach(models.indices, id: \.self) { i in
				GridCell(model: models[i])
					.frame(width: side, height: side)
			}
		}
		.padding(Theme.Grid.inset)
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
