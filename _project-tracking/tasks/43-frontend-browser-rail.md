# 43 — The browser rail: Tree's first real consumer

**Areas:** frontend. **Blocked by:** 42-seam-wire-shapes.md. Parallel to 44.
**References:** D37 (the Tree primitive), D41 (mapping + count + offline rulings), C1/C2
(scope is the durable nav axis; the state equation), C14, design constitution §13 (scent
counts), §14 (one glyph per kind), §15 (single scope); `frontend/CLAUDE.md` §2/§4 (state
planes; fetch policy).

## Scope

`features/browser` — fetch the nav axes, map onto `TreeNodeData`, drive scope.

- **Hooks in `api/queries.ts`:** `useFolderTree()`, `useCollections()` (TanStack;
  `retry: false`; explicit error render per the architecture record). **Invalidation:**
  `catalog`/`changed` through the existing event-pump gate — counts stay fresh through
  imports (D41).
- **Mapping** (the feature's only real logic; `buildForest` for collections is a pure
  unit-tested helper):

  | Axis | icon | count | children | notes |
  |---|---|---|---|---|
  | Volume | `"source"` (hard-drive glyph) | `.assetCount` | `.folders` | offline → **visual state, still selectable** (D41); never `isDisabled` |
  | Folder | `"folder"` | `.assetCount` (subtree) | `.children` | tracked roots + derived nodes render identically |
  | Collection | `"collection"` | `.assetCount` (`null` → no badge) | from `parentId` | smart + manual; kind may pick a glyph variant |
  | Tags section | `"tag"` | — | — | coming-soon placeholder, `isDisabled` — the one sanctioned use |

- **Selection → scope** (single, §15): Volume → `{kind:"volume-wide folder scope"}` i.e.
  `{kind:"folder", volumeId, path:"", recursive:true}`; Folder → same with its path;
  Collection → `{kind:"collection", id}`. Dispatched into the catalog store; scope is C2's
  extensional root, distinct from filter.
- Offline visual treatment: dimmed icon + offline mark, tokens only — a constitution-
  conformant state, eye-gated with the round. The rail is chrome: hue-free.
- i18n complete (C14); loading/error/empty states rendered, not implied.

## Out of scope

Manage-sources verbs (44), tags backend (task 10 — the placeholder section is this task's
whole tag surface), rail filtering/search, drag-and-drop.

## Acceptance

- Against the mock: all three sections render; selecting any node updates the grid's working
  set to exactly that scope; C3 holds (leaving to a task view and back restores selection +
  scroll).
- Counts: folder badges equal what clicking shows; a smart and a manual collection both badge
  correctly; `null` count renders no badge (not `0`).
- Offline volume: visibly offline, still selectable, its cataloged assets browse normally.
- An import (mock ticking job) visibly bumps rail counts without user action.
- Keyboard: RAC tree navigation + typeahead reach every node.
