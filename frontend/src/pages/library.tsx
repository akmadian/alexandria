import { useMemo, useState } from "react";
import { AssetGrid } from "@/components/library/asset-grid";
import { Inspector } from "@/components/library/inspector";
import { Sidebar } from "@/components/library/sidebar";
import { Toolbar, type Filters, type SortKey } from "@/components/library/toolbar";
import type { Asset, AssetFilter, AssetPatch, AssetScope, AssetSort, FileType, ListQuery } from "@/api/contract";
import { collections, sources, tags } from "@/api/mock";
import { useAsset, useAssets, useCatalogSync, usePatchAssets } from "@/api/queries";

const titleFor = (view: string): string => {
    if (view === "all") return "All Assets";
    if (view === "recent") return "Recent Imports";
    if (view === "picks") return "Picks";
    const [kind, id] = view.split(":");
    const from =
        kind === "source" ? sources.find((s) => s.id === id)?.name : kind === "collection" ? collections.find((c) => c.id === id)?.name : tags.find((t) => t.id === id)?.name;
    return from ?? "Assets";
};

// A "view" is where you're looking: sources/tags are filter fields, a collection
// is a scope, and "picks" is a filter preset (docs/frontend-architecture.md §3).
const scopeAndFilterFor = (view: string): { scope: AssetScope; viewFilter: AssetFilter } => {
    const [kind, id] = view.split(":");
    if (kind === "collection") return { scope: { kind: "collection", id }, viewFilter: {} };
    if (kind === "source") return { scope: { kind: "library" }, viewFilter: { sourceIds: [id] } };
    if (kind === "tag") return { scope: { kind: "library" }, viewFilter: { tagIds: [id] } };
    if (view === "picks") return { scope: { kind: "library" }, viewFilter: { flags: ["pick"] } };
    return { scope: { kind: "library" }, viewFilter: {} };
};

const sortFor: Record<SortKey, AssetSort> = {
    "captured-desc": { field: "captured", dir: "desc" },
    "captured-asc": { field: "captured", dir: "asc" },
    "rating-desc": { field: "rating", dir: "desc" },
    "name-asc": { field: "filename", dir: "asc" },
    "size-desc": { field: "size", dir: "desc" },
};

/** Inspector edits arrive as a Partial<Asset>; keep only the triage fields the
 *  AssetPatch envelope carries. */
const toPatch = (p: Partial<Asset>): AssetPatch => {
    const patch: AssetPatch = {};
    if ("rating" in p) patch.rating = p.rating;
    if ("colorLabel" in p) patch.colorLabel = p.colorLabel;
    if ("flag" in p) patch.flag = p.flag;
    if ("note" in p) patch.note = p.note;
    return patch;
};

export const Library = () => {
    useCatalogSync(); // catalog:changed / job:done / source:status → cache invalidation

    const [view, setView] = useState("all");
    const [selectedId, setSelectedId] = useState<string | null>(null);
    const [filters, setFilters] = useState<Filters>({
        search: "",
        fileType: "all",
        minRating: 0,
        sort: "captured-desc",
        density: "comfortable",
    });

    const query = useMemo<ListQuery>(() => {
        const { scope, viewFilter } = scopeAndFilterFor(view);
        const filter: AssetFilter = { ...viewFilter };
        if (filters.search.trim()) filter.searchText = filters.search.trim();
        if (filters.fileType !== "all") filter.fileTypes = [filters.fileType as FileType];
        if (filters.minRating > 0) filter.ratingMin = filters.minRating;
        const isRecent = view === "recent";
        return {
            scope,
            filter,
            sort: isRecent ? { field: "added", dir: "desc" } : sortFor[filters.sort],
            ...(isRecent ? { page: { limit: 12, offset: 0 } } : null),
        };
    }, [view, filters]);

    const { data, isPending } = useAssets(query);
    const rows = data?.items ?? [];

    const { data: selected } = useAsset(selectedId);
    const patchAssets = usePatchAssets();

    const updateSelected = (p: Partial<Asset>) => {
        if (!selectedId) return;
        patchAssets.mutate({ target: { ids: [selectedId] }, patch: toPatch(p) });
    };

    return (
        <div className="flex h-dvh overflow-hidden bg-primary">
            <Sidebar
                active={view}
                onSelect={(k) => {
                    setView(k);
                    setSelectedId(null);
                }}
                total={data?.total ?? 0}
            />

            <main className="flex min-w-0 flex-1 flex-col">
                <Toolbar title={titleFor(view)} count={data?.total ?? 0} filters={filters} onChange={(f) => setFilters((p) => ({ ...p, ...f }))} />
                <div className="min-h-0 flex-1 overflow-y-auto">
                    {isPending ? (
                        <div className="p-5 text-sm text-tertiary">Loading…</div>
                    ) : (
                        <AssetGrid assets={rows} selectedId={selectedId} onSelect={setSelectedId} density={filters.density} />
                    )}
                </div>
            </main>

            <Inspector asset={selected ?? null} onUpdate={updateSelected} onClose={() => setSelectedId(null)} />
        </div>
    );
};
