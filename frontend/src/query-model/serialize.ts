// Serialize a query to a stable string — the TanStack Query key. Canonical form
// (recursively sorted object keys) so two queries that mean the same thing key the
// same cache entry regardless of how their objects were assembled (frontend/09:
// "pills and cache key cannot disagree"). Pure.

import type { Arrangement, Query } from "./ast";

function canonicalize(value: unknown): unknown {
    if (Array.isArray(value)) return value.map(canonicalize);
    if (value !== null && typeof value === "object") {
        const sorted: Record<string, unknown> = {};
        for (const key of Object.keys(value as Record<string, unknown>).sort()) {
            sorted[key] = canonicalize((value as Record<string, unknown>)[key]);
        }
        return sorted;
    }
    return value;
}

function stableStringify(value: unknown): string {
    return JSON.stringify(canonicalize(value));
}

/** Stable key for a query + arrangement pair (the grid fetches an ordered window). */
export function serializeQuery(query: Query, arrangement: Arrangement): string {
    return stableStringify({ query, arrangement });
}
