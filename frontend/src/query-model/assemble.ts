// Filter assembly — pure tree edits behind the flat pill row. The bar shows the
// TOP-LEVEL AND's children as pills (frontend/03, 09), so the top-level filter is
// only ever null · a single leaf · an AND of leaves. These helpers keep that
// canonical, so the pills and the serialized cache key can't disagree. No I/O, no
// React. Nested groups (the advanced editor) are a widen concern.

import { type Leaf, type WhereNode, isLeaf } from "./ast";

/** The leaves the flat pill row renders. A top-level non-AND group has no flat
 *  representation yet — it renders as a single group pill later (widen). */
export function topLevelLeaves(filter: WhereNode | null): Leaf[] {
    if (filter === null) return [];
    if (isLeaf(filter)) return [filter];
    if (filter.op === "and") return filter.children.filter(isLeaf);
    return [];
}

/** Rebuild the canonical top-level shape from a flat leaf list. */
function fromLeaves(leaves: Leaf[]): WhereNode | null {
    if (leaves.length === 0) return null;
    if (leaves.length === 1) return leaves[0];
    return { op: "and", children: leaves };
}

/** Append a predicate to the top-level AND (creating/widening it as needed). */
export function addLeaf(filter: WhereNode | null, leaf: Leaf): WhereNode {
    if (filter === null) return leaf;
    if (isLeaf(filter)) return { op: "and", children: [filter, leaf] };
    if (filter.op === "and") return { op: "and", children: [...filter.children, leaf] };
    // A top-level or/not: wrap it alongside the new leaf rather than mutate it.
    return { op: "and", children: [filter, leaf] };
}

/** Remove the pill at `index` in the flat row; collapses back to leaf/null. */
export function removeLeaf(filter: WhereNode | null, index: number): WhereNode | null {
    return fromLeaves(topLevelLeaves(filter).filter((_, i) => i !== index));
}

/** Replace the pill at `index` (operator or value edit). */
export function replaceLeaf(filter: WhereNode | null, index: number, leaf: Leaf): WhereNode | null {
    return fromLeaves(topLevelLeaves(filter).map((existing, i) => (i === index ? leaf : existing)));
}
