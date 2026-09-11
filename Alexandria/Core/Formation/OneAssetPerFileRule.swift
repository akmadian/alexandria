//
//  OneAssetPerFileRule.swift
//  Alexandria
//

nonisolated extension FormationRule {
	/// The universal floor: every file may found its own asset, alone — the
	/// singleton is the common case, and it is what a later pass's pair rule
	/// can join. Confirmed on the reflexive candidate (a file's relationship
	/// with itself), which generation emits for every unformed file.
	///
	/// Sidecars are the one exception: a sidecar rides a subject, never
	/// founds — an unattached sidecar stays formation-pending.
	///
	/// LAST in the registry by design: founding must not outrank a real
	/// admission, or every file's provenance would read as the floor. The
	/// provenance tests pin this ordering.
	static let oneAssetPerFile = FormationRule(id: "one_asset_per_file") { a, b in
		a.id == b.id && !a.isSidecar
	}
}
