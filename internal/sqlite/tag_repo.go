package sqlite

import (
	"context"
	"database/sql"
	"errors"
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
// color_mode='inherit'. Sibling uniqueness (the UNIQUE(slug, IFNULL(parent_id,”))
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
	if !errors.Is(err, sql.ErrNoRows) {
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
	return r.recomposeFTSTags(ctx, assetID)
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

// recomposeFTSTags rewrites the tags column in assets_fts for one asset.
// Space-joined display names of ACTIVE (removed_at IS NULL) tags, including
// ancestor names for hierarchical hits ("wedding" matches assets tagged
// "weddings/2026").
func (r *TagRepo) recomposeFTSTags(ctx context.Context, assetID string) error {
	var tagsText string
	err := r.DB.QueryRowContext(ctx, `
		SELECT COALESCE(group_concat(name, ' '), '') FROM (
			SELECT DISTINCT t.name
			FROM asset_tags at
			JOIN tags t ON t.id = at.tag_id
			WHERE at.asset_id = ? AND at.removed_at IS NULL
		)`,
		assetID).Scan(&tagsText)
	if err != nil {
		return fmt.Errorf("tags: recompose fts for %s: %w", assetID, err)
	}

	// Also include ancestor tag names for hierarchical search.
	var ancestorText string
	err = r.DB.QueryRowContext(ctx, `
		SELECT COALESCE(group_concat(name, ' '), '') FROM (
			SELECT DISTINCT ancestor.name
			FROM asset_tags at
			JOIN tags leaf ON leaf.id = at.tag_id
			JOIN tags ancestor ON leaf.path LIKE '%/' || ancestor.id || '/%'
			WHERE at.asset_id = ? AND at.removed_at IS NULL AND ancestor.id != leaf.id
		)`,
		assetID).Scan(&ancestorText)
	if err != nil {
		return fmt.Errorf("tags: recompose fts ancestors for %s: %w", assetID, err)
	}

	combined := tagsText
	if ancestorText != "" {
		if combined != "" {
			combined += " "
		}
		combined += ancestorText
	}

	_, err = r.DB.ExecContext(ctx,
		"UPDATE assets_fts SET tags = ? WHERE asset_id = ?",
		combined, assetID)
	if err != nil {
		return fmt.Errorf("tags: update fts for %s: %w", assetID, err)
	}
	return nil
}

// AssetTagNames returns the active tag names for an asset, split into flat names
// (dc:subject) and pipe-delimited hierarchical paths (lr:hierarchicalSubject)
// for XMP outbound write. A tag with ancestors gets a hierarchical entry
// ("Travel|Japan|Tokyo"); a root-level tag is flat only. Both lists are
// deduplicated and sorted for stable output.
func (r *TagRepo) AssetTagNames(ctx context.Context, assetID string) ([]string, []string, error) {
	rows, err := r.DB.QueryContext(ctx, `
		SELECT t.name, t.parent_id IS NOT NULL AS has_parent, t.path
		FROM asset_tags at
		JOIN tags t ON t.id = at.tag_id
		WHERE at.asset_id = ? AND at.removed_at IS NULL
		ORDER BY t.name`, assetID)
	if err != nil {
		return nil, nil, fmt.Errorf("tags: asset tag names %s: %w", assetID, err)
	}
	defer rows.Close()

	var flat []string
	type leafInfo struct {
		name      string
		hasParent bool
		path      string
	}
	var leaves []leafInfo
	for rows.Next() {
		var info leafInfo
		if err := rows.Scan(&info.name, &info.hasParent, &info.path); err != nil {
			return nil, nil, err
		}
		flat = append(flat, info.name)
		if info.hasParent {
			leaves = append(leaves, info)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var hierarchical []string
	for _, leaf := range leaves {
		chain, err := r.buildHierarchyChain(ctx, leaf.path)
		if err != nil {
			return nil, nil, err
		}
		hierarchical = append(hierarchical, strings.Join(chain, "|"))
	}
	return flat, hierarchical, nil
}

// buildHierarchyChain walks a materialized path to produce a name chain for
// lr:hierarchicalSubject.
func (r *TagRepo) buildHierarchyChain(ctx context.Context, tagPath string) ([]string, error) {
	// Extract IDs from the path: "/rootId/childId/leafId/" → [rootId, childId, leafId]
	parts := strings.Split(strings.Trim(tagPath, "/"), "/")
	if len(parts) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(parts))
	arguments := make([]any, len(parts))
	for i, id := range parts {
		placeholders[i] = "?"
		arguments[i] = id
	}
	rows, err := r.DB.QueryContext(ctx,
		"SELECT id, name FROM tags WHERE id IN ("+strings.Join(placeholders, ",")+`)`, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nameByID := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		nameByID[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	chain := make([]string, 0, len(parts))
	for _, id := range parts {
		if name, ok := nameByID[id]; ok {
			chain = append(chain, name)
		}
	}
	return chain, nil
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
			_ = rows.Close()
			return err
		}
		parent[id] = parentID.String
		ids = append(ids, id)
	}
	_ = rows.Close()
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
