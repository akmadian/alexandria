//
//  SegmentedRing.swift
//  Alexandria
//
//  A determinate progress ring drawn as consecutive arcs — the reusable
//  segmented counterpart to ProgressView(.circular), which SwiftUI only
//  offers single-fraction (ProgressViewStyle exposes fractionCompleted
//  alone, so segmentation can't be a style). Fractions are absolute
//  (0...1, summed ≤ 1); the remainder renders as track.
//

import SwiftUI

struct SegmentedRing: View {
	struct Segment: Equatable {
		let fraction: Double
		let color: Color
	}

	let segments: [Segment]
	var lineWidth: CGFloat = 2

	var body: some View {
		ZStack {
			Circle().stroke(.quaternary, lineWidth: lineWidth)
			ForEach(arcs.indices, id: \.self) { index in
				let arc = arcs[index]
				Circle()
					.trim(from: arc.from, to: arc.to)
					.stroke(arc.color, style: StrokeStyle(lineWidth: lineWidth, lineCap: .butt))
			}
		}
		.rotationEffect(.degrees(-90))  // 12 o'clock start
		.padding(lineWidth / 2)  // stroke straddles the path; keep it inside the frame
	}

	/// Segments laid end to end, clamped into 0...1 — a malformed sum
	/// overdraws nothing past the full circle.
	private var arcs: [(from: Double, to: Double, color: Color)] {
		var cursor = 0.0
		return segments.map { segment in
			let from = cursor
			cursor = min(cursor + max(segment.fraction, 0), 1)
			return (from, cursor, segment.color)
		}
	}
}

#Preview {
	VStack(spacing: 12) {
		SegmentedRing(segments: [
			.init(fraction: 0.55, color: .green),
			.init(fraction: 0.25, color: .blue),
		])
		.frame(width: 14, height: 14)
		SegmentedRing(segments: [.init(fraction: 1, color: .green)])
			.frame(width: 14, height: 14)
	}
	.padding()
}
