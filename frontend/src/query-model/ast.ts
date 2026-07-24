// The query AST — the spine (C6). Typed structs, never stringly maps: the filter
// bar renders it, smart collections persist it, NL parses into it, the backend
// compiles it to SQL. Every union here is GENERATED (C13/C15) — field/cmp,
// scope kinds, group ops, sort fields; this module adds only the per-kind
// payload shapes and composes/validates trees. It never evaluates them (that's
// SQL on the real backend, the mock query engine in isolation) and does no I/O.

import type { GroupOp, ScopeKind, SortDir, SortField, TokenField, TokenOperator } from "@/_generated-types/vocabulary";

export type { GroupOp, ScopeKind, SortDir, SortField };

/** Persisted/compiled query: an extensional scope narrowed by a predicate tree. */
export interface Query {
    version: 1;
    scope: Scope;
    where: WhereNode | null;
}

// Scope = WHERE you're looking (an extensional set), set from the sidebar and
// durable (C1). Distinct from the filter predicate. `tag` is a scope (subtree
// navigation) as well as a filter token (seam/01 ledger #2). The payload table
// is keyed by the GENERATED ScopeKind (C10 completeness): a new kind in Go
// fails to compile here until its payload is declared.
interface ScopePayloads {
    library: object;
    collection: { id: string };
    folder: { volumeId: string; path: string; recursive?: boolean };
    tag: { id: string };
}
type ScopePayloadsComplete = ScopePayloads extends Record<ScopeKind, object> ? ScopePayloads : never;

export type Scope = { [K in ScopeKind]: { kind: K } & ScopePayloadsComplete[K] }[ScopeKind];

export type WhereNode = GroupNode | Leaf;

/** Boolean composition. `not` normalizes to negated leaf operators where it can
 *  (assembler, widen phase); it survives over groups and non-negatable leaves. */
export interface GroupNode {
    op: GroupOp;
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

// Arrangement: order + sectioning, never membership (C4). SortField/SortDir
// are generated (SortField reuses TokenField spellings where a token exists;
// `size` is sort-only). groupBy is frontend-only for now — shaped but
// unimplemented backend-side (impl/13) — so GroupKey stays hand-authored
// until the backend grows grouping.
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
