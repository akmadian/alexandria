// Tree — the §12 browser rail: one reusable hierarchical component across Sources / Collections /
// Tags, "icons + indent guides + muted right-aligned counts." DOCKED chrome (flat on surface.panel,
// no §6 shell). Hierarchy is carried by weight + ink (§9) AND by real elbow connectors.
//
// v3 (D37) is ITEMS-DRIVEN: a `nodes` tree in, so the connector guide model can be computed from the
// sibling/last-child structure — the one thing CSS and RAC's flat list can't see. The folder icon IS
// the expand toggle (RAC's `slot="chevron"`) and swaps open/closed on state; the label click sets the
// scope. Filled kind-icons per the §14 tree exception. RAC owns behavior (keyboard, expand, selection).

import { useMemo, type ReactElement, type ReactNode } from "react";
import {
    Button as AriaButton,
    Collection as AriaCollection,
    Tree as AriaTree,
    TreeItem as AriaTreeItem,
    TreeItemContent as AriaTreeItemContent,
    type TreeItemContentRenderProps,
    type TreeProps as AriaTreeProps,
} from "react-aria-components";
import { Badge } from "@/components/badge/badge";
import { Checkbox } from "@/components/checkbox/checkbox";
import { Icon, type IconConcept } from "@/components/icon/icon";
import { formatCount } from "@/lib/format";
import { cx } from "@/lib/cx";
import styles from "./tree.module.css";

/** A hierarchy node. Domain-blind — `features/browser` maps source/collection/tag DTOs onto this. */
export interface TreeNodeData {
    id: string;
    label: ReactNode;
    /** Leading kind icon (folder / collection / tag / source). One glyph per kind. */
    icon?: IconConcept;
    /** Scent count (§13), rendered muted. */
    count?: number;
    children?: TreeNodeData[];
    isDisabled?: boolean;
    /** Typeahead string; defaults to `label` when it's a string. */
    textValue?: string;
}

// A connector segment for one indent column of a row (left→right): a pass-through vertical ("line")
// or an empty gap ("blank") for ancestor levels, and the node's own elbow ("tee" = ├ has a following
// sibling, "end" = └ last child) at its own level.
export type Guide = "line" | "blank" | "tee" | "end";

// computeGuides walks the tree once and returns each node's connector array (length = depth − 1;
// top-level nodes get []). A column is "line" iff the ancestor at that level has a following sibling
// (its parent-vertical passes through this row); the node's own column is "tee"/"end" by last-child.
// Structural (independent of expansion), so it's precomputed and looked up by id at render time.
export function computeGuides(nodes: TreeNodeData[]): Map<string, Guide[]> {
    const map = new Map<string, Guide[]>();
    const walk = (siblings: TreeNodeData[], trail: Guide[], isTopLevel: boolean) => {
        siblings.forEach((node, index) => {
            const hasFollowingSibling = index < siblings.length - 1;
            // Roots don't connect upward; deeper nodes get the ancestor verticals + their own elbow.
            const guides: Guide[] = isTopLevel ? [] : [...trail, hasFollowingSibling ? "tee" : "end"];
            map.set(node.id, guides);
            if (node.children?.length) {
                // The child trail extends by THIS node's vertical: it passes through the children's
                // rows iff this node has a following sibling. Root's own column is dropped (no vertical
                // left of a top-level node), so top-level recursion starts the trail fresh.
                const childTrail: Guide[] = isTopLevel ? [] : [...trail, hasFollowingSibling ? "line" : "blank"];
                walk(node.children, childTrail, false);
            }
        });
    };
    walk(nodes, [], true);
    return map;
}

const GUIDE_CLASS: Record<Guide, string> = {
    line: styles.line,
    blank: styles.blank,
    tee: styles.tee,
    end: styles.end,
};

export interface TreeProps<T extends object> extends Omit<AriaTreeProps<T>, "className" | "items" | "children"> {
    nodes: TreeNodeData[];
    className?: string;
}

// Scope default is single (§15); `multiple` opts into a union scope (checkbox in the selection slot).
export function Tree({ nodes, className, selectionMode = "single", ...props }: TreeProps<TreeNodeData>) {
    const guides = useMemo(() => computeGuides(nodes), [nodes]);

    const renderNode = (node: TreeNodeData): ReactElement => (
        <AriaTreeItem
            id={node.id}
            textValue={node.textValue ?? (typeof node.label === "string" ? node.label : node.id)}
            isDisabled={node.isDisabled}
            className={styles.item}
        >
            <AriaTreeItemContent>
                {(renderProps: TreeItemContentRenderProps) => (
                    <NodeContent node={node} guides={guides.get(node.id) ?? []} {...renderProps} />
                )}
            </AriaTreeItemContent>
            {node.children?.length ? (
                <AriaCollection items={node.children}>{renderNode}</AriaCollection>
            ) : null}
        </AriaTreeItem>
    );

    return (
        <AriaTree {...props} items={nodes} selectionMode={selectionMode} className={cx(styles.tree, className)}>
            {renderNode}
        </AriaTree>
    );
}

function NodeContent({
    node,
    guides,
    hasChildItems,
    selectionMode,
}: { node: TreeNodeData; guides: Guide[] } & TreeItemContentRenderProps) {
    // One glyph per kind — no fill, no open/closed swap. Selection alone is the state that matters, and
    // it's carried by icon stroke-weight + ink (§9), set in CSS. The chevron shows expand.
    return (
        <>
            {guides.map((guide, level) => (
                <span key={level} className={cx(styles.guide, GUIDE_CLASS[guide])} aria-hidden="true" />
            ))}
            {/* Multiple mode leads with the checkbox (the union-scope control); the connectors center on it. */}
            {selectionMode === "multiple" && (
                <Checkbox slot="selection" size="xs" aria-label="Select" className={styles.selection} />
            )}
            {/* The chevron is the explicit expand affordance (rotates on open); RAC's slot toggles.
              * Leaves reserve the gutter so their icons align under branch icons. Clicking the row /
              * label sets the scope. */}
            {hasChildItems ? (
                <AriaButton slot="chevron" className={styles.chevron}>
                    <Icon concept="disclose" className={styles.chevronGlyph} />
                </AriaButton>
            ) : (
                <span className={styles.chevronSpacer} aria-hidden="true" />
            )}
            <span className={styles.icon}>
                {node.icon && <Icon concept={node.icon} className={styles.iconGlyph} />}
            </span>
            <span className={styles.label} title={typeof node.label === "string" ? node.label : undefined}>
                {node.label}
            </span>
            {node.count !== undefined && (
                <span className={styles.count}>
                    <Badge hue="gray" style="tint" size="inline">
                        {formatCount(node.count)}
                    </Badge>
                </span>
            )}
        </>
    );
}
