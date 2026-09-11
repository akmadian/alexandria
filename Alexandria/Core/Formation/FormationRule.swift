//
//  FormationRule.swift
//  Alexandria
//

/// One named formation pattern: a pure pairwise judgment over a candidate
/// relationship. Candidate generation and sidecar-subject selection are the
/// engine's stages (AssetFormation); a rule only answers whether these two
/// files manifest the same work.
///
/// The body's shape carries the evidence semantics: refutations return
/// false early, corroborations return true, and the final line returns
/// false — so a check that cannot fire (missing metadata) abstains by
/// falling through. Rules are leaves: they never invoke each other, and
/// each lives in one file with its tunables.
nonisolated struct FormationRule: Sendable {
	/// Written to files.formation_rule on admission — provenance: the rule
	/// that admitted THAT file, a historical fact.
	let id: String

	/// Do these two files manifest the same work? Symmetric by convention.
	let confirms: @Sendable (FormationFile, FormationFile) -> Bool
}

