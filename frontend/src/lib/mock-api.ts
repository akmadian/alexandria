// In-memory implementation of AlexandriaAPI over the seed data. This is the test
// double the whole frontend is built against until the Wails backend exists;
// createMockApi() → createWailsApi() is the only swap.
//
// The pure resolution helpers (scope, filter, sort, folder-tree) are exported so
// mock-api.check.ts can assert them without React or a test framework.

import {
    assets as seedAssets,
    collections as seedCollections,
    sources as seedSources,
    tags as seedTags,
    thumbUrl,
    type Asset,
    type Collection,
    type Source,
    type Tag,
} from "./mock.ts";
import type {
    AlexandriaAPI,
    AssetFilter,
    AssetRow,
    AssetScope,
    AssetSort,
    CatalogChange,
    CollectionInput,
    CollectionPatch,
    FolderNode,
    HistoryState,
    JobDone,
    JobProgress,
    Keybinding,
    KeybindingContext,
    ListQuery,
    PatchTarget,
    Settings,
    SettingsPatch,
    SourceInput,
    SourcePatch,
    SourceStatusEvent,
    TagInput,
    TagPatch,
    Unsubscribe,
    UpdateAvailable,
    AssetPatch,
} from "./api.ts";
import { ApiError } from "./api.ts";

// --- Smart collection predicates. Manual collections use an explicit membership set. ---
const smartPredicate: Record<string, (a: Asset) => boolean> = {
    "col-5star": (a) => a.rating === 5,
    "col-untagged": (a) => a.tagIds.length === 0,
};

// ---------------------------------------------------------------------------
// Pure resolution helpers (exported for the check)
// ---------------------------------------------------------------------------

/** Directory portion of a relative path, "/"-normalized, no trailing slash. */
export function dirOf(relativePath: string): string {
    const i = relativePath.lastIndexOf("/");
    return i < 0 ? "" : relativePath.slice(0, i);
}

export function inScope(scope: AssetScope, a: Asset, isMember: (colId: string, a: Asset) => boolean): boolean {
    switch (scope.kind) {
        case "library":
            return true;
        case "collection":
            return isMember(scope.id, a);
        case "folder": {
            if (a.sourceId !== scope.sourceId) return false;
            const dir = dirOf(a.relativePath);
            if (scope.path === "") return scope.recursive === false ? dir === "" : true;
            if (scope.recursive === false) return dir === scope.path;
            return dir === scope.path || dir.startsWith(scope.path + "/");
        }
    }
}

export function matchesFilter(a: Asset, f: AssetFilter): boolean {
    if (f.fileTypes?.length && !f.fileTypes.includes(a.fileType)) return false;
    if (f.ratingExact != null && a.rating !== f.ratingExact) return false;
    if (f.ratingMin != null && a.rating < f.ratingMin) return false;
    if (f.colorLabels?.length && (!a.colorLabel || !f.colorLabels.includes(a.colorLabel))) return false;
    if (f.flags?.length && (!a.flag || !f.flags.includes(a.flag))) return false;
    if (f.tagIds?.length && !f.tagIds.some((t) => a.tagIds.includes(t))) return false;
    if (f.sourceIds?.length && !f.sourceIds.includes(a.sourceId)) return false;
    if (f.fileStatus?.length && !f.fileStatus.includes(a.fileStatus)) return false;
    if (f.capturedAfter && a.capturedAt < f.capturedAfter) return false;
    if (f.capturedBefore && a.capturedAt > f.capturedBefore) return false;
    if (f.unrated && a.rating !== 0) return false;
    if (f.unflagged && a.flag !== null) return false;
    if (f.unlabeled && a.colorLabel !== null) return false;
    if (f.untagged && a.tagIds.length > 0) return false;
    if (f.searchText) {
        const q = f.searchText.trim().toLowerCase();
        const hay = `${a.filename} ${a.cameraModel ?? ""} ${a.lensModel ?? ""} ${a.location ?? ""} ${a.note ?? ""}`.toLowerCase();
        if (!hay.includes(q)) return false;
    }
    return true;
}

const keyOf: Record<AssetSort["field"], (a: Asset) => number | string> = {
    captured: (a) => a.capturedAt,
    added: (a) => a.ingestedAt,
    rating: (a) => a.rating,
    filename: (a) => a.filename,
    size: (a) => a.sizeBytes,
};

/** Sort with `id` as the final tie-breaker — non-negotiable for stable paging (§5). */
export function sortAssets(list: Asset[], sort?: AssetSort): Asset[] {
    const s = sort ?? { field: "captured" as const, dir: "desc" as const };
    const dir = s.dir === "asc" ? 1 : -1;
    const get = keyOf[s.field];
    return [...list].sort((a, b) => {
        const ka = get(a);
        const kb = get(b);
        if (ka < kb) return -dir;
        if (ka > kb) return dir;
        return a.id < b.id ? -1 : a.id > b.id ? 1 : 0; // deterministic tie-break
    });
}

/** Build the derived folder tree from a source's assets. Counts scale with distinct
 *  directories, not assets. Root node has name "" and path "". */
export function buildFolderTree(assetsForSource: Asset[]): FolderNode {
    const root: FolderNode = { name: "", path: "", directCount: 0, totalCount: 0, children: [] };
    const child = (parent: FolderNode, name: string): FolderNode => {
        let c = parent.children.find((x) => x.name === name);
        if (!c) {
            c = { name, path: parent.path ? `${parent.path}/${name}` : name, directCount: 0, totalCount: 0, children: [] };
            parent.children.push(c);
        }
        return c;
    };
    for (const a of assetsForSource) {
        const dir = dirOf(a.relativePath);
        const segments = dir === "" ? [] : dir.split("/");
        let node = root;
        node.totalCount++;
        for (const seg of segments) {
            node = child(node, seg);
            node.totalCount++;
        }
        node.directCount++;
    }
    const sortRec = (n: FolderNode) => {
        n.children.sort((a, b) => a.name.localeCompare(b.name));
        n.children.forEach(sortRec);
    };
    sortRec(root);
    return root;
}

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

export function createMockApi(): AlexandriaAPI {
    const store = new Map(seedAssets.map((a) => [a.id, { ...a }]));
    const deleted = new Set<string>();
    const sources: Source[] = seedSources.map((s) => ({ ...s }));
    const collections: Collection[] = seedCollections.map((c) => ({ ...c }));
    const tags: Tag[] = seedTags.map((t) => ({ ...t }));

    // Manual collection membership, materialized from the old stable-subset rule.
    const membership = new Map<string, Set<string>>();
    for (const c of collections) {
        if (c.id in smartPredicate) continue;
        const set = new Set<string>();
        for (const a of store.values()) {
            if ((a.id.charCodeAt(a.id.length - 1) + c.id.length) % 2 === 0) set.add(a.id);
        }
        membership.set(c.id, set);
    }
    const isMember = (colId: string, a: Asset): boolean => {
        const pred = smartPredicate[colId];
        if (pred) return pred(a);
        return membership.get(colId)?.has(a.id) ?? false;
    };

    let settings: Settings = {
        xmpConflictResolution: "xmp_wins",
        duplicateHandling: "review",
        thumbnailQuality: 85,
        defaultSortField: "captured",
        defaultSortDir: "desc",
        undoStackSize: 50,
        catalogBackupCount: 10,
        updateCheckEnabled: true,
    };
    let keybindings: Keybinding[] = seedKeybindings();

    // --- events ---
    type Bus<T> = Set<(v: T) => void>;
    const catalogBus: Bus<CatalogChange> = new Set();
    const jobProgressBus: Bus<JobProgress> = new Set();
    const jobDoneBus: Bus<JobDone> = new Set();
    const sourceStatusBus: Bus<SourceStatusEvent> = new Set();
    const historyBus: Bus<HistoryState> = new Set();
    const updateBus: Bus<UpdateAvailable> = new Set();
    const sub = <T>(bus: Bus<T>, cb: (v: T) => void): Unsubscribe => (bus.add(cb), () => void bus.delete(cb));
    const emit = <T>(bus: Bus<T>, v: T) => bus.forEach((cb) => cb(v));

    // --- undo stack ---
    type Command = { label: string; undo: () => void; redo: () => void };
    const undoStack: Command[] = [];
    const redoStack: Command[] = [];
    const historyState = (): HistoryState => ({
        canUndo: undoStack.length > 0,
        canRedo: redoStack.length > 0,
        undoLabel: undoStack.at(-1)?.label,
        redoLabel: redoStack.at(-1)?.label,
    });
    const record = (cmd: Command) => {
        cmd.redo();
        undoStack.push(cmd);
        if (undoStack.length > settings.undoStackSize) undoStack.shift();
        redoStack.length = 0;
        emit(historyBus, historyState());
    };

    // --- helpers over the live store ---
    const live = () => [...store.values()].filter((a) => !deleted.has(a.id));
    const resolveTarget = (t: PatchTarget): string[] => {
        if ("ids" in t) return t.ids;
        return live()
            .filter((a) => inScope(t.scope, a, isMember) && matchesFilter(a, t.filter ?? {}))
            .map((a) => a.id);
    };
    const snapshot = (ids: string[]) => ids.map((id) => ({ ...store.get(id)! })).filter((a) => a.id);
    const applyPatch = (ids: string[], patch: AssetPatch) => {
        for (const id of ids) {
            const a = store.get(id);
            if (!a) continue;
            const next = { ...a };
            if ("rating" in patch) next.rating = patch.rating ?? 0;
            if ("colorLabel" in patch) next.colorLabel = patch.colorLabel ?? null;
            if ("flag" in patch) next.flag = patch.flag ?? null;
            if ("note" in patch) next.note = patch.note ?? null;
            store.set(id, next);
        }
    };
    const restore = (snaps: Asset[]) => snaps.forEach((s) => store.set(s.id, { ...s }));

    const toRow = (a: Asset): AssetRow => ({
        kind: "asset",
        id: a.id,
        filename: a.filename,
        extension: a.extension,
        fileType: a.fileType,
        fileStatus: a.fileStatus,
        rating: a.rating,
        colorLabel: a.colorLabel,
        flag: a.flag,
        width: a.width,
        height: a.height,
        durationSecs: a.durationSecs,
        capturedAt: a.capturedAt,
        thumbURL: `${thumbUrl(a, 480)}?v=${a.thumbnailAt}`,
    });

    const countIn = (pred: (a: Asset) => boolean) => live().filter(pred).length;
    const refreshCounts = () => {
        for (const s of sources) s.count = countIn((a) => a.sourceId === s.id);
        for (const c of collections) c.count = countIn((a) => isMember(c.id, a));
        for (const t of tags) t.count = countIn((a) => a.tagIds.includes(t.id));
    };

    // --- fake job runner ---
    const jobs = new Map<string, ReturnType<typeof setInterval>>();

    return {
        // ---- Assets ----
        async listAssets(query: ListQuery) {
            const base = live().filter(
                (a) => inScope(query.scope, a, isMember) && matchesFilter(a, query.filter ?? {}),
            );
            const sorted = sortAssets(base, query.sort);
            const offset = query.page?.offset ?? 0;
            const page = query.page ? sorted.slice(offset, offset + query.page.limit) : sorted;
            return { items: page.map(toRow), total: sorted.length };
        },

        async getAsset(id) {
            const a = store.get(id);
            return a && !deleted.has(id) ? { ...a } : null;
        },

        async patchAssets(target, patch) {
            const ids = resolveTarget(target);
            if (ids.length === 0) return;
            const before = snapshot(ids);
            const label = ids.length === 1 ? `Edit — ${store.get(ids[0])?.filename ?? ids[0]}` : `Edit — ${ids.length} assets`;
            record({ label, redo: () => applyPatch(ids, patch), undo: () => restore(before) });
            refreshCounts();
            emit(catalogBus, { scope: "assets", ids });
        },

        async setAssetTags(assetId, tagIds) {
            const a = store.get(assetId);
            if (!a || deleted.has(assetId)) throw new ApiError("domain", `asset ${assetId} not found`, "not_found");
            const before = snapshot([assetId]);
            const next = [...tagIds];
            record({
                label: `Tags — ${a.filename}`,
                redo: () => store.set(assetId, { ...store.get(assetId)!, tagIds: [...next] }),
                undo: () => restore(before),
            });
            refreshCounts();
            emit(catalogBus, { scope: "assets", ids: [assetId] });
        },

        async removeFromCatalog(target) {
            const ids = resolveTarget(target).filter((id) => !deleted.has(id));
            if (ids.length === 0) return;
            record({
                label: `Remove — ${ids.length} asset${ids.length > 1 ? "s" : ""}`,
                redo: () => ids.forEach((id) => deleted.add(id)),
                undo: () => ids.forEach((id) => deleted.delete(id)),
            });
            refreshCounts();
            emit(catalogBus, { scope: "assets", ids });
        },

        async deleteFromDisk(ids) {
            ids.forEach((id) => {
                store.delete(id);
                deleted.delete(id);
            });
            refreshCounts();
            emit(catalogBus, { scope: "assets", ids }); // not undoable — no command recorded
        },

        async openAsset(id) {
            const a = store.get(id);
            if (a?.fileStatus === "offline") throw new ApiError("degraded", "source offline", "source_offline");
            console.info("[mock] open", id);
        },
        async openWith(id, appName) {
            console.info("[mock] open", id, "with", appName);
        },
        async revealInFileManager(id) {
            console.info("[mock] reveal", id);
        },

        // ---- Sources ----
        async listSources() {
            refreshCounts();
            return sources.map((s) => ({ ...s }));
        },
        async createSource(input: SourceInput) {
            const s: Source = { id: `src-${Date.now().toString(36)}`, name: input.name, kind: input.kind, status: "active", count: 0 };
            sources.push(s);
            emit(catalogBus, { scope: "sources" });
            return { ...s };
        },
        async updateSource(id, patch: SourcePatch) {
            const s = sources.find((x) => x.id === id);
            if (!s) throw new ApiError("domain", `source ${id} not found`, "not_found");
            if (patch.name != null) s.name = patch.name;
            if (patch.status != null) {
                s.status = patch.status;
                emit(sourceStatusBus, { sourceId: id, status: patch.status });
            }
            emit(catalogBus, { scope: "sources", ids: [id] });
        },
        async removeSource(id) {
            const i = sources.findIndex((x) => x.id === id);
            if (i >= 0) sources.splice(i, 1);
            emit(catalogBus, { scope: "sources" });
        },
        async pickDirectory() {
            return "/Users/ari/Pictures/Imported"; // native dialog stub
        },
        async getFolderTree(sourceId) {
            return buildFolderTree(live().filter((a) => a.sourceId === sourceId));
        },

        // ---- Tags ----
        async tagTree() {
            refreshCounts();
            return tags.map((t) => ({ ...t }));
        },
        async createTag(input: TagInput) {
            const t: Tag = { id: `t-${Date.now().toString(36)}`, name: input.name, color: input.color ?? "blue", count: 0 };
            tags.push(t);
            emit(catalogBus, { scope: "tags" });
            return { ...t };
        },
        async updateTag(id, patch: TagPatch) {
            const t = tags.find((x) => x.id === id);
            if (!t) throw new ApiError("domain", `tag ${id} not found`, "not_found");
            if (patch.name != null) t.name = patch.name;
            if (patch.color != null) t.color = patch.color;
            emit(catalogBus, { scope: "tags", ids: [id] });
        },
        async deleteTag(id) {
            const i = tags.findIndex((x) => x.id === id);
            if (i >= 0) tags.splice(i, 1);
            for (const a of store.values()) {
                if (a.tagIds.includes(id)) store.set(a.id, { ...a, tagIds: a.tagIds.filter((t) => t !== id) });
            }
            emit(catalogBus, { scope: "tags" });
        },

        // ---- Collections ----
        async listCollections() {
            refreshCounts();
            return collections.map((c) => ({ ...c }));
        },
        async createCollection(input: CollectionInput) {
            const c: Collection = { id: `col-${Date.now().toString(36)}`, name: input.name, kind: input.kind ?? "manual", count: 0 };
            collections.push(c);
            if (c.kind === "manual") membership.set(c.id, new Set());
            emit(catalogBus, { scope: "collections" });
            return { ...c };
        },
        async updateCollection(id, patch: CollectionPatch) {
            const c = collections.find((x) => x.id === id);
            if (!c) throw new ApiError("domain", `collection ${id} not found`, "not_found");
            if (patch.name != null) c.name = patch.name;
            emit(catalogBus, { scope: "collections", ids: [id] });
        },
        async deleteCollection(id) {
            const i = collections.findIndex((x) => x.id === id);
            if (i >= 0) collections.splice(i, 1);
            membership.delete(id);
            emit(catalogBus, { scope: "collections" });
        },
        async addToCollection(collectionId, target) {
            const set = membership.get(collectionId);
            if (!set) throw new ApiError("domain", "not a manual collection", "invalid");
            resolveTarget(target).forEach((id) => set.add(id));
            refreshCounts();
            emit(catalogBus, { scope: "collections", ids: [collectionId] });
        },
        async removeFromCollection(collectionId, target) {
            const set = membership.get(collectionId);
            if (!set) throw new ApiError("domain", "not a manual collection", "invalid");
            resolveTarget(target).forEach((id) => set.delete(id));
            refreshCounts();
            emit(catalogBus, { scope: "collections", ids: [collectionId] });
        },

        // ---- Import & jobs ----
        async startImport(sourceId) {
            const jobId = `job-${Math.random().toString(36).slice(2, 8)}`;
            const total = 50;
            let done = 0;
            const timer = setInterval(() => {
                done += 5;
                if (done >= total) {
                    clearInterval(timer);
                    jobs.delete(jobId);
                    emit(jobDoneBus, { jobId, kind: "import", summary: { added: total, updated: 0, skipped: 3, errors: 0 } });
                    emit(catalogBus, { scope: "assets" });
                } else {
                    emit(jobProgressBus, { jobId, kind: "import", done, total, stage: done < 25 ? "hashing" : "thumbnailing" });
                }
            }, 200);
            jobs.set(jobId, timer);
            console.info("[mock] import started for", sourceId, "→", jobId);
            return jobId;
        },
        async cancelJob(jobId) {
            const timer = jobs.get(jobId);
            if (timer) {
                clearInterval(timer);
                jobs.delete(jobId);
                emit(jobDoneBus, { jobId, kind: "import", error: "cancelled" });
            }
        },

        // ---- Settings & keybindings ----
        async getSettings() {
            return { ...settings };
        },
        async updateSettings(patch: SettingsPatch) {
            settings = { ...settings, ...patch };
        },
        async listKeybindings() {
            return keybindings.map((k) => ({ ...k }));
        },
        async setKeybinding(action, combo, context: KeybindingContext) {
            const clash = keybindings.find((k) => k.combo === combo && k.context === context && k.action !== action);
            if (clash) throw new ApiError("domain", `${combo} already bound to ${clash.action}`, "keybinding_conflict", clash.action);
            const existing = keybindings.find((k) => k.action === action && k.context === context);
            if (existing) existing.combo = combo;
            else keybindings.push({ action, combo, context });
        },
        async resetKeybindings() {
            keybindings = seedKeybindings();
        },

        // ---- History ----
        async undo() {
            const cmd = undoStack.pop();
            if (!cmd) return;
            cmd.undo();
            redoStack.push(cmd);
            refreshCounts();
            emit(historyBus, historyState());
            emit(catalogBus, { scope: "assets" });
        },
        async redo() {
            const cmd = redoStack.pop();
            if (!cmd) return;
            cmd.redo();
            undoStack.push(cmd);
            refreshCounts();
            emit(historyBus, historyState());
            emit(catalogBus, { scope: "assets" });
        },

        // ---- Events ----
        onCatalogChanged: (cb) => sub(catalogBus, cb),
        onJobProgress: (cb) => sub(jobProgressBus, cb),
        onJobDone: (cb) => sub(jobDoneBus, cb),
        onSourceStatus: (cb) => sub(sourceStatusBus, cb),
        onHistoryChanged: (cb) => sub(historyBus, cb),
        onUpdateAvailable: (cb) => sub(updateBus, cb),

        // ---- Binary channel (URL builders) ----
        thumbnailURL: (a) => `${thumbUrl(a, 480)}?v=${a.thumbnailAt}`,
        previewURL: (a) => `${thumbUrl(a, 1600)}?v=${a.thumbnailAt}`,
        originalURL: (a) => thumbUrl(a, Math.min(a.width, 3000)),
    };
}

function seedKeybindings(): Keybinding[] {
    return [
        { action: "rate_0", combo: "0", context: "grid" },
        { action: "rate_1", combo: "1", context: "grid" },
        { action: "rate_5", combo: "5", context: "grid" },
        { action: "flag_pick", combo: "p", context: "grid" },
        { action: "flag_reject", combo: "x", context: "grid" },
        { action: "undo", combo: "cmd+z", context: "global" },
        { action: "redo", combo: "cmd+shift+z", context: "global" },
    ];
}
