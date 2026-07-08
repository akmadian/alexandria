// The query layer (docs/project-tracking/seam/01-queries-and-commands.md). Everything the UI needs from
// the backend flows through these hooks — components never call `api` directly.
//
// TanStack Query provides the machinery the doc used to hand-roll: single-flight
// dedupe, stale-while-revalidate, LRU (gcTime), request cancellation, and
// query-key-based superseding. What stays bespoke lives elsewhere (thumbnail
// loader). Here we add: query-key conventions, the catalog:changed → invalidate
// bridge, and optimistic triage (§10).

import {
    useMutation,
    useQuery,
    useQueryClient,
    type QueryClient,
} from "@tanstack/react-query";
import { useEffect } from "react";
import { createMockApi } from "./mock-api.ts";
// import { createWailsApi } from "./wails-api.ts"; // ← the one-line swap when the Go backend binds
import type {
    Asset,
    AssetPatch,
    AssetRow,
    ListAssetsResult,
    ListQuery,
    PatchTarget,
} from "./contract.ts";

// The live backend. Components consume the hooks below, never `api` directly for
// queries/mutations. The one sanctioned direct use is event subscription
// (onJobProgress/onJobDone/…), which isn't request/response and has no hook —
// features/jobs/use-jobs.ts imports it for exactly that.
// Swap createMockApi() → createWailsApi() to go real; nothing below changes.
export const api = createMockApi();

// --- Query keys. Reference data is stable; lists are keyed by the whole query. ---
export const keys = {
    assets: (q: ListQuery) => ["assets", q] as const,
    asset: (id: string) => ["asset", id] as const,
    sources: ["sources"] as const,
    collections: ["collections"] as const,
    tags: ["tags"] as const,
    folderTree: (sourceId: string) => ["folderTree", sourceId] as const,
    settings: ["settings"] as const,
    keybindings: ["keybindings"] as const,
};

// Reference data changes rarely and only via catalog:changed — never re-fetch on
// mount or window focus.
const REFERENCE = { staleTime: Infinity } as const;

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

export function useAssets(query: ListQuery) {
    return useQuery({
        queryKey: keys.assets(query),
        queryFn: () => api.listAssets(query),
        // Keep the previous page visible while the next loads (no fl/ on scroll).
        placeholderData: (prev) => prev,
    });
}

export function useAsset(id: string | null) {
    return useQuery({
        queryKey: keys.asset(id ?? "none"),
        queryFn: () => api.getAsset(id!),
        enabled: id != null,
    });
}

export const useSources = () => useQuery({ queryKey: keys.sources, queryFn: () => api.listSources(), ...REFERENCE });
export const useCollections = () => useQuery({ queryKey: keys.collections, queryFn: () => api.listCollections(), ...REFERENCE });
export const useTags = () => useQuery({ queryKey: keys.tags, queryFn: () => api.tagTree(), ...REFERENCE });
export const useFolderTree = (sourceId: string | null) =>
    useQuery({ queryKey: keys.folderTree(sourceId ?? "none"), queryFn: () => api.getFolderTree(sourceId!), enabled: sourceId != null, ...REFERENCE });

export const useSettings = () => useQuery({ queryKey: keys.settings, queryFn: () => api.getSettings(), ...REFERENCE });
export const useKeybindings = () => useQuery({ queryKey: keys.keybindings, queryFn: () => api.listKeybindings(), ...REFERENCE });

// ---------------------------------------------------------------------------
// Optimistic triage (§10) — the one place we lie to the UI for latency's sake.
// Rating / flag / label reflect instantly in both the grid rows and the detail;
// on error we roll back; catalog:changed reconciles either way.
// ---------------------------------------------------------------------------

/** Apply an AssetPatch to a slim row (mirrors the backend's field semantics). */
function patchRow(row: AssetRow, patch: AssetPatch): AssetRow {
    return {
        ...row,
        ...("rating" in patch ? { rating: patch.rating ?? 0 } : null),
        ...("colorLabel" in patch ? { colorLabel: patch.colorLabel ?? null } : null),
        ...("flag" in patch ? { flag: patch.flag ?? null } : null),
    };
}

function patchAssetDetail(a: Asset, patch: AssetPatch): Asset {
    return {
        ...a,
        ...("rating" in patch ? { rating: patch.rating ?? 0 } : null),
        ...("colorLabel" in patch ? { colorLabel: patch.colorLabel ?? null } : null),
        ...("flag" in patch ? { flag: patch.flag ?? null } : null),
        ...("note" in patch ? { note: patch.note ?? null } : null),
    };
}

function applyOptimistic(qc: QueryClient, ids: Set<string>, patch: AssetPatch) {
    // Every cached list page.
    qc.setQueriesData<ListAssetsResult>({ queryKey: ["assets"] }, (prev) =>
        prev ? { ...prev, items: prev.items.map((r) => (ids.has(r.id) ? patchRow(r, patch) : r)) } : prev,
    );
    // Every cached detail.
    for (const id of ids) {
        qc.setQueryData<Asset | null>(keys.asset(id), (prev) => (prev ? patchAssetDetail(prev, patch) : prev));
    }
}

type TriageVars = { target: PatchTarget; patch: AssetPatch };

/** Triage mutation with optimistic rows + rollback. Only meaningful for {ids}
 *  targets (single/multi select); query-targets fall back to invalidate-on-settle. */
export function usePatchAssets() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: ({ target, patch }: TriageVars) => api.patchAssets(target, patch),
        onMutate: async ({ target, patch }) => {
            if (!("ids" in target)) return { snapshot: null };
            await qc.cancelQueries({ queryKey: ["assets"] });
            const ids = new Set(target.ids);
            const snapshot = qc.getQueriesData<ListAssetsResult>({ queryKey: ["assets"] });
            const details = target.ids.map((id) => [id, qc.getQueryData<Asset | null>(keys.asset(id))] as const);
            applyOptimistic(qc, ids, patch);
            return { snapshot, details };
        },
        onError: (_err, _vars, ctx) => {
            // Roll back to the pre-mutation snapshots.
            ctx?.snapshot?.forEach(([key, data]) => qc.setQueryData(key, data));
            ctx?.details?.forEach(([id, data]) => qc.setQueryData(keys.asset(id), data));
        },
        // Success path leaves the optimistic values in place; catalog:changed
        // (fired by the mock/backend) invalidates and reconciles against truth.
    });
}

// ---------------------------------------------------------------------------
// Non-triage mutations — pessimistic (invalidate on settle). Cheap and correct.
// ---------------------------------------------------------------------------

export function useSetAssetTags() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: ({ assetId, tagIds }: { assetId: string; tagIds: string[] }) => api.setAssetTags(assetId, tagIds),
        onSuccess: (_r, { assetId }) => {
            qc.invalidateQueries({ queryKey: keys.asset(assetId) });
            qc.invalidateQueries({ queryKey: keys.tags });
        },
    });
}

export function useStartImport() {
    return useMutation({ mutationFn: (sourceId: string) => api.startImport(sourceId) });
}

// ---------------------------------------------------------------------------
// The event bridge — mount once at the app root. Turns backend push events into
// cache invalidations and keeps optimistic writes honest.
// ---------------------------------------------------------------------------

export function useCatalogSync() {
    const qc = useQueryClient();
    useEffect(() => {
        const offCatalog = api.onCatalogChanged((change) => {
            // Coarse by default: any catalog change re-validates lists + details.
            // Scope hint lets us skip the untouched reference queries.
            if (!change.scope || change.scope === "assets") {
                qc.invalidateQueries({ queryKey: ["assets"] });
                qc.invalidateQueries({ queryKey: ["asset"] });
            }
            if (!change.scope || change.scope === "sources") qc.invalidateQueries({ queryKey: keys.sources });
            if (!change.scope || change.scope === "collections") qc.invalidateQueries({ queryKey: keys.collections });
            if (!change.scope || change.scope === "tags") qc.invalidateQueries({ queryKey: keys.tags });
        });
        const offSource = api.onSourceStatus(() => qc.invalidateQueries({ queryKey: keys.sources }));
        const offJobDone = api.onJobDone(() => qc.invalidateQueries({ queryKey: ["assets"] }));
        return () => {
            offCatalog();
            offSource();
            offJobDone();
        };
    }, [qc]);
}
