// Domain → TreeNode adapters. Pure functions; the Tree component never sees
// domain types, features never see tree internals. Models are flat today
// (no parentId) so these produce flat root lists — when the models grow
// hierarchy, only these functions change.

import type { TreeNode } from "@/components/tree/tree";
import type { Collection, Source, Tag } from "@/api/contract";

export const sourcesToNodes = (sources: Source[]): TreeNode<Source>[] =>
    sources.map((s) => ({ id: `source:${s.id}`, label: s.name, data: s }));
// TODO(folder tree): children come from useFolderTree(sourceId) on first expand —
// scope {kind:"folder"} browsing is designed (docs/project-tracking/frontend/02-state-model.md), not yet wired.

export const collectionsToNodes = (collections: Collection[]): TreeNode<Collection>[] =>
    collections.map((c) => ({ id: `collection:${c.id}`, label: c.name, data: c }));

export const tagsToNodes = (tags: Tag[]): TreeNode<Tag>[] =>
    tags.map((t) => ({ id: `tag:${t.id}`, label: t.name, data: t }));
