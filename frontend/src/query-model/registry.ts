// The token registry — the dictionary keyed by field (C1/C10). Seeded from the
// GENERATED `fieldGrammar` (the shared Go↔TS spine), so completeness is inherited
// from the generated `satisfies Record<TokenField, FieldGrammar>` — we never
// hand-maintain a parallel field list. Parse/format/narration + per-kind editors
// arrive with the filter bar (widen); this module owns construction + validation.

import { type TokenField, type TokenOperator, type ValueKind, fieldGrammar } from "@/_generated-types/vocabulary";
import type { Leaf } from "./ast";

export interface Token {
    field: TokenField;
    kind: ValueKind;
    operators: readonly TokenOperator[];
}

function buildTokens(): Record<TokenField, Token> {
    const out = {} as Record<TokenField, Token>;
    for (const field of Object.keys(fieldGrammar) as TokenField[]) {
        const grammar = fieldGrammar[field];
        out[field] = { field, kind: grammar.kind, operators: grammar.operators };
    }
    return out;
}

/** The registry. `tokens[field]` is the definition for that filterable dimension. */
export const tokens: Record<TokenField, Token> = buildTokens();

/** Construct a predicate leaf. Loose by design at the wire; `validate` is the gate. */
export function leaf(field: TokenField, cmp: TokenOperator, value: unknown): Leaf {
    return { field, cmp, value };
}

// Absence is an operator on the base token ("unrated" → rating empty), so
// empty/notEmpty carry no value — the filter bar renders no value segment for them.
export function valuelessOperator(cmp: TokenOperator): boolean {
    return cmp === "empty" || cmp === "notEmpty";
}

function isStringArray(value: unknown): value is string[] {
    return Array.isArray(value) && value.every((v) => typeof v === "string");
}

// A date value normalizes to { anchor: ISODate | "now", duration } — half-open
// interval (frontend/09). Light shape check here; full grammar arrives with the
// date editor (widen).
function isDateRange(value: unknown): boolean {
    return typeof value === "object" && value !== null && "anchor" in value && "duration" in value;
}

function valueValidForKind(kind: ValueKind, cmp: TokenOperator, value: unknown): boolean {
    if (valuelessOperator(cmp)) return true;
    const membership = cmp === "in" || cmp === "notIn";
    switch (kind) {
        case "numeric":
            return typeof value === "number" && Number.isFinite(value);
        case "enum":
            return membership ? isStringArray(value) && value.length > 0 : typeof value === "string";
        case "text":
        case "freeText":
            return typeof value === "string" && value.length > 0;
        case "tagReference":
        case "entityReference":
            return membership ? isStringArray(value) : typeof value === "string";
        case "dateRange":
            return isDateRange(value);
    }
}

/**
 * The persistence-boundary gate (frontend/09): every leaf entering a persisted
 * tree passes here. Unknown field or an operator/value the field doesn't allow →
 * false (the caller renders an inert "unknown token" pill; never a crash or a
 * silently dropped predicate — the D20-grade trust rule).
 */
export function validate(node: Leaf): boolean {
    const token = tokens[node.field];
    if (!token) return false;
    if (!token.operators.includes(node.cmp)) return false;
    return valueValidForKind(token.kind, node.cmp, node.value);
}
