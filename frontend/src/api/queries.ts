// TanStack read hooks — the only door features use to reach the backend. Keyed by
// the stable serialized query so identical queries share a cache entry.

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { log } from "@/lib/logger";
import type { Arrangement, Query } from "@/query-model/ast";
import { serializeQuery } from "@/query-model/serialize";
import { api } from "./client";
import type { Page } from "./contract";

// ponytail: single wide page for the vertical (64 mock rows). The AG-Grid-style
// windowed block model — fixed-size blocks keyed by (query+arrangement, block)
// via useQueries, fetched on scroll — is the widen step and touches only this hook.
const PAGE: Page = { offset: 0, limit: 500 };

/**
 * The full-asset detail read — the inspector's server state, keyed by id so a
 * revisited subject is a cache hit. `keepPreviousData` holds the outgoing
 * asset's rows on screen during arrow-key navigation (no flicker); `enabled`
 * gates the fetch off while no cursor exists (empty working set).
 */
export function useAsset(id: string | null) {
    return useQuery({
        queryKey: ["asset", id],
        enabled: id !== null,
        placeholderData: keepPreviousData,
        queryFn: async () => {
            if (id === null) throw new Error("useAsset queryFn ran without an id");
            try {
                const detail = await api.getAsset(id);
                log.debug("api: getAsset resolved", { id });
                return detail;
            } catch (error) {
                log.error("api: getAsset failed", { id, error: String(error) });
                throw error;
            }
        },
    });
}

export function useQueryAssets(query: Query, arrangement: Arrangement) {
    return useQuery({
        queryKey: ["assets", serializeQuery(query, arrangement)],
        queryFn: async () => {
            try {
                const result = await api.queryAssets(query, arrangement, PAGE);
                log.info("api: queryAssets resolved", { total: result.total, returned: result.items.length });
                if (result.total > result.items.length) {
                    // Real catalogs exceed the single-page cap TODAY: rows past
                    // PAGE.limit render as permanent placeholder mats. Loud, not
                    // silent (the UI never pretends). TRIGGER: the block-model widen.
                    log.warn("api: page cap truncates the working set", {
                        total: result.total,
                        loaded: result.items.length,
                    });
                }
                return result;
            } catch (error) {
                log.error("api: queryAssets failed", { error: String(error) });
                throw error;
            }
        },
    });
}
