package xmp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/akmadian/alexandria/internal/catalog"
	"github.com/akmadian/alexandria/internal/dependency"
	"github.com/akmadian/alexandria/internal/domain"
	"github.com/akmadian/alexandria/internal/settings"
	"github.com/cespare/xxhash/v2"
	"github.com/charmbracelet/log"
)

// Syncer reconciles one asset against its sidecar in both directions:
//   - Inbound: sidecar → catalog (rating, label, keywords via union).
//   - Outbound: catalog → sidecar (when xmpWriteBack is enabled).
//
// Inbound judgment applies via AssetSyncWriter.ApplyXMPInbound, which by
// construction never bumps judgment_modified_at — loop prevention level 2.
// Outbound writes merge into the existing sidecar (preserving foreign namespaces)
// and record the new hash for the watcher's echo check — loop prevention level 1.
//
// Still pending: caption/title inbound (blocked on a sparse observation writer).
type Syncer struct {
	daemon   *dependency.ExiftoolDaemon
	reader   catalog.AssetReader
	writer   catalog.AssetSyncWriter
	keywords catalog.TagRepository // nil = skip tag union (e.g. judgment-only tests)
	settings func() settings.Settings
	logger   *log.Logger
}

func NewSyncer(daemon *dependency.ExiftoolDaemon, reader catalog.AssetReader, writer catalog.AssetSyncWriter, keywords catalog.TagRepository, settingsFunc func() settings.Settings, logger *log.Logger) *Syncer {
	if logger == nil {
		logger = log.Default()
	}
	if settingsFunc == nil {
		settingsFunc = func() settings.Settings { return settings.DefaultSettings() }
	}
	return &Syncer{daemon: daemon, reader: reader, writer: writer, keywords: keywords, settings: settingsFunc, logger: logger}
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

	current := s.settings()
	policy := PolicyXMPWins
	if current.XMPConflictResolution == string(PolicyCatalogWins) {
		policy = PolicyCatalogWins
	}
	state := SyncState{
		SidecarChanged:   asset.XMPHash == nil || *asset.XMPHash != hash,
		CatalogChanged:   catalogChanged(asset),
		WriteBackEnabled: current.XMPWriteBack,
	}
	action := Decide(state, policy)
	s.logger.Debug("xmp: sync decision", "asset", asset.ID, "sidecar", sidecarPath,
		"action", action, "sidecarChanged", state.SidecarChanged, "catalogChanged", state.CatalogChanged)

	inbound := action == ActionApplyInbound || (action == ActionConflict && policy.InboundWins())
	outbound := action == ActionWriteOutbound || (action == ActionConflict && !policy.InboundWins())

	// Tags are the documented exception to the judgment policy: they always UNION,
	// both directions, never delete on absence (impl/06). So we read + union keywords
	// whenever the SIDECAR changed — even under catalog_wins, where the judgment
	// verdict is outbound — as long as a keyword importer is wired.
	unionTags := state.SidecarChanged && s.keywords != nil
	if !inbound && !outbound && !unionTags {
		return action, nil
	}

	if inbound || unionTags {
		fields, err := Read(ctx, s.daemon, sidecarPath)
		if err != nil {
			return action, err
		}

		if inbound {
			patch := s.toTriagePatch(fields, asset.ID)
			if err := s.writer.ApplyXMPInbound(ctx, asset.ID, patch, time.Now().UTC(), hash); err != nil {
				return action, fmt.Errorf("xmp: apply inbound %s: %w", asset.ID, err)
			}
			s.logger.Info("xmp: applied inbound judgment", "asset", asset.ID, "action", action,
				"rating", patch.Rating.Set, "label", patch.ColorLabel.Set)
		}

		if unionTags && (len(fields.Tags) > 0 || len(fields.Hierarchical) > 0) {
			hierarchical := splitHierarchical(fields.Hierarchical)
			if err := s.keywords.ImportKeywords(ctx, asset.ID, fields.Tags, hierarchical, "xmp"); err != nil {
				return action, fmt.Errorf("xmp: import keywords %s: %w", asset.ID, err)
			}
			s.logger.Info("xmp: unioned keywords", "asset", asset.ID,
				"flat", len(fields.Tags), "hierarchical", len(hierarchical))
		}

		if fields.Caption != "" || fields.Title != "" {
			s.logger.Debug("xmp: caption/title present but not yet applied (pending sparse observation writer)",
				"asset", asset.ID, "hasCaption", fields.Caption != "", "hasTitle", fields.Title != "")
		}
	}

	if outbound {
		if err := s.writeOutbound(ctx, asset, sidecarPath); err != nil {
			return action, err
		}
	}

	return action, nil
}

// writeOutbound pushes the asset's current catalog values into the sidecar, then
// records the write cursor + new hash so the watcher's echo check drops the
// resulting file event.
func (s *Syncer) writeOutbound(ctx context.Context, asset *domain.Asset, sidecarPath string) error {
	fields := WriteFields{
		Rating:     asset.Rating,
		ColorLabel: asset.ColorLabel,
		Caption:    ptrOr(asset.Caption, ""),
		Title:      ptrOr(asset.Title, ""),
	}

	if s.keywords != nil {
		flat, hierarchical, err := s.keywords.AssetTagNames(ctx, asset.ID)
		if err != nil {
			return fmt.Errorf("xmp: read tags for outbound %s: %w", asset.ID, err)
		}
		fields.Tags = flat
		fields.Hierarchical = hierarchical
	}

	if err := Write(ctx, s.daemon, sidecarPath, fields); err != nil {
		return err
	}

	written, err := os.ReadFile(sidecarPath)
	if err != nil {
		return fmt.Errorf("xmp: hash after write %s: %w", sidecarPath, err)
	}
	newHash := HashSidecar(written)
	now := time.Now().UTC()
	if err := s.writer.RecordXMPWritten(ctx, asset.ID, now, newHash); err != nil {
		return fmt.Errorf("xmp: record written %s: %w", asset.ID, err)
	}
	s.logger.Info("xmp: wrote outbound sidecar", "asset", asset.ID, "path", sidecarPath)
	return nil
}

func ptrOr(pointer *string, fallback string) string {
	if pointer != nil {
		return *pointer
	}
	return fallback
}

// splitHierarchical turns lr:hierarchicalSubject strings ("Travel|Japan|Tokyo")
// into per-node chains for ImportKeywords, dropping empty segments. Keeping the
// "|" convention here leaves the tag repo format-agnostic.
func splitHierarchical(paths []string) [][]string {
	var out [][]string
	for _, path := range paths {
		var chain []string
		for _, part := range strings.Split(path, "|") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				chain = append(chain, trimmed)
			}
		}
		if len(chain) > 0 {
			out = append(out, chain)
		}
	}
	return out
}

// toTriagePatch maps the judgment subset of a sidecar onto a TriagePatch. Inbound
// apply is WHOLESALE, not an overlay: the sidecar is authoritative, so a field it
// omits CLEARS the catalog value. This is what upholds the conflict policy — under
// xmp_wins the sidecar wins including its removals (matching LrC "Read Metadata");
// under catalog_wins we never reach here (that verdict writes outbound instead), so
// the catalog is upheld either way. Clearing is safe because the 3-way merge already
// routed any user judgment newer than the last sync to a conflict — a plain
// apply-inbound only clears state that was already in sync, i.e. a genuine sidecar
// removal. Note is never synced (Alexandria-private); flag lives in a custom
// namespace we don't read yet (best-effort, open question #8) so it stays untouched.
func (s *Syncer) toTriagePatch(fields Fields, assetID string) catalog.TriagePatch {
	var patch catalog.TriagePatch

	switch {
	case fields.Rating == nil:
		patch.Rating = domain.ClearOpt[int]()
	case *fields.Rating >= 0 && *fields.Rating <= 5:
		patch.Rating = domain.SetOpt(*fields.Rating)
	default:
		// XMP ratings range -1..5 (-1 = "rejected"); our schema's CHECK allows only
		// 0..5. An unrepresentable value clears rather than leaving a stale rating —
		// still upholding "sidecar wins". Never mapped onto flag (lossy, opt-in P3, D15).
		patch.Rating = domain.ClearOpt[int]()
		s.logger.Warn("xmp: rating out of range, cleared", "asset", assetID, "rating", *fields.Rating)
	}

	if label, ok := NormalizeLabel(fields.Label); ok {
		patch.ColorLabel = domain.SetOpt(label)
	} else {
		// Empty or unrecognized: the field map leaves color_label unset (the raw
		// string is preserved for round-trip by the outbound write, once it exists).
		patch.ColorLabel = domain.ClearOpt[domain.ColorLabel]()
		if fields.Label != "" {
			s.logger.Warn("xmp: unrecognized color label, left unset", "asset", assetID, "label", fields.Label)
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
