// Optimistic cache patching for triage writes (frontend-architecture §Optimistic
// mutation × undo, point 4). Keystroke-speed feedback: patch the TanStack cache
// the instant a write fires, reconcile behind. BOTH caches carry the same asset
// facts, so both get patched — the list cache (`["assets", …]`, one entry per
// live query) and the detail cache (`["asset", id]`). Patches carry ABSOLUTE
// values (never deltas), so applying one is idempotent and the last write for a
// field wins in the cache exactly as it wins at the engine.
//
// The pure patchers (patchRow/patchDetail) are the testable core; the QueryClient
// wrappers below drive them across the two cache shapes and snapshot prior values
// for rollback. A present patch key SETS (a value) or CLEARS (null); an ABSENT key
// (value undefined) is left untouched — the three-state contract, cache-side.

import type { QueryClient } from "@tanstack/react-query";
import type { AssetDetail } from "@/_generated-types/models";
import type { AssetQueryResult, AssetRow, TriagePatch } from "./contract";

/** Apply the present triage fields to a grid row (note lives only on the detail). */
export function patchRow(row: AssetRow, patch: TriagePatch): AssetRow {
    const next = { ...row };
    if (patch.rating !== undefined) next.rating = patch.rating;
    if (patch.colorLabel !== undefined) next.colorLabel = patch.colorLabel;
    if (patch.flag !== undefined) next.flag = patch.flag;
    return next;
}

/** Apply the present triage fields to a detail record (carries the note too). */
export function patchDetail(detail: AssetDetail, patch: TriagePatch): AssetDetail {
    const next = { ...detail };
    if (patch.rating !== undefined) next.rating = patch.rating;
    if (patch.colorLabel !== undefined) next.colorLabel = patch.colorLabel;
    if (patch.flag !== undefined) next.flag = patch.flag;
    if (patch.note !== undefined) next.note = patch.note;
    return next;
}

/** Patch every matching row of one list-cache result; identity-stable when nothing matches. */
function patchResult(result: AssetQueryResult | undefined, ids: ReadonlySet<string>, patch: TriagePatch): AssetQueryResult | undefined {
    if (result === undefined) return result;
    let touched = false;
    const items = result.items.map((row) => {
        if (!ids.has(row.id)) return row;
        touched = true;
        return patchRow(row, patch);
    });
    return touched ? { ...result, items } : result;
}

/** The prior cache state captured before an optimistic patch, replayed on rollback. */
export interface CacheSnapshot {
    lists: [readonly unknown[], AssetQueryResult | undefined][];
    details: [string, AssetDetail | undefined][];
}

const LIST_KEY = ["assets"] as const;
const detailKey = (id: string): readonly unknown[] => ["asset", id];

/**
 * Cancel in-flight reads that our patch would touch (cancel-on-mutate, point 1):
 * a refetch that started before the write must not land ON TOP of the optimistic
 * value. The list cache is the collective `["assets"]` prefix; details are per id.
 */
export async function cancelCatalogReads(queryClient: QueryClient, ids: readonly string[]): Promise<void> {
    await queryClient.cancelQueries({ queryKey: LIST_KEY });
    await Promise.all(ids.map((id) => queryClient.cancelQueries({ queryKey: detailKey(id) })));
}

/**
 * Snapshot both caches, then patch every affected row in place. The returned
 * snapshot restores the exact prior values on failure. Ids-targets only (the
 * frontend never optimistically patches an `all`-shaped write — those invalidate).
 */
export function applyOptimisticPatch(queryClient: QueryClient, ids: readonly string[], patch: TriagePatch): CacheSnapshot {
    const idSet = new Set(ids);
    const lists = queryClient.getQueriesData<AssetQueryResult>({ queryKey: LIST_KEY });
    const details: [string, AssetDetail | undefined][] = ids.map((id) => [id, queryClient.getQueryData<AssetDetail>(detailKey(id))]);

    queryClient.setQueriesData<AssetQueryResult>({ queryKey: LIST_KEY }, (result) => patchResult(result, idSet, patch));
    for (const id of ids) {
        queryClient.setQueryData<AssetDetail>(detailKey(id), (detail) => (detail === undefined ? detail : patchDetail(detail, patch)));
    }
    return { lists, details };
}

/** Restore the pre-patch cache values (rollback after a failed write). */
export function rollbackCatalog(queryClient: QueryClient, snapshot: CacheSnapshot): void {
    for (const [key, data] of snapshot.lists) queryClient.setQueryData(key, data);
    for (const [id, data] of snapshot.details) queryClient.setQueryData(detailKey(id), data);
}

/** Invalidate both caches so the reconciling refetch pulls engine truth (point 1's gate). */
export async function invalidateCatalog(queryClient: QueryClient): Promise<void> {
    await queryClient.invalidateQueries({ queryKey: LIST_KEY });
    await queryClient.invalidateQueries({ queryKey: ["asset"] });
}
