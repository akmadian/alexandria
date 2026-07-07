package xmp

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/dependency"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/cespare/xxhash/v2"
	"github.com/charmbracelet/log"
)

// Syncer reconciles one asset against its sidecar. This increment implements the
// INBOUND JUDGMENT application only — rating and color label via the sync writer
// (AssetSyncWriter.ApplyXMPInbound, which by construction never bumps
// judgment_modified_at, so applying a sidecar can never masquerade as a user edit
// and trigger an outbound write: the D15 loop-prevention level 2).
//
// Deliberately NOT here yet, each blocked on infrastructure that does not exist:
//   - Keyword union (dc:subject/lr:hierarchicalSubject → asset_tags source='xmp')
//     needs the whole tag repository (find-or-create, hierarchy nodes, FTS tags
//     maintenance) — deferred from impl/04, not built.
//   - Caption/title (dc:description/dc:title → observation columns) needs a sparse
//     observation-metadata writer; ApplyFilePatch always rewrites the file-fact
//     columns, so it would clobber them with zeros here.
//   - Outbound writes (write-back) — a separate increment with its own settings.
//
// So WriteBackEnabled is hard-wired false and outbound/catalog-wins verdicts are
// logged and skipped. Inbound rating/label is the complete, testable slice.
type Syncer struct {
	daemon *dependency.ExiftoolDaemon
	writer catalog.AssetSyncWriter
	policy ConflictPolicy
	logger *log.Logger
}

func NewSyncer(daemon *dependency.ExiftoolDaemon, writer catalog.AssetSyncWriter, policy ConflictPolicy, logger *log.Logger) *Syncer {
	if logger == nil {
		logger = log.Default()
	}
	return &Syncer{daemon: daemon, writer: writer, policy: policy, logger: logger}
}

// SyncSidecar reconciles asset against the sidecar file at sidecarPath and returns
// the action taken. The asset carries the sync-state cursors the conflict decision
// reads; the caller (ingest / watcher hint) supplies the freshly-loaded asset.
func (s *Syncer) SyncSidecar(ctx context.Context, asset *domain.Asset, sidecarPath string) (Action, error) {
	content, err := os.ReadFile(sidecarPath)
	if err != nil {
		return "", fmt.Errorf("xmp: read sidecar %s: %w", sidecarPath, err)
	}
	hash := HashSidecar(content)

	state := SyncState{
		SidecarChanged:   asset.XMPHash == nil || *asset.XMPHash != hash,
		CatalogChanged:   catalogChanged(asset),
		WriteBackEnabled: false, // outbound write-back is a later increment
	}
	action := Decide(state, s.policy)
	s.logger.Debug("xmp: sync decision", "asset", asset.ID, "sidecar", sidecarPath,
		"action", action, "sidecarChanged", state.SidecarChanged, "catalogChanged", state.CatalogChanged)

	inbound := action == ActionApplyInbound || (action == ActionConflict && s.policy.InboundWins())
	if !inbound {
		if action != ActionNoop {
			s.logger.Info("xmp: outbound sync pending (write-back not implemented)",
				"asset", asset.ID, "action", action)
		}
		return action, nil
	}

	fields, err := Read(ctx, s.daemon, sidecarPath)
	if err != nil {
		return action, err
	}
	patch := s.toTriagePatch(fields, asset.ID)
	if err := s.writer.ApplyXMPInbound(ctx, asset.ID, patch, time.Now().UTC(), hash); err != nil {
		return action, fmt.Errorf("xmp: apply inbound %s: %w", asset.ID, err)
	}
	s.logger.Info("xmp: applied inbound judgment", "asset", asset.ID, "action", action,
		"rating", patch.Rating.Set, "label", patch.ColorLabel.Set)

	if len(fields.Tags) > 0 || fields.Caption != "" || fields.Title != "" {
		s.logger.Debug("xmp: tags/caption/title present but not yet applied (pending tag repo + observation writer)",
			"asset", asset.ID, "tags", len(fields.Tags), "hasCaption", fields.Caption != "", "hasTitle", fields.Title != "")
	}
	return action, nil
}

// toTriagePatch maps the judgment subset of a sidecar onto a TriagePatch. It is
// SET-ONLY: a field absent from the sidecar is left untouched, never cleared — a
// sparse sidecar (rating but no label, say) must not wipe a judgment the user set
// in Alexandria. Note is never synced (Alexandria-private); flag lives in a custom
// namespace we don't read yet (best-effort, open question #8).
func (s *Syncer) toTriagePatch(fields Fields, assetID string) catalog.TriagePatch {
	var patch catalog.TriagePatch

	if fields.Rating != nil {
		// XMP ratings range -1..5 (-1 = "rejected"); our schema's CHECK allows only
		// 0..5. Skip out-of-range rather than let it abort the whole apply — and we
		// never map a rejected rating onto flag (lossy mapping is opt-in P3, D15).
		if *fields.Rating >= 0 && *fields.Rating <= 5 {
			patch.Rating = domain.SetOpt(*fields.Rating)
		} else {
			s.logger.Warn("xmp: rating out of range, skipped", "asset", assetID, "rating", *fields.Rating)
		}
	}
	if fields.Label != "" {
		if label, ok := NormalizeLabel(fields.Label); ok {
			patch.ColorLabel = domain.SetOpt(label)
		} else {
			s.logger.Warn("xmp: unrecognized color label, left unmapped", "asset", assetID, "label", fields.Label)
		}
	}
	return patch
}

// catalogChanged reports whether a user judgment postdates the last sync in either
// direction — the "catalog changed?" input to the 3-way merge. A never-judged asset
// has not changed; a judged-but-never-synced asset has.
func catalogChanged(asset *domain.Asset) bool {
	if asset.JudgmentModifiedAt == nil {
		return false
	}
	lastSync := laterTime(asset.XMPLastReadAt, asset.XMPLastWrittenAt)
	if lastSync == nil {
		return true
	}
	return asset.JudgmentModifiedAt.After(*lastSync)
}

func laterTime(a, b *time.Time) *time.Time {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case b.After(*a):
		return b
	default:
		return a
	}
}

// HashSidecar is the sync-state fingerprint of a sidecar's raw bytes. It is
// xmp-sync's OWN cursor (stored in assets.xmp_hash and compared only against
// itself), so the algorithm is free — it just has to be stable and shared with the
// watcher's file-level echo check (D15 loop-prevention level 1), which compares an
// on-disk sidecar to xmp_hash to drop our own write-back echoes.
func HashSidecar(content []byte) string {
	return fmt.Sprintf("%x", xxhash.Sum64(content))
}
