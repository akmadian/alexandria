package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/akmadian/alexandria/internal/domain"
)

// TagRepo is the tag/asset_tags repository. This increment builds only the two
// consumer-driven paths (impl/10): keyword import (impl/06 XMP sync, impl/09 LrC
// migration) and its find-or-create + union primitives, plus the derived-path
// rebuild. The tag-management UI surface (Tree/Get/Update/Delete/replace-semantics
// SetAssetTags) is deferred until that UI is the caller.
//
// Atomicity and scoping ride the shared Store/Repos/InTx seam: a whole
// ImportKeywords runs inside one Store.InTx, so a half-built hierarchy never
// commits.
type TagRepo struct {
	DB DBTX
}

// EnsureTag finds a tag by (slug, parentID) or creates it — the find-or-create
// atom. A new tag mints a UUIDv7, computes its path from the parent, and defaults
// color_mode='inherit'. Sibling uniqueness (the UNIQUE(slug, IFNULL(parent_id,''))
// index) makes the lookup a single indexed probe.
func (r *TagRepo) EnsureTag(ctx context.Context, name string, parentID *string) (string, error) {
	slug := domain.Slugify(name)

	var id string
	// IFNULL collapse mirrors the unique index so a NULL parent matches the root scope.
	err := r.DB.QueryRowContext(ctx,
		"SELECT id FROM tags WHERE slug = ? AND IFNULL(parent_id, '') = IFNULL(?, '')",
		slug, parentID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("tags: lookup %q: %w", slug, err)
	}

	id = domain.NewID()
	path, err := r.childPath(ctx, id, parentID)
	if err != nil {
		return "", err
	}
	_, err = r.DB.ExecContext(ctx,
		`INSERT INTO tags (id, name, slug, parent_id, color_mode, path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, name, slug, parentID, string(domain.ColorInherit), path, formatTime(time.Now().UTC()))
	if err != nil {
		return "", fmt.Errorf("tags: create %q: %w", slug, err)
	}
	return id, nil
}

// childPath builds a new tag's materialized path from its parent's: root →
// '/selfId/', child → parent.path + 'selfId/'.
func (r *TagRepo) childPath(ctx context.Context, id string, parentID *string) (string, error) {
	if parentID == nil {
		return "/" + id + "/", nil
	}
	var parentPath string
	if err := r.DB.QueryRowContext(ctx, "SELECT path FROM tags WHERE id = ?", *parentID).Scan(&parentPath); err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("tags: parent %s not found", *parentID)
		}
		return "", err
	}
	return parentPath + id + "/", nil
}

// AddAssetTags unions tagIDs onto an asset with the given source. INSERT … ON
// CONFLICT DO NOTHING makes it idempotent and, critically, tombstone-safe: a
// suppressed row's PK already exists so the insert no-ops, never clearing
// removed_at (a judgment column this observation-class write may not touch). This
// is what closes the XMP round-trip resurrection bug — a user-deleted tag stays
// gone even as the sidecar keeps re-asserting it.
func (r *TagRepo) AddAssetTags(ctx context.Context, assetID string, tagIDs []string, source string) error {
	if len(tagIDs) == 0 {
		return nil
	}
	now := formatTime(time.Now().UTC())
	for _, tagID := range tagIDs {
		_, err := r.DB.ExecContext(ctx,
			`INSERT INTO asset_tags (asset_id, tag_id, source, created_at)
			 VALUES (?, ?, ?, ?) ON CONFLICT(asset_id, tag_id) DO NOTHING`,
			assetID, tagID, source, now)
		if err != nil {
			return fmt.Errorf("tags: attach %s→%s: %w", assetID, tagID, err)
		}
	}
	// TODO(fts): recompose assets_fts.tags for this asset once FTS⋈tags lands
	// (impl/10 "FTS integration — DEFERRED", pending the FTS deep-dive). Until then
	// tag text is only searchable after a RebuildFTS.
	return nil
}

// ImportKeywords is the orchestrator both sync consumers call (inside a
// Store.InTx). It builds each hierarchy chain leaf-first, records every node name
// it saw, then attaches the leaf of each path plus any flat name NOT already
// carried by the hierarchy — dedupe rule: lr:hierarchicalSubject is authoritative,
// dc:subject only contributes genuinely-flat keywords. `hierarchical` is pre-split
// by the caller so the repo stays free of the XMP "|" convention.
func (r *TagRepo) ImportKeywords(ctx context.Context, assetID string, flat []string, hierarchical [][]string, source string) error {
	var leafIDs []string
	hierarchySlugs := make(map[string]struct{}) // every node slug seen in the hierarchy

	for _, chain := range hierarchical {
		var parentID *string
		var leafID string
		for _, rawName := range chain {
			name := strings.TrimSpace(rawName)
			if name == "" {
				continue
			}
			id, err := r.EnsureTag(ctx, name, parentID)
			if err != nil {
				return err
			}
			hierarchySlugs[domain.Slugify(name)] = struct{}{}
			leafID = id
			node := id
			parentID = &node // descend
		}
		if leafID != "" {
			leafIDs = append(leafIDs, leafID)
		}
	}

	for _, rawName := range flat {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		if _, carried := hierarchySlugs[domain.Slugify(name)]; carried {
			continue // already attached via its hierarchical path
		}
		id, err := r.EnsureTag(ctx, name, nil)
		if err != nil {
			return err
		}
		leafIDs = append(leafIDs, id)
	}

	return r.AddAssetTags(ctx, assetID, leafIDs, source)
}

// RebuildTagPaths recomputes every tag's path from parent_id (the derived-state
// rebuild path, mirroring RebuildFTS). Used to repair path or after a bulk
// structural change. The tree is small, so a full read + per-node UPDATE is fine.
func (r *TagRepo) RebuildTagPaths(ctx context.Context) error {
	rows, err := r.DB.QueryContext(ctx, "SELECT id, parent_id FROM tags")
	if err != nil {
		return err
	}
	parent := make(map[string]string) // id → parentID ('' = root)
	var ids []string
	for rows.Next() {
		var id string
		var parentID sql.NullString
		if err := rows.Scan(&id, &parentID); err != nil {
			rows.Close()
			return err
		}
		parent[id] = parentID.String
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	memo := make(map[string]string)
	inProgress := make(map[string]bool)
	var pathOf func(id string) (string, error)
	pathOf = func(id string) (string, error) {
		if p, ok := memo[id]; ok {
			return p, nil
		}
		if inProgress[id] {
			return "", fmt.Errorf("tags: cycle detected at %s", id) // FK+reparent guard should prevent this
		}
		inProgress[id] = true
		parentID := parent[id]
		p := "/" + id + "/"
		if parentID != "" {
			parentPath, err := pathOf(parentID)
			if err != nil {
				return "", err
			}
			p = parentPath + id + "/"
		}
		delete(inProgress, id)
		memo[id] = p
		return p, nil
	}

	for _, id := range ids {
		p, err := pathOf(id)
		if err != nil {
			return err
		}
		if _, err := r.DB.ExecContext(ctx, "UPDATE tags SET path = ? WHERE id = ?", p, id); err != nil {
			return err
		}
	}
	return nil
}
