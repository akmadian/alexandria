package domain

// CreateFolderOutcomeKind is the disposition of a folder-add attempt (D41's
// "disjoint roots, graceful merge — reject nothing" rule). Adding a folder never
// fails on overlap; instead the engine reports which of these happened so the UI
// can show the result in place:
//
//   - created: a new, disjoint tracked root was minted.
//   - already_tracked_within: the path is a tracked root or lives under one, so
//     the request redirects to that root (this also covers an exact duplicate).
//   - absorbed: the path is a parent of one or more existing roots, so a new
//     parent root was minted and the children absorbed under it (QUIET by
//     default, per D41's dated note).
//   - needs_confirmation: the absorb WOULD change behavior (a watched or
//     scheduled child falling under a parent with a different sync_mode). No
//     mutation happened; the caller re-issues with confirm=true to proceed.
//
// This mirrors task 40's engine outcomes; a sibling worktree may declare the
// same values, and identical members make the merge trivial (as with SyncMode).
type CreateFolderOutcomeKind string

const (
	CreateFolderCreated              CreateFolderOutcomeKind = "created"
	CreateFolderAlreadyTrackedWithin CreateFolderOutcomeKind = "already_tracked_within"
	CreateFolderAbsorbed             CreateFolderOutcomeKind = "absorbed"
	CreateFolderNeedsConfirmation    CreateFolderOutcomeKind = "needs_confirmation"
)
