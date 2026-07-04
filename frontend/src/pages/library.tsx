import { useMemo, useState } from "react";
import { AssetGrid } from "@/components/library/asset-grid";
import { Inspector } from "@/components/library/inspector";
import { Sidebar } from "@/components/library/sidebar";
import { Toolbar, type Filters } from "@/components/library/toolbar";
import { assets as seedAssets, collections, sources, tags, type Asset } from "@/lib/mock";

const titleFor = (view: string): string => {
    if (view === "all") return "All Assets";
    if (view === "recent") return "Recent Imports";
    if (view === "picks") return "Picks";
    const [kind, id] = view.split(":");
    const from =
        kind === "source" ? sources.find((s) => s.id === id)?.name : kind === "collection" ? collections.find((c) => c.id === id)?.name : tags.find((t) => t.id === id)?.name;
    return from ?? "Assets";
};

const matchesView = (a: Asset, view: string): boolean => {
    if (view === "all" || view === "recent") return true;
    if (view === "picks") return a.flag === "pick";
    const [kind, id] = view.split(":");
    if (kind === "source") return a.sourceId === id;
    if (kind === "tag") return a.tagIds.includes(id);
    if (kind === "collection") {
        if (id === "col-5star") return a.rating === 5;
        if (id === "col-untagged") return a.tagIds.length === 0;
        // Manual collections have no membership table in the mock — fake a stable subset.
        return (a.id.charCodeAt(a.id.length - 1) + id.length) % 2 === 0;
    }
    return true;
};

const sorters: Record<Filters["sort"], (a: Asset, b: Asset) => number> = {
    "captured-desc": (a, b) => b.capturedAt.localeCompare(a.capturedAt),
    "captured-asc": (a, b) => a.capturedAt.localeCompare(b.capturedAt),
    "rating-desc": (a, b) => b.rating - a.rating || b.capturedAt.localeCompare(a.capturedAt),
    "name-asc": (a, b) => a.filename.localeCompare(b.filename),
    "size-desc": (a, b) => b.sizeBytes - a.sizeBytes,
};

export const Library = () => {
    const [assets, setAssets] = useState<Asset[]>(seedAssets);
    const [view, setView] = useState("all");
    const [selectedId, setSelectedId] = useState<string | null>(null);
    const [filters, setFilters] = useState<Filters>({
        search: "",
        fileType: "all",
        minRating: 0,
        sort: "captured-desc",
        density: "comfortable",
    });

    const visible = useMemo(() => {
        const q = filters.search.trim().toLowerCase();
        const list = assets.filter((a) => {
            if (!matchesView(a, view)) return false;
            if (filters.fileType !== "all" && a.fileType !== filters.fileType) return false;
            if (a.rating < filters.minRating) return false;
            if (q) {
                const hay = `${a.filename} ${a.cameraModel ?? ""} ${a.lensModel ?? ""} ${a.location ?? ""}`.toLowerCase();
                if (!hay.includes(q)) return false;
            }
            return true;
        });
        list.sort(sorters[filters.sort]);
        return view === "recent" ? list.slice(0, 12) : list;
    }, [assets, view, filters]);

    const selected = assets.find((a) => a.id === selectedId) ?? null;

    const updateSelected = (patch: Partial<Asset>) =>
        setAssets((prev) => prev.map((a) => (a.id === selectedId ? { ...a, ...patch } : a)));

    return (
        <div className="flex h-dvh overflow-hidden bg-primary">
            <Sidebar
                active={view}
                onSelect={(k) => {
                    setView(k);
                    setSelectedId(null);
                }}
                total={assets.length}
            />

            <main className="flex min-w-0 flex-1 flex-col">
                <Toolbar title={titleFor(view)} count={visible.length} filters={filters} onChange={(f) => setFilters((p) => ({ ...p, ...f }))} />
                <div className="min-h-0 flex-1 overflow-y-auto">
                    <AssetGrid assets={visible} selectedId={selectedId} onSelect={setSelectedId} density={filters.density} />
                </div>
            </main>

            <Inspector asset={selected} onUpdate={updateSelected} onClose={() => setSelectedId(null)} />
        </div>
    );
};
