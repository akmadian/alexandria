// The single binding boundary — the app's shared language with the Go backend.
// Everything the frontend knows about the backend goes through `AlexandriaAPI`;
// nothing else touches window.go.* or runtime.*.
//
// This file is the CONTRACT (types + interface + error shape). The mock lives in
// ./mock-api.ts; the real Wails impl (later) in ./wails-api.ts. Swapping backends
// is the one-line change at the bottom.
//
// Deliberately NOT here: caching, debouncing, single-flight, request superseding,
// optimistic updates. Those are consumer concerns (TanStack Query + a thumbnail
// loader) and live one layer up. This module stays a thin, honest transport.
//
// Design conventions this surface obeys (docs/frontend-architecture.md §2):
//   1. Resource-oriented verbs; surface grows with ENTITIES, not features.
//   2. Envelopes absorb field growth — never add per-field bindings.
//   3. One job envelope for every long-running op.
//   4. Binaries never cross this boundary — they go over the asset URL scheme.
//   5. Codes cross the seam, not strings (ApiError.kind/code, enum values).
//   6. Consumers tolerate unknown enum values (forward-compatible rendering).

import type {
    Asset,
    Collection,
    ColorLabel,
    FileStatus,
    FileType,
    Flag,
    Source,
    SourceStatus,
    Tag,
} from "../models/index.ts";

// Re-export the domain models as part of the contract, so consumers have one
// door: import every backend type from `@/api/contract`.
export type { Asset, Collection, ColorLabel, FileStatus, FileType, Flag, Source, SourceStatus, Tag };

// ---------------------------------------------------------------------------
// The query model: scope × filter × sort × page (§3)
//
// A scope is WHERE you're looking (an extensional set); a filter is a predicate
// over that set. Collections are scopes, not filter fields — never merge them.
// ---------------------------------------------------------------------------

export type AssetScope =
    | { kind: "library" }
    | { kind: "collection"; id: string } // manual or smart — backend resolves either
    | { kind: "folder"; sourceId: string; path: string; recursive?: boolean }; // path "" = source root
// reserved (P1): | { kind: "group"; id: string }

/** Predicate only — no scope, no sort, no paging. Grows by adding optional fields. */
export interface AssetFilter {
    fileTypes?: FileType[];
    ratingExact?: number;
    ratingMin?: number;
    colorLabels?: ColorLabel[];
    flags?: Exclude<Flag, null>[];
    tagIds?: string[];
    sourceIds?: string[];
    fileStatus?: FileStatus[];
    capturedAfter?: string; // ISO
    capturedBefore?: string; // ISO
    searchText?: string;
    // Absence queries — the triage workflow filters on "what I haven't done yet".
    unrated?: boolean;
    unflagged?: boolean;
    unlabeled?: boolean;
    untagged?: boolean;
    includeDeleted?: boolean;
}

export type AssetSortField = "captured" | "added" | "rating" | "filename" | "size";
export interface AssetSort {
    field: AssetSortField;
    dir: "asc" | "desc";
}

export interface Page {
    limit: number;
    offset: number;
}

export interface ListQuery {
    scope: AssetScope;
    filter?: AssetFilter;
    sort?: AssetSort;
    page?: Page;
}

/** Grid-card projection — the slim ~15-field shape (§4). Full Asset is getAsset only.
 *  Structurally open: gains a `kind` discriminator when P1 asset groups land. */
export interface AssetRow {
    kind: "asset";
    id: string;
    filename: string;
    extension: string;
    fileType: FileType;
    fileStatus: FileStatus;
    rating: number;
    colorLabel: ColorLabel | null;
    flag: Flag;
    width: number;
    height: number;
    sizeBytes: number;
    durationSecs?: number;
    capturedAt: string;
    thumbURL: string; // content-addressed, immutable
}

export interface ListAssetsResult {
    items: AssetRow[];
    /** Total matching scope+filter, ignoring paging — sizes the grid scrollbar. */
    total: number;
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

/** Mutation target: explicit ids, or "everything matching this view".
 *  Destructive disk ops require {ids} — never a query. */
export type PatchTarget = { ids: string[] } | { scope: AssetScope; filter?: AssetFilter };

/** Sparse triage patch. Absent = don't touch; null = clear (Opt[T] on the Go side). */
export interface AssetPatch {
    rating?: number | null;
    colorLabel?: ColorLabel | null;
    flag?: Flag; // Flag already includes null
    note?: string | null;
}

export interface SourceInput {
    name: string;
    kind: Source["kind"];
    basePath: string;
    scanRecursively?: boolean;
}
export interface SourcePatch {
    name?: string;
    status?: Extract<SourceStatus, "active" | "offline">;
    scanRecursively?: boolean;
}

export interface TagInput {
    name: string;
    parentId?: string | null;
    color?: ColorLabel;
}
export interface TagPatch {
    name?: string;
    parentId?: string | null; // rename + reparent through one verb
    color?: ColorLabel;
}

export interface CollectionInput {
    name: string;
    parentId?: string | null;
    kind?: Collection["kind"];
}
export interface CollectionPatch {
    name?: string;
    parentId?: string | null;
    coverAssetId?: string | null;
}

// ---------------------------------------------------------------------------
// Folder view — derived from asset paths, never stored (§3)
// ---------------------------------------------------------------------------

export interface FolderNode {
    name: string; // "" for the source root
    path: string; // "" for root, else "a/b/c"
    directCount: number; // assets directly in this folder
    totalCount: number; // assets in this folder + all descendants
    children: FolderNode[];
}

// ---------------------------------------------------------------------------
// Jobs — one envelope for every long-running op (§2)
// ---------------------------------------------------------------------------

export type JobKind = "import" | "reconcile" | "integrity" | "xmp_sync" | "thumb_rebuild";

export interface JobProgress {
    jobId: string;
    kind: JobKind;
    done: number;
    total: number;
    stage?: string;
}

export interface JobSummary {
    added: number;
    updated: number;
    skipped: number;
    errors: number;
}

export interface JobDone {
    jobId: string;
    kind: JobKind;
    summary?: JobSummary;
    error?: string;
}

// ---------------------------------------------------------------------------
// Settings & keybindings
// ---------------------------------------------------------------------------

/** Catalog-scoped, user-editable subset of domain.Settings. Machine-scoped tuning
 *  (worker pools, memory limit) is not here — it lives in machine.json. */
export interface Settings {
    xmpConflictResolution: "xmp_wins" | "catalog_wins";
    duplicateHandling: "auto_drop" | "review";
    thumbnailQuality: number;
    defaultSortField: AssetSortField;
    defaultSortDir: "asc" | "desc";
    undoStackSize: number;
    catalogBackupCount: number;
    updateCheckEnabled: boolean;
}
export type SettingsPatch = Partial<Settings>;

export type KeybindingContext = "global" | "grid" | "detail" | "import";
export interface Keybinding {
    action: string; // domain keybinding action constant, e.g. "rate_5"
    combo: string; // e.g. "5", "cmd+z"
    context: KeybindingContext;
}

// ---------------------------------------------------------------------------
// Events (Go → JS, push)
// ---------------------------------------------------------------------------

/** Coarse by default, scope-capable. Consumers may ignore the payload and
 *  invalidate the active view, or use scope/ids for selective invalidation. */
export interface CatalogChange {
    scope?: "assets" | "tags" | "collections" | "sources";
    ids?: string[];
}

export interface SourceStatusEvent {
    sourceId: string;
    status: SourceStatus;
}

export interface HistoryState {
    canUndo: boolean;
    canRedo: boolean;
    undoLabel?: string;
    redoLabel?: string;
}

export interface UpdateAvailable {
    version: string;
    url: string;
}

export type Unsubscribe = () => void;

// ---------------------------------------------------------------------------
// Errors — every failure normalizes to this so consumers switch on kind/code
// rather than sniffing strings, and display text stays frontend-owned.
// ---------------------------------------------------------------------------

export type ApiErrorKind = "transport" | "degraded" | "domain" | "unexpected";

export class ApiError extends Error {
    kind: ApiErrorKind;
    code?: string; // typed domain code, e.g. "source_offline", "keybinding_conflict", "not_found"
    detail?: unknown;
    constructor(kind: ApiErrorKind, message: string, code?: string, detail?: unknown) {
        super(message);
        this.name = "ApiError";
        this.kind = kind;
        this.code = code;
        this.detail = detail;
    }
}

// ---------------------------------------------------------------------------
// The contract
// ---------------------------------------------------------------------------

export interface AlexandriaAPI {
    // --- Assets ---
    listAssets(query: ListQuery): Promise<ListAssetsResult>;
    getAsset(id: string): Promise<Asset | null>;
    patchAssets(target: PatchTarget, patch: AssetPatch): Promise<void>;
    setAssetTags(assetId: string, tagIds: string[]): Promise<void>;
    removeFromCatalog(target: PatchTarget): Promise<void>; // soft delete, undoable
    deleteFromDisk(ids: string[]): Promise<void>; // destructive, NOT undoable
    openAsset(id: string): Promise<void>;
    openWith(id: string, appName: string): Promise<void>;
    revealInFileManager(id: string): Promise<void>;

    // --- Sources ---
    listSources(): Promise<Source[]>;
    createSource(input: SourceInput): Promise<Source>;
    updateSource(id: string, patch: SourcePatch): Promise<void>;
    removeSource(id: string): Promise<void>;
    pickDirectory(): Promise<string | null>; // native dialog, for Add Source
    getFolderTree(sourceId: string): Promise<FolderNode>;

    // --- Tags ---
    tagTree(): Promise<Tag[]>;
    createTag(input: TagInput): Promise<Tag>;
    updateTag(id: string, patch: TagPatch): Promise<void>;
    deleteTag(id: string): Promise<void>;

    // --- Collections ---
    listCollections(): Promise<Collection[]>;
    createCollection(input: CollectionInput): Promise<Collection>;
    updateCollection(id: string, patch: CollectionPatch): Promise<void>;
    deleteCollection(id: string): Promise<void>;
    addToCollection(collectionId: string, target: PatchTarget): Promise<void>;
    removeFromCollection(collectionId: string, target: PatchTarget): Promise<void>;

    // --- Import & jobs ---
    startImport(sourceId: string): Promise<string>; // returns jobId; progress via job events
    cancelJob(jobId: string): Promise<void>;

    // --- Settings & keybindings ---
    getSettings(): Promise<Settings>;
    updateSettings(patch: SettingsPatch): Promise<void>;
    listKeybindings(): Promise<Keybinding[]>;
    setKeybinding(action: string, combo: string, context: KeybindingContext): Promise<void>;
    resetKeybindings(): Promise<void>;

    // --- History ---
    undo(): Promise<void>;
    redo(): Promise<void>;

    // --- Events ---
    onCatalogChanged(cb: (c: CatalogChange) => void): Unsubscribe;
    onJobProgress(cb: (p: JobProgress) => void): Unsubscribe;
    onJobDone(cb: (d: JobDone) => void): Unsubscribe;
    onSourceStatus(cb: (e: SourceStatusEvent) => void): Unsubscribe;
    onHistoryChanged(cb: (s: HistoryState) => void): Unsubscribe;
    onUpdateAvailable(cb: (u: UpdateAvailable) => void): Unsubscribe;

    // --- Binary channel (URL builders; bytes never cross IPC) ---
    // Grid tiles carry their own thumbURL on the row; these are for the inspector.
    thumbnailURL(asset: Asset): string;
    previewURL(asset: Asset): string;
    originalURL(asset: Asset): string;
}

// Deferred bindings, pre-named to prove the conventions hold — DO NOT implement:
//   groupAssets / ungroupAssets / setGroupCover     (P1 asset groups)
//   listDuplicates / resolveDuplicate               (duplicate review)
//   startIntegrityCheck                             (job — reuses the job envelope)
// Each lands as standard verbs on an existing channel; none reshapes what's above.
//
// The active backend singleton lives at the top of ./queries.ts (its only
// consumer), so this contract module stays a runtime leaf with no dependency on
// any implementation — it's the artifact that will mirror the Go domain types.
