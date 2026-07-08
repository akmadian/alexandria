// The flagship reusable (docs/project-tracking/frontend/01-flows-and-views.md): one Tree serves
// folders-in-sources, collections, and tags. Generic over the node payload —
// Tree never looks inside `data`; features adapt their domain to TreeNode<T>
// in pure functions and interpret it back in onSelect/renderDecoration.
//
// Built on RAC Tree: roving tabindex, arrow keys, aria-expanded, typeahead all
// come from react-aria-components. This file is adapter + styling + persistence.

import { ChevronRight } from "lucide-react";
import { useCallback, useState } from "react";
import {
    Button as AriaButton,
    Collection as AriaCollection,
    Tree as AriaTree,
    TreeItem as AriaTreeItem,
    TreeItemContent as AriaTreeItemContent,
    type Key,
    type Selection,
} from "react-aria-components";
import { Icon } from "@/components/icon/icon";
import type { ReactNode } from "react";
import s from "./tree.module.css";

export interface TreeNode<T> {
    id: string;
    label: string;
    children?: TreeNode<T>[];
    data: T;
}

interface TreeProps<T> {
    /** Stable id — expansion state persists to localStorage under it. */
    treeId: string;
    "aria-label": string;
    nodes: TreeNode<T>[];
    selectedId: string | null;
    onSelect: (node: TreeNode<T>) => void;
    /** Count badge, status dot, color chip — rendered right-aligned. */
    renderDecoration?: (node: TreeNode<T>) => ReactNode;
}

function indexNodes<T>(nodes: TreeNode<T>[], into: Map<string, TreeNode<T>>): Map<string, TreeNode<T>> {
    for (const n of nodes) {
        into.set(n.id, n);
        if (n.children) indexNodes(n.children, into);
    }
    return into;
}

function loadExpanded(treeId: string): Set<Key> {
    try {
        return new Set(JSON.parse(localStorage.getItem(`alexandria.tree.${treeId}`) ?? "[]") as string[]);
    } catch {
        return new Set();
    }
}

export function Tree<T>({ treeId, nodes, selectedId, onSelect, renderDecoration, ...aria }: TreeProps<T>) {
    const [expanded, setExpanded] = useState<Set<Key>>(() => loadExpanded(treeId));

    const onExpandedChange = useCallback(
        (keys: Set<Key>) => {
            setExpanded(keys);
            localStorage.setItem(`alexandria.tree.${treeId}`, JSON.stringify([...keys]));
        },
        [treeId],
    );

    const byId = indexNodes(nodes, new Map<string, TreeNode<T>>());

    const onSelectionChange = (sel: Selection) => {
        if (sel === "all") return;
        const id = [...sel][0];
        const node = id != null ? byId.get(String(id)) : undefined;
        if (node) onSelect(node);
    };

    const renderItem = (node: TreeNode<T>): ReactNode => (
        <AriaTreeItem id={node.id} textValue={node.label} className={s.item}>
            <AriaTreeItemContent>
                {({ level }) => (
                    <div className={s.row} style={{ "--level": level } as React.CSSProperties}>
                        <AriaButton slot="chevron" className={s.chevron} data-leaf={!node.children?.length || undefined}>
                            <Icon icon={ChevronRight} size={12} />
                        </AriaButton>
                        <span className={s.label}>{node.label}</span>
                        {renderDecoration && <span className={s.decoration}>{renderDecoration(node)}</span>}
                    </div>
                )}
            </AriaTreeItemContent>
            {node.children && <AriaCollection items={node.children}>{renderItem}</AriaCollection>}
        </AriaTreeItem>
    );

    return (
        <AriaTree
            {...aria}
            className={s.tree}
            items={nodes}
            selectionMode="single"
            selectedKeys={selectedId ? [selectedId] : []}
            onSelectionChange={onSelectionChange}
            expandedKeys={expanded}
            onExpandedChange={onExpandedChange}
        >
            {renderItem}
        </AriaTree>
    );
}
