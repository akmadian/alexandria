//
//  SidecarAttachRule.swift
//  Alexandria
//

nonisolated extension FormationRule {
	/// Sidecar attachment: a sidecar belongs with the file it describes.
	/// Same-folder only — sidecar naming conventions are directory-local by
	/// nature. Two convention forms:
	///
	///   photo.raf.xmp → subject photo.raf   (exact form: the stem carries
	///                                        the subject's full name)
	///   photo.xmp     → subject photo.*     (short form: shared stem)
	///
	/// This rule confirms EVERY describing relationship; the engine's
	/// selection stage keeps one subject per sidecar (exact form, else raw
	/// capture, else lowest file id — deterministic, never iteration
	/// order). A sidecar with no confirmed subject abstains and stays
	/// formation-pending; cross-import adoption is a deferred round
	/// (_design/grouping.md).
	static let sidecarAttach = FormationRule(id: "sidecar_attach") { a, b in
		guard let (sidecar, subject) = sidecarAndSubject(a, b),
		      sidecar.record.folderId == subject.record.folderId
		else { return false }
		return sidecarDescribes(sidecar, subject) || sidecar.record.fileStem == subject.record.fileStem
	}
}
