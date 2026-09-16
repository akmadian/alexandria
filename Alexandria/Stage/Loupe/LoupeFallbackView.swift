//
//  LoupeFallbackView.swift
//  Alexandria
//
//  Created by ari on 9/16/26.
//

import SwiftUI
import QuickLookUI

/// The interim loupe: whatever the file is, Quick Look renders it — the same
/// engine Finder's spacebar uses, embedded inline. A stand-in until bespoke
/// per-kind views land. Takes a resolved on-disk URL; the caller owns the
/// nil/offline case (an unmounted volume never reaches here).
struct LoupeFallbackView: View {
	let url: URL

	var body: some View {
		QuickLookPreview(url: url)
	}
}

/// The AppKit bridge, private so nothing outside this file touches QLPreviewView
/// or NSViewRepresentable — the composer sees only LoupeFallbackView.
private struct QuickLookPreview: NSViewRepresentable {
	let url: URL

	func makeNSView(context: Context) -> QLPreviewView {
		// .normal keeps QL's own chrome (video scrubber, PDF paging). The failable
		// init returns nil only if the view can't be created — fall back rather
		// than force-unwrap.
		let view = QLPreviewView(frame: .zero, style: .normal) ?? QLPreviewView()
		view.autostarts = false          // no surprise audio/video playback in the loupe
		view.previewItem = url as NSURL
		return view
	}

	func updateNSView(_ view: QLPreviewView, context: Context) {
		// Swap the item in place on cursor change — never rebuild the view.
		guard view.previewItem?.previewItemURL != url else { return }
		view.previewItem = url as NSURL
	}
}

// TODO: Quick Look renders in previewd, a separate process, so files outside our
// container — user-imported originals, and network volumes (smb/nfs) once those
// mount rungs land — need a security-scoped bookmark started before the URL
// resolves, or the preview comes back blank. Wire bookmark access when originals
// live off local, sandbox-reachable storage.
