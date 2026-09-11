//
//  FormationFile.swift
//  Alexandria
//

import Foundation

/// One file as asset formation sees it: the committed row plus its decoded
/// metadata — the rules' evidence. Metadata that is absent or undecodable is
/// nil; rules treat missing evidence as abstention, never as refutation.
/// Formation reads only these values — it never touches the disk.
nonisolated struct FormationFile: Sendable {
	let record: File
	let metadata: FileMetadata?

	var id: Identifier<File> { record.id }
	var isFormed: Bool { record.assetId != nil }
	var isSidecar: Bool { record.kind == .sidecar }
}

// Shared derivations: the facets and evidence every rule reads the same way.
nonisolated extension FormationFile {
	/// The registry's rawness facet, resolved from the stored extension.
	var isRawCapture: Bool {
		FileFormat.resolve(extension: record.fileExtension, contentType: nil).isRawCapture
	}

	var capturedAt: Date? { metadata?.capturedAt }

	/// Camera identity as normalized make+model; nil when the file carries
	/// neither. Serial number joins this when the extractor learns it.
	var cameraIdentity: String? {
		guard let metadata else { return nil }
		let make = metadata.cameraMake?.trimmingCharacters(in: .whitespaces).lowercased() ?? ""
		let model = metadata.cameraModel?.trimmingCharacters(in: .whitespaces).lowercased() ?? ""
		if make.isEmpty && model.isEmpty { return nil }
		return make + "|" + model
	}
}

// Sidecar vocabulary — shared derivations, not rule internals: the attach
// rule, the engine's exact-form candidate reach, its subject selection, and
// its admission asymmetry all read the same two facts.

/// Orients a candidate pair: exactly one sidecar and one non-sidecar, or nil.
nonisolated func sidecarAndSubject(
	_ a: FormationFile, _ b: FormationFile
) -> (sidecar: FormationFile, subject: FormationFile)? {
	switch (a.isSidecar, b.isSidecar) {
	case (true, false): (a, b)
	case (false, true): (b, a)
	default: nil
	}
}

/// The exact convention form: the sidecar's stem is the subject's full name
/// ("photo.raf.xmp" describes "photo.raf"). In the subject tie-break, exact
/// form outranks the short (shared-stem) form.
nonisolated func sidecarDescribes(_ sidecar: FormationFile, _ subject: FormationFile) -> Bool {
	sidecar.record.fileStem == subject.record.fileStem + "." + subject.record.fileExtension
}
