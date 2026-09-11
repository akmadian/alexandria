//
//  RawRenditionPairRule.swift
//  Alexandria
//

import Foundation

nonisolated extension FormationRule {
	/// RAW+rendition pairing (ratified 2026-09-11): same import, same stem
	/// (the candidate generator's blocking key), raw on one side and a
	/// rendition on the other, gated by evidence.
	///
	/// Check order IS the semantics: refutations veto first (evidence
	/// present on BOTH sides and disagreeing); same-folder then pairs on the
	/// stem alone; cross-folder must earn the pair with positive
	/// corroboration, because abstention protects nothing against stripped
	/// metadata (the archive acid case: 2015/IMG_1234.CR2 + 2024/IMG_1234.JPG
	/// must not pair).
	///
	/// Deliberate narrowness: raw pairs with rendition, never raw↔raw (a DNG
	/// beside its original) or rendition↔rendition (a PNG export beside a
	/// JPEG) — those are lineage/stacking questions, not same-work
	/// manifestation.
	static let rawRenditionPair = FormationRule(id: "raw_rendition_pair") { a, b in
		guard a.record.kind == .image, b.record.kind == .image,
		      a.isRawCapture != b.isRawCapture
		else { return false }

		let gap = captureGap(a, b)
		if let gap, gap > captureTimeTolerance { return false }
		if let cameraA = a.cameraIdentity, let cameraB = b.cameraIdentity, cameraA != cameraB { return false }
		if a.record.folderId == b.record.folderId { return true }
		if let gap, gap <= captureTimeTolerance { return true }
		if let cameraA = a.cameraIdentity, let cameraB = b.cameraIdentity, cameraA == cameraB { return true }
		return false
	}
}

/// EXIF stores capture time at second granularity, and a RAW and its
/// rendition are written in the same instant; ±2s matches digiKam's
/// time-grouping window.
private nonisolated let captureTimeTolerance: TimeInterval = 2

private nonisolated func captureGap(_ a: FormationFile, _ b: FormationFile) -> TimeInterval? {
	guard let capturedA = a.capturedAt, let capturedB = b.capturedAt else { return nil }
	return abs(capturedA.timeIntervalSince(capturedB))
}
