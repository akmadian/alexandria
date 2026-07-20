// TanStack read hooks — the only door features use to reach the backend. Keyed by
// the stable serialized query so identical queries share a cache entry.

import { useQuery } from "@tanstack/react-query";
import { log } from "@/lib/logger";
import type { Arrangement, Query } from "@/query-model/ast";
import { serializeQuery } from "@/query-model/serialize";
import { api } from "./client";
import type { Page } from "./contract";

// ponytail: single wide page for the vertical (64 mock rows). The AG-Grid-style
// windowed block model — fixed-size blocks keyed by (query+arrangement, block)
// via useQueries, fetched on scroll — is the widen step and touches only this hook.
const PAGE: Page = { offset: 0, limit: 500 };

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
