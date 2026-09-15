//
//  StarRating.swift
//  Alexandria
//
//  Created by ari on 9/14/26.
//

import SwiftUI

/// Reusable star-rating control. One primitive, two modes:
///
///   StarRating(asset.rating)                   // display only
///   StarRating(asset.rating) { catalog.set($0) } // editable
///
/// Ratings are 1–5; `nil` is unrated. `0` is never a rating (see the schema's
/// `CHECK (rating BETWEEN 1 AND 5)`), so clearing sets `nil`: clicking the
/// current rating's star again unrates.
///
/// While editing, hovering previews the prospective value with outline stars:
/// a 2-star asset hovered on star 4 shows two `star.fill`, two `star`, and one
/// dot. Levels past the value (or past the hover preview) are dots, not empty
/// stars, so an unrated control still reads as five discrete slots.
///
/// Color follows the ambient `foregroundStyle` (filled = primary, prospective
/// = secondary, dots = tertiary); size follows the ambient `font`. Override
/// either at the call site.
struct StarRating: View {
	private let value: Int?
	private let onSet: ((Int?) -> Void)?

	@State private var hover: Int?
	@FocusState private var focused: Bool

	/// `onSet == nil` is display-only. Supplying it makes the control editable;
	/// it fires with the new rating (`nil` when the user unrates).
	init(_ value: Int?, onSet: ((Int?) -> Void)? = nil) {
		self.value = value
		self.onSet = onSet
	}

	private var isEditable: Bool { onSet != nil }
	private var committed: Int { value ?? 0 }

	/// What the stars should reflect right now: the hover preview while
	/// editing, otherwise the committed value.
	private var shown: Int { (isEditable ? hover : nil) ?? committed }

	var body: some View {
		HStack(spacing: 2) {
			ForEach(1...5, id: \.self) { level in
				glyph(for: level)
					.contentShape(.rect)
					.onHover { if $0 { hover = level } }
					.onTapGesture {
						guard let onSet else { return }
						onSet(level == value ? nil : level) // reclick clears
						focused = true // clicking gives the control the keyboard
					}
			}
		}
		// Only the whole control clears the hover, so sliding between stars
		// never flickers back to the committed value mid-gesture.
		.onHover { if !$0 { hover = nil } }
		.animation(.easeOut(duration: 0.1), value: shown)
		.focusable(isEditable)
		.focused($focused)
		.onKeyPress { press in
			guard isEditable else { return .ignored }
			switch press.key {
			case .leftArrow, .downArrow: step(-1); return .handled
			case .rightArrow, .upArrow: step(1); return .handled
			default:
				// Digit keys set directly; 0 unrates.
				guard let n = press.characters.first?.wholeNumberValue, (0...5).contains(n)
				else { return .ignored }
				onSet?(n == 0 ? nil : n)
				return .handled
			}
		}
		.accessibilityElement(children: .ignore)
		.accessibilityLabel("Rating")
		.accessibilityValue(committed == 0 ? "Unrated" : "\(committed) of 5 stars")
		.accessibilityAdjustableAction { direction in
			guard isEditable else { return }
			switch direction {
			case .increment: step(1)
			case .decrement: step(-1)
			@unknown default: break
			}
		}
	}

	/// Move the rating by `delta`, clamped to 1...5; stepping below 1 unrates.
	/// Shared by the arrow keys and the accessibility adjustable action.
	private func step(_ delta: Int) {
		let next = committed + delta
		onSet?(next < 1 ? nil : min(next, 5))
	}

	@ViewBuilder
	private func glyph(for level: Int) -> some View {
		if level <= committed {
			Image(systemName: "star.fill")
		} else if level <= shown {
			Image(systemName: "star") // prospective
				.foregroundStyle(.secondary)
		} else {
			Image(systemName: "circle.fill") // unselected slot
				.scaleEffect(0.25)
				.foregroundStyle(.tertiary)
		}
	}
}

#if DEBUG
#Preview("Display") {
	VStack(alignment: .leading, spacing: 12) {
		StarRating(nil)
		StarRating(2)
		StarRating(5)
	}
	.padding()
}

#Preview("Editable") {
	@Previewable @State var rating: Int? = 2
	VStack(alignment: .leading, spacing: 12) {
		StarRating(rating) { rating = $0 }
		Text(rating.map { "\($0)" } ?? "unrated")
			.font(.caption)
			.foregroundStyle(.secondary)
	}
	.padding()
}
#endif
