package xmp

// Action is the verdict of the file-level 3-way merge for one asset's sidecar.
type Action string

const (
	// ActionNoop — neither side changed since the last sync. The overwhelming case;
	// a sync pass over an unchanged catalog is ~free.
	ActionNoop Action = "noop"
	// ActionApplyInbound — the sidecar changed, the catalog did not: read the sidecar
	// into the catalog.
	ActionApplyInbound Action = "apply-inbound"
	// ActionWriteOutbound — the catalog changed, the sidecar did not: write the
	// catalog out to the sidecar (only when write-back is enabled).
	ActionWriteOutbound Action = "write-outbound"
	// ActionConflict — both sides changed: resolved per the configured policy.
	ActionConflict Action = "conflict"
)

// ConflictPolicy is the tie-breaker when both sides changed (setting
// xmpConflictResolution). xmp_wins is the default.
type ConflictPolicy string

const (
	PolicyXMPWins     ConflictPolicy = "xmp_wins"
	PolicyCatalogWins ConflictPolicy = "catalog_wins"
)

// SyncState is the three inputs the decision reads, already reduced to booleans by
// the caller (which owns the hash and timestamp comparisons against the sync-state
// columns):
//
//   - SidecarChanged: current sidecar hash != assets.xmp_hash.
//   - CatalogChanged: judgment_modified_at > max(xmp_last_read_at, xmp_last_written_at).
//   - WriteBackEnabled: the xmpWriteBack setting.
//
// Keeping the comparisons in the caller keeps this a pure table, trivially testable
// and matching impl/06's conflict grid exactly.
type SyncState struct {
	SidecarChanged   bool
	CatalogChanged   bool
	WriteBackEnabled bool
}

// Decide returns the action for one asset. It is the impl/06 conflict grid:
//
//	sidecar  catalog  → action
//	 no       no        noop
//	 yes      no        apply inbound
//	 no       yes       write outbound (only if write-back on; else noop)
//	 yes      yes       conflict → policy
//
// Tags are the documented exception: they always union both directions regardless
// of this verdict, so the caller merges keywords even on a noop/conflict — that
// union is not routed through here.
func Decide(state SyncState, policy ConflictPolicy) Action {
	switch {
	case state.SidecarChanged && state.CatalogChanged:
		if policy == PolicyCatalogWins {
			// Catalog wins a conflict: push our values out to the sidecar. Still a
			// write, so it needs write-back; otherwise there is nothing to do until
			// the user enables it.
			if state.WriteBackEnabled {
				return ActionConflict
			}
			return ActionNoop
		}
		// xmp_wins (default): take the sidecar. Always actionable — inbound needs no
		// write-back permission.
		return ActionConflict
	case state.SidecarChanged:
		return ActionApplyInbound
	case state.CatalogChanged:
		if state.WriteBackEnabled {
			return ActionWriteOutbound
		}
		return ActionNoop
	default:
		return ActionNoop
	}
}

// InboundWins reports whether a conflict resolves toward the sidecar (values flow
// into the catalog) or the catalog (values flow out to the sidecar). Only
// meaningful when Decide returned ActionConflict.
func (policy ConflictPolicy) InboundWins() bool {
	return policy != PolicyCatalogWins
}
