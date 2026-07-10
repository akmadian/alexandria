// The query AST — the spine (C6). Typed structs, never stringly maps: the filter
// bar renders it, smart collections persist it, NL parses into it, the backend
// compiles it to SQL. `field`/`cmp` are the GENERATED unions (C13); this module
// only composes and validates trees — it never evaluates them (that's SQL on the
// real backend, the mock query engine in isolation) and does no I/O.

import type { TokenField, TokenOperator } from "@/_generated-types/vocabulary";

/** Persisted/compiled query: an extensional scope narrowed by a predicate tree. */
export interface Query {
    version: 1;
    scope: Scope;
    where: WhereNode | null;
}

// Scope = WHERE you're looking (an extensional set), set from the sidebar and
// durable (C1). Distinct from the filter predicate. `tag` is a scope (subtree
// navigation) as well as a filter token (seam/01 ledger #2).
export type Scope =
    | { kind: "library" }
    | { kind: "collection"; id: string }
    | { kind: "folder"; sourceId: string; path: string; recursive?: boolean }
    | { kind: "tag"; id: string };

export type WhereNode = GroupNode | Leaf;

/** Boolean composition. `not` normalizes to negated leaf operators where it can
 *  (assembler, widen phase); it survives over groups and non-negatable leaves. */
export interface GroupNode {
    op: "and" | "or" | "not";
    children: WhereNode[];
}

/** One predicate — the thing a pill renders (C1). `value` is loose at the wire;
 *  strictness lives in the registry constructors + validate() gate. */
export interface Leaf {
    field: TokenField;
    cmp: TokenOperator;
    value: unknown;
}

export function isLeaf(node: WhereNode): node is Leaf {
    return "field" in node;
}

// Arrangement: order + sectioning, never membership (C4). groupBy is shaped but
// unimplemented backend-side (impl/13); default is captured-desc.
export type SortField = "capturedAt" | "ingestedAt" | "rating" | "filename";
export type SortDir = "asc" | "desc";
export type GroupKey = "capturedDay" | "capturedMonth" | "source" | "fileType";

export interface Arrangement {
    sortField: SortField;
    sortDir: SortDir;
    groupBy: GroupKey | null;
}

export const DEFAULT_ARRANGEMENT: Arrangement = { sortField: "capturedAt", sortDir: "desc", groupBy: null };

export interface Page {
    limit: number;
    offset: number;
}
