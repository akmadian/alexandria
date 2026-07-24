// The mock backend — a faithful `AlexandriaAPI` implementation with an in-memory
// AST query engine. `evaluate` is the SQL stand-in: it runs the same WhereNode
// tree the real compiler will, so filter / sort / paging genuinely work and the
// UI develops against real query behavior with no Wails, no Go. When the Wails
// adapter binds, the contract is unchanged — this file is simply not selected in
// ./client.ts.

import type { EventType, JobState } from "@/_generated-types/events";
import type { CollectionKind, ColorLabel, FileStatus, FileType, Flag, VolumeConnectivity, VolumeKind, SyncMode } from "@/_generated-types/enums";
import type { Envelope, JobDone, JobProgress } from "@/_generated-types/models";
import type { SortField, TokenField } from "@/_generated-types/vocabulary";
import { log } from "@/lib/logger";
import type { Arrangement, Leaf, Query, WhereNode } from "@/query-model/ast";
import { DEFAULT_ARRANGEMENT, isLeaf } from "@/query-model/ast";
import { leaf } from "@/query-model/registry";
import type {
    AlexandriaAPI,
    AssetDetail,
    AssetQueryResult,
    AssetRow,
    CollectionNode,
    CreateFolderOutcome,
    FolderNode,
    FolderPatch,
    TriagePatch,
    UpdateTarget,
    VolumeNode,
} from "./contract";
import { ApiError } from "./contract";

// Rich internal record the engine evaluates over — a superset of AssetRow plus the
// filterable metadata fields (stands in for a catalog row).
interface MockAsset {
    id: string;
    filename: string;
    fileType: FileType;
    fileStatus: FileStatus;
    rating: number | null; // null = unrated (NULL is the truth end to end)
    colorLabel: ColorLabel | null;
    flag: Flag | null;
    width: number | null; // null = extraction absent (with capturedAt, on the "undated scan" seeds)
    height: number | null;
    sizeBytes: number;
    durationSecs: number | null;
    capturedAt: string | null; // ISO; null = undated (scan with no EXIF)
    ingestedAt: string; // ISO
    cameraMake: string | null;
    cameraModel: string | null;
    lensModel: string | null;
    title: string | null;
    caption: string | null;
    creator: string | null;
    copyright: string | null;
    volumeId: string;
    tagIds: string[];
    thumbURL: string;

    // Detail-only fields (the getAsset read; never queried by the AST engine).
    extension: string;
    mimeType: string;
    relativePath: string;
    mtime: string;
    focalLengthMm: number | null;
    aperture: number | null;
    shutterSpeed: string | null;
    iso: number | null;
    gpsLat: number | null;
    gpsLon: number | null;
    colorSpace: string | null;
    bitDepth: number | null;
    note: string | null;
    extendedMetadata: Record<string, unknown> | undefined;
}

// --- seed --------------------------------------------------------------------

function thumbDataUri(seed: number, ratio: [number, number]): string {
    const hue = (seed * 47) % 360;
    const [rw, rh] = ratio;
    const width = 240;
    const height = Math.round((width * rh) / rw);
    const svg =
        `<svg xmlns='http://www.w3.org/2000/svg' width='${width}' height='${height}'>` +
        `<defs><linearGradient id='g' x1='0' y1='0' x2='1' y2='1'>` +
        `<stop offset='0' stop-color='hsl(${hue}, 42%, 56%)'/>` +
        `<stop offset='1' stop-color='hsl(${(hue + 40) % 360}, 38%, 34%)'/>` +
        `</linearGradient></defs><rect width='100%' height='100%' fill='url(#g)'/></svg>`;
    return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
}

// The five assignable LrC labels (§5) — `orange` stays in the ColorLabel enum for
// XMP round-trip but is not part of the palette, so the mock never seeds it.
const LABELS: (ColorLabel | null)[] = ["red", "yellow", "green", "blue", "purple", null, null, null, null];
const CAMERAS: [string, string][] = [
    ["Sony", "A7 IV"],
    ["Canon", "EOS R5"],
    ["Nikon", "Z8"],
    ["Fujifilm", "X-T5"],
    ["Leica", "Q3"],
];
const KIND: { type: FileType; ext: string; mime: string; ratio: [number, number]; temporal: boolean }[] = [
    { type: "image", ext: "jpg", mime: "image/jpeg", ratio: [3, 2], temporal: false },
    { type: "raw", ext: "arw", mime: "image/x-sony-arw", ratio: [3, 2], temporal: false },
    { type: "image", ext: "png", mime: "image/png", ratio: [1, 1], temporal: false },
    { type: "video", ext: "mp4", mime: "video/mp4", ratio: [16, 9], temporal: true },
    { type: "vector", ext: "svg", mime: "image/svg+xml", ratio: [4, 3], temporal: false },
];

const APERTURES = [1.8, 2.8, 4, 5.6, 8];
const SHUTTERS = ["1/1000", "1/250", "1/80", "1/30", "1/8000"];
const ISOS = [100, 200, 400, 800, 3200];
const FOCALS = [24, 35, 50, 85, 18.5];

function seededAssets(count: number): MockAsset[] {
    const assets: MockAsset[] = [];
    for (let i = 0; i < count; i++) {
        const kind = KIND[i % KIND.length];
        const [rw, rh] = kind.ratio;
        const longEdge = 4000 + (i % 6) * 640;
        const camera = i % 6 === 5 ? null : CAMERAS[i % CAMERAS.length];
        const flag: Flag | null = i % 7 === 0 ? "pick" : i % 11 === 0 ? "reject" : null;
        // "Undated scan" seeds: no EXIF at all — null capture date AND null
        // dimensions — so absence-shaped grammar (empty/notWithin/dim guards)
        // is exercisable against the mock.
        const undatedScan = i % 9 === 4;
        assets.push({
            id: `mock-${String(i).padStart(4, "0")}`,
            filename: `${kind.temporal ? "CLIP" : "DSC"}_${String(4820 + i).padStart(5, "0")}.${kind.ext}`,
            fileType: kind.type,
            fileStatus: "online",
            rating: [null, null, 3, 5, 2, 4, null, 1][i % 8],
            colorLabel: LABELS[i % LABELS.length],
            flag,
            width: undatedScan ? null : longEdge,
            height: undatedScan ? null : Math.round((longEdge * rh) / rw),
            sizeBytes: (8 + (i % 20)) * 1024 * 1024,
            durationSecs: kind.temporal ? 12 + (i % 5) * 47 : null,
            capturedAt: undatedScan ? null : new Date(2025, i % 12, (i % 27) + 1, 9 + (i % 8), (i * 7) % 60).toISOString(),
            ingestedAt: new Date(2026, 5, (i % 27) + 1).toISOString(),
            cameraMake: camera?.[0] ?? null,
            cameraModel: camera?.[1] ?? null,
            lensModel: camera ? "24-70mm F2.8" : null,
            title: i % 4 === 0 ? `Untitled ${i}` : null,
            caption: null,
            creator: i % 3 === 0 ? "A. Photographer" : null,
            copyright: null,
            volumeId: `src-${i % 3}`,
            tagIds: [],
            thumbURL: thumbDataUri(i + 1, kind.ratio),

            extension: kind.ext,
            mimeType: kind.mime,
            // A real folder segment so the inspector's Folder row (and dirname
            // logic) is exercisable against the mock, like the real catalog.
            relativePath: `2026/${kind.temporal ? "CLIP" : "DSC"}_${String(4820 + i).padStart(5, "0")}.${kind.ext}`,
            mtime: new Date(2026, 4, (i % 27) + 1, 12, (i * 11) % 60).toISOString(),
            // Exposure travels with the camera: an undated scan carries none.
            focalLengthMm: camera && !undatedScan ? FOCALS[i % FOCALS.length] : null,
            aperture: camera && !undatedScan ? APERTURES[i % APERTURES.length] : null,
            shutterSpeed: camera && !undatedScan ? SHUTTERS[i % SHUTTERS.length] : null,
            iso: camera && !undatedScan ? ISOS[i % ISOS.length] : null,
            gpsLat: i % 5 === 0 && !undatedScan ? 47.6 + (i % 10) / 100 : null,
            gpsLon: i % 5 === 0 && !undatedScan ? -122.33 - (i % 10) / 100 : null,
            colorSpace: undatedScan ? null : i % 2 === 0 ? "sRGB" : "Adobe RGB",
            bitDepth: kind.type === "raw" ? 14 : kind.temporal ? 10 : 8,
            note: i % 13 === 0 ? "Check focus on the eyes before export." : null,
            extendedMetadata:
                camera && !undatedScan
                    ? {
                          "EXIF:Flash": "Did not fire",
                          "EXIF:MeteringMode": "Center-weighted average",
                          "EXIF:ExposureProgram": "Aperture priority",
                          "EXIF:ExposureCompensation": "-0.33",
                          "EXIF:SerialNumber": `52E0${1900 + i}`,
                          "EXIF:Software": `${camera[0]} Firmware 4.31`,
                          // Structured value, mirroring the importer's
                          // alexandria:extension_mismatch map — the renderer
                          // must never show "[object Object]".
                          ...(i % 10 === 0
                              ? { "alexandria:extension_mismatch": { declared: "jpg", detected: "png" } }
                              : {}),
                      }
                    : undefined,
        });
    }
    return assets;
}

const CATALOG: MockAsset[] = seededAssets(64);

function toRow(asset: MockAsset): AssetRow {
    return {
        kind: "asset",
        id: asset.id,
        volumeId: asset.volumeId,
        filename: asset.filename,
        fileType: asset.fileType,
        fileStatus: asset.fileStatus,
        rating: asset.rating,
        colorLabel: asset.colorLabel,
        flag: asset.flag,
        width: asset.width,
        height: asset.height,
        sizeBytes: asset.sizeBytes,
        durationSecs: asset.durationSecs,
        cameraModel: asset.cameraModel,
        capturedAt: asset.capturedAt,
        ingestedAt: asset.ingestedAt,
        thumbnailAt: null, // mock thumbs are data URIs, not generated files
        relativePath: asset.relativePath,
        thumbURL: asset.thumbURL,
    };
}

function toDetail(asset: MockAsset): AssetDetail {
    return {
        id: asset.id,
        volumeId: asset.volumeId,
        filename: asset.filename,
        extension: asset.extension,
        mimeType: asset.mimeType,
        fileType: asset.fileType,
        fileStatus: asset.fileStatus,
        relativePath: asset.relativePath,
        sizeBytes: asset.sizeBytes,
        mtime: asset.mtime,
        ingestedAt: asset.ingestedAt,
        width: asset.width,
        height: asset.height,
        durationSecs: asset.durationSecs,
        capturedAt: asset.capturedAt,
        cameraMake: asset.cameraMake,
        cameraModel: asset.cameraModel,
        lensModel: asset.lensModel,
        focalLengthMm: asset.focalLengthMm,
        aperture: asset.aperture,
        shutterSpeed: asset.shutterSpeed,
        iso: asset.iso,
        gpsLat: asset.gpsLat,
        gpsLon: asset.gpsLon,
        colorSpace: asset.colorSpace,
        bitDepth: asset.bitDepth,
        title: asset.title,
        caption: asset.caption,
        creator: asset.creator,
        copyright: asset.copyright,
        rating: asset.rating,
        colorLabel: asset.colorLabel,
        flag: asset.flag,
        note: asset.note,
        extendedMetadata: asset.extendedMetadata,
    };
}

// --- the query engine (SQL stand-in) -----------------------------------------

// Field → value accessor. `satisfies Record<TokenField, …>` is the completeness
// gate (C10): a new generated field fails to compile until it has an accessor.
const FIELD: Record<TokenField, (asset: MockAsset) => unknown> = {
    cameraMake: (a) => a.cameraMake,
    cameraModel: (a) => a.cameraModel,
    caption: (a) => a.caption,
    capturedAt: (a) => a.capturedAt,
    // Backend-computed signals; the mock carries no signal fixtures yet, so they
    // read as "not computed" (null) — filters match nothing until it grows them
    // (DEFERRED §12).
    clippingHighlights: () => null,
    clippingShadows: () => null,
    colorLabel: (a) => a.colorLabel,
    copyright: (a) => a.copyright,
    creator: (a) => a.creator,
    fileStatus: (a) => a.fileStatus,
    fileType: (a) => a.fileType,
    filename: (a) => a.filename,
    flag: (a) => a.flag,
    height: (a) => a.height,
    ingestedAt: (a) => a.ingestedAt,
    lensModel: (a) => a.lensModel,
    rating: (a) => a.rating,
    sharpness: () => null,
    volumeId: (a) => a.volumeId,
    tag: (a) => a.tagIds,
    text: (a) => `${a.filename} ${a.title ?? ""} ${a.caption ?? ""} ${a.creator ?? ""}`,
    title: (a) => a.title,
    width: (a) => a.width,
} satisfies Record<TokenField, (asset: MockAsset) => unknown>;

function isEmpty(value: unknown): boolean {
    if (Array.isArray(value)) return value.length === 0;
    return value === null || value === undefined || value === "";
}

const asString = (value: unknown): string => (value == null ? "" : String(value)).toLowerCase();

// Half-open date interval from an anchor + signed ISO 8601 duration — the
// decided wire grammar (03-data-model, 2026-07-10): anchor "now" | RFC 3339 |
// date-only; duration "-P30D" / "PT2H" / "P3M".
// ponytail: month/year arithmetic approximates (30d/365d) where the backend is
// calendar-exact via AddDate; exact-parity lands with the date editor if the
// mock outlives the Wails bind.
function parseISODurationMs(raw: string): number | null {
    const match = /^([+-]?)P(?:(\d+)Y)?(?:(\d+)M)?(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$/.exec(raw);
    if (!match || raw.endsWith("P") || raw.endsWith("T")) return null;
    const [, sign, years, months, weeks, days, hours, minutes, seconds] = match;
    const dayTotal = Number(years ?? 0) * 365 + Number(months ?? 0) * 30 + Number(weeks ?? 0) * 7 + Number(days ?? 0);
    const ms =
        dayTotal * 86_400_000 + Number(hours ?? 0) * 3_600_000 + Number(minutes ?? 0) * 60_000 + Number(seconds ?? 0) * 1_000;
    if (ms === 0) return null;
    return sign === "-" ? -ms : ms;
}

function dateWithin(iso: string, value: unknown): boolean {
    if (typeof value !== "object" || value === null) return false;
    const { anchor, duration } = value as { anchor?: unknown; duration?: unknown };
    const anchorMs = anchor === "now" ? Date.now() : Date.parse(String(anchor));
    const offsetMs = parseISODurationMs(String(duration));
    if (Number.isNaN(anchorMs) || offsetMs === null) return false;
    const otherMs = anchorMs + offsetMs;
    const t = Date.parse(iso);
    return t >= Math.min(anchorMs, otherMs) && t < Math.max(anchorMs, otherMs);
}

function evaluateLeaf(leaf: Leaf, asset: MockAsset): boolean {
    const value = FIELD[leaf.field](asset);
    const arr = Array.isArray(leaf.value) ? (leaf.value as unknown[]) : null;
    switch (leaf.cmp) {
        case "eq":
            return value === leaf.value;
        case "neq":
            return value !== leaf.value;
        case "gte":
            return typeof value === "number" && typeof leaf.value === "number" && value >= leaf.value;
        case "lte":
            return typeof value === "number" && typeof leaf.value === "number" && value <= leaf.value;
        case "contains":
            return asString(value).includes(asString(leaf.value));
        case "startsWith":
            return asString(value).startsWith(asString(leaf.value));
        case "matches":
            // ponytail: substring over concatenated fields where the backend is
            // FTS5 MATCH — different ranking/tokenization by design; the mock
            // only needs plausible free-text narrowing.
            return asString(value).includes(asString(leaf.value));
        case "in":
            return arr !== null && arr.includes(value);
        case "notIn":
            // NULL-negation policy: negation includes absent — a null value is
            // "not in" any listed set (matches the compiled SQL's IS NULL arm).
            return arr === null || !arr.includes(value);
        case "empty":
            return isEmpty(value);
        case "notEmpty":
            return !isEmpty(value);
        case "has":
            return Array.isArray(value) && value.includes(leaf.value);
        case "lacks":
        case "notUnder":
            return !(Array.isArray(value) && value.includes(leaf.value));
        case "under":
            // ponytail: `under` evaluates as flat `has` — no tag tree exists in
            // the mock. Subtree semantics land with the tag editor/browser slice.
            return Array.isArray(value) && value.includes(leaf.value);
        case "within":
            return typeof value === "string" && dateWithin(value, leaf.value);
        case "notWithin":
            // NULL-negation policy: an absent date is "not within" any range.
            return !(typeof value === "string" && dateWithin(value, leaf.value));
    }
}

function evaluate(node: WhereNode | null, asset: MockAsset): boolean {
    if (node === null) return true;
    if (isLeaf(node)) return evaluateLeaf(node, asset);
    switch (node.op) {
        case "and":
            return node.children.every((child) => evaluate(child, asset));
        case "or":
            return node.children.some((child) => evaluate(child, asset));
        case "not":
            return !node.children.some((child) => evaluate(child, asset));
    }
}

// Scope narrowing (the extensional root). library = all; collection membership is
// real (backed by COLLECTIONS below); folder narrows by volume (path-precise
// narrowing lands with the folder-scope work — the tree's counts are honest
// regardless); tag membership lands with the tag browser.
function inScope(query: Query, asset: MockAsset): boolean {
    switch (query.scope.kind) {
        case "library":
            return true;
        case "folder":
            return asset.volumeId === query.scope.volumeId;
        case "collection":
            return collectionContains(query.scope.id, asset);
        case "tag":
            return true; // membership lands with the tag browser
    }
}

// A collection's membership, the SQL membership-table / smart-query stand-in: a
// manual collection lists its members; a smart collection re-runs its stored
// predicate through the SAME `evaluate` the query engine uses, so its scope and
// its badge count can never disagree (D41).
function collectionContains(id: string, asset: MockAsset): boolean {
    const collection = COLLECTIONS.find((candidate) => candidate.id === id);
    if (collection === undefined) return false;
    if (collection.kind === "manual") return collection.memberIds.includes(asset.id);
    return evaluate(collection.where, asset);
}

// Sort accessors keyed by the GENERATED SortField union — the completeness
// gate (C10): a new sort field fails to compile until it has an accessor.
const SORT_ACCESSOR: Record<SortField, (a: MockAsset) => number | string | null> = {
    // ponytail: backend COALESCEs captured_at to mtime; the mock has no mtime,
    // so undated seeds sort nulls-first instead. Parity lands with the fixture
    // work if the mock outlives the Wails bind (DEFERRED).
    capturedAt: (a) => a.capturedAt,
    ingestedAt: (a) => a.ingestedAt,
    rating: (a) => a.rating,
    filename: (a) => a.filename,
    size: (a) => a.sizeBytes,
} satisfies Record<SortField, (a: MockAsset) => number | string | null>;

function compare(a: MockAsset, b: MockAsset, arrangement: Arrangement): number {
    const accessor = SORT_ACCESSOR[arrangement.sortField];
    const av = accessor(a);
    const bv = accessor(b);
    let primary: number;
    // SQLite sorts NULLs smallest; match it so mock ordering equals compiled ordering.
    if (av === null || bv === null) primary = av === bv ? 0 : av === null ? -1 : 1;
    else if (typeof av === "number" && typeof bv === "number") primary = av - bv;
    else primary = String(av).localeCompare(String(bv));
    // Direction applies to the sort field ONLY; the id tiebreaker is always
    // ascending — matching SQL `ORDER BY <field> <dir>, id ASC` (seam/01 §Additions
    // #4). Negating the tiebreaker too would order tied rows id-descending under
    // desc, diverging from the backend this mock stands in for.
    const directed = arrangement.sortDir === "desc" ? -primary : primary;
    return directed !== 0 ? directed : a.id.localeCompare(b.id);
}

function orderedMatches(query: Query, arrangement: Arrangement): MockAsset[] {
    return CATALOG.filter((asset) => inScope(query, asset) && evaluate(query.where, asset)).sort((a, b) =>
        compare(a, b, arrangement),
    );
}

// --- the write path (triage) -------------------------------------------------

// Apply a sparse three-state patch to a seeded record IN PLACE — the SQL
// UPDATE stand-in. A present key sets (a value) or clears (null). Gated on
// `!== undefined`, not `in`: an explicitly-undefined key would be DROPPED by
// JSON serialization at the real seam ("don't touch"), so the mock must treat
// it the same, never as a clear. The frontend never sends 0 for rating (the
// Rating primitive clears with null), so "0 is not a rating" holds unguarded.
function applyPatch(asset: MockAsset, patch: TriagePatch): void {
    if (patch.rating !== undefined) asset.rating = patch.rating;
    if (patch.colorLabel !== undefined) asset.colorLabel = patch.colorLabel;
    if (patch.flag !== undefined) asset.flag = patch.flag;
    if (patch.note !== undefined) asset.note = patch.note;
}

// --- sources -----------------------------------------------------------------

// MockSource is the mock's INTERNAL legacy seed shape — the wire's Source type
// died with the Volume/Folder split (D41); this survives only to drive the fake
// import job until the frontend-import epic rebinds startImport to folder seeds.
interface MockSource {
    ID: string;
    Name: string;
    Kind: VolumeKind;
    BasePath: string;
    ScanRecursively: boolean;
    Enabled: boolean;
    Connectivity: VolumeConnectivity;
    CreatedAt: string;
    UpdatedAt: string;
}

function seededSources(): MockSource[] {
    const now = new Date(2026, 5, 1).toISOString();
    const names = ["Studio SSD", "Field Drive", "Archive NAS"];
    return names.map((name, index) => ({
        ID: `src-${index}`,
        Name: name,
        Kind: index === 2 ? "smb" : index === 1 ? "external_drive" : "local",
        BasePath: `/Volumes/${name.replace(/\s+/g, "")}`,
        ScanRecursively: true,
        Enabled: true,
        Connectivity: "online",
        CreatedAt: now,
        UpdatedAt: now,
    }));
}

const SOURCES: MockSource[] = seededSources();

// --- the browser rail: volumes, folders, collections (D41) -------------------
//
// The top navigation axis the rail renders. Volumes and folders are a SEPARATE
// seed from SOURCES: the backend's Volume/Folder split (D41) has EXECUTED — the
// wire has no Source surface anymore — but the mock keeps its internal legacy
// SOURCES seed to drive the fake import job until the frontend-import epic
// rebinds startImport to folder seeds. Each
// seeded volume maps to the assets of one seeded volumeId (VOLUME_OF), so folder
// counts are DERIVED from the real catalog and sum honestly.

interface MockVolume {
    id: string;
    name: string;
    kind: VolumeKind;
    connectivity: VolumeConnectivity;
    basePath: string; // the mount prefix; disjoint createFolder paths derive a volume from theirs
}

// A tracked folder root. Roots are mutable (createFolder/removeFolder/updateFolder
// rewrite them); the rendered tree nests one root under another purely by path
// containment, so an absorb needs no reparenting bookkeeping.
interface MockTrackedRoot {
    id: string;
    volumeId: string;
    name: string;
    path: string;
    syncMode: SyncMode;
}

// volumeId → volumeId, so a volume's assets are exactly its source's assets.
const VOLUME_OF: Record<string, string> = { "src-0": "vol-studio", "src-1": "vol-field", "src-2": "vol-archive" };

function seededVolumes(): MockVolume[] {
    return [
        { id: "vol-studio", name: "Studio SSD", kind: "local", connectivity: "online", basePath: "/Volumes/StudioSSD" },
        { id: "vol-field", name: "Field Drive", kind: "external_drive", connectivity: "online", basePath: "/Volumes/FieldDrive" },
        // One offline volume (D41): dimmed + marked in the rail, but its catalog
        // stays fully browsable — connectivity is an observation, never a gate.
        { id: "vol-archive", name: "Archive NAS", kind: "smb", connectivity: "offline", basePath: "/Volumes/ArchiveNAS" },
    ];
}

function seededTrackedRoots(): MockTrackedRoot[] {
    return [
        // One watched root — the subject of the needs-confirmation flow when a
        // manual parent would absorb it.
        { id: "folder-studio", volumeId: "vol-studio", name: "Photos", path: "/Volumes/StudioSSD/Photos", syncMode: "watched" },
        // Two sibling roots under a common parent — the quiet-absorb flow (both
        // manual, so a manual parent changes nothing).
        { id: "folder-field-2024", volumeId: "vol-field", name: "2024", path: "/Volumes/FieldDrive/2024", syncMode: "manual" },
        { id: "folder-field-2025", volumeId: "vol-field", name: "2025", path: "/Volumes/FieldDrive/2025", syncMode: "manual" },
        { id: "folder-archive", volumeId: "vol-archive", name: "Archive", path: "/Volumes/ArchiveNAS/Archive", syncMode: "scheduled" },
    ];
}

let VOLUMES: MockVolume[] = seededVolumes();
let TRACKED_ROOTS: MockTrackedRoot[] = seededTrackedRoots();
let folderCounter = 0;

/** Test seam: restore the seeded volume/folder rail after mutation cases. */
export function resetMockBrowserRail(): void {
    VOLUMES = seededVolumes();
    TRACKED_ROOTS = seededTrackedRoots();
    folderCounter = 0;
}

const normalizePath = (path: string): string => path.replace(/\/+$/, "");
const basename = (path: string): string => path.slice(path.lastIndexOf("/") + 1);
const isStrictDescendant = (path: string, ancestor: string): boolean => path.startsWith(ancestor + "/");

// The nearest tracked root strictly containing `root` within its volume — the
// parent under which the rail nests it. Undefined = a top-level root.
function nearestAncestorRoot(root: MockTrackedRoot, roots: MockTrackedRoot[]): MockTrackedRoot | undefined {
    let best: MockTrackedRoot | undefined;
    for (const candidate of roots) {
        if (candidate.id === root.id || candidate.volumeId !== root.volumeId) continue;
        if (isStrictDescendant(root.path, candidate.path) && (best === undefined || candidate.path.length > best.path.length)) {
            best = candidate;
        }
    }
    return best;
}

// A leaf root (no child roots) bears assets; a container root (has child roots)
// holds none directly — its count is its children's. Volume assets are
// partitioned across the volume's leaf roots deterministically, so every asset
// lands in exactly one root's subtree and counts sum to the volume total.
function assetsForLeafRoot(root: MockTrackedRoot, volumeAssets: MockAsset[], volumeRoots: MockTrackedRoot[]): MockAsset[] {
    const leaves = volumeRoots.filter((candidate) => !volumeRoots.some((other) => nearestAncestorRoot(other, volumeRoots)?.id === candidate.id));
    leaves.sort((a, b) => a.id.localeCompare(b.id));
    const slot = leaves.findIndex((candidate) => candidate.id === root.id);
    if (slot === -1 || leaves.length === 0) return [];
    return volumeAssets.filter((_, index) => index % leaves.length === slot);
}

// Derived (non-root) subfolders: one level bucketed by capture year, "undated"
// for the EXIF-less scans. Ids are synthetic (volumeId + ":" + path, per D41),
// carry no syncMode (they inherit the root's), and hold their bucket's assets.
function yearBuckets(root: MockTrackedRoot, assets: MockAsset[], volumeId: string): FolderNode[] {
    const groups = new Map<string, number>();
    for (const asset of assets) {
        const key = asset.capturedAt === null ? "undated" : String(new Date(asset.capturedAt).getFullYear());
        groups.set(key, (groups.get(key) ?? 0) + 1);
    }
    return [...groups.keys()]
        .sort()
        .map((key): FolderNode => {
            const path = `${root.path}/${key}`;
            return { id: `${volumeId}:${path}`, name: key, path, assetCount: groups.get(key) ?? 0, children: [] };
        });
}

function buildRootNode(root: MockTrackedRoot, volumeRoots: MockTrackedRoot[], volumeAssets: MockAsset[], volumeId: string): FolderNode {
    const childRoots = volumeRoots.filter((candidate) => nearestAncestorRoot(candidate, volumeRoots)?.id === root.id);
    const childNodes = childRoots.map((childRoot) => buildRootNode(childRoot, volumeRoots, volumeAssets, volumeId));
    // A leaf root bears the assets (as year buckets); a container root defers to
    // its child roots and holds none directly.
    const derived = childRoots.length === 0 ? yearBuckets(root, assetsForLeafRoot(root, volumeAssets, volumeRoots), volumeId) : [];
    const children = [...childNodes, ...derived];
    const assetCount = children.reduce((sum, child) => sum + child.assetCount, 0);
    return { id: root.id, name: root.name, path: root.path, syncMode: root.syncMode, assetCount, children };
}

function buildVolumeTree(volume: MockVolume): VolumeNode {
    const volumeRoots = TRACKED_ROOTS.filter((root) => root.volumeId === volume.id);
    const volumeAssets = CATALOG.filter((asset) => VOLUME_OF[asset.volumeId] === volume.id);
    const topRoots = volumeRoots.filter((root) => nearestAncestorRoot(root, volumeRoots) === undefined);
    const folders = topRoots.map((root) => buildRootNode(root, volumeRoots, volumeAssets, volume.id));
    const assetCount = folders.reduce((sum, folder) => sum + folder.assetCount, 0);
    return { id: volume.id, name: volume.name, kind: volume.kind, connectivity: volume.connectivity, assetCount, folders };
}

// Resolve (or mint) the volume a disjoint createFolder path lives on, so a newly
// tracked root always has a volume to hang under in the rail.
function ensureVolumeForPath(path: string): string {
    const mount = "/" + path.split("/").slice(1, 3).join("/");
    const existing = VOLUMES.find((volume) => volume.basePath === mount);
    if (existing !== undefined) return existing.id;
    const volume: MockVolume = {
        id: `vol-${VOLUMES.length}`,
        name: basename(mount),
        kind: "external_drive",
        connectivity: "online",
        basePath: mount,
    };
    VOLUMES.push(volume);
    return volume.id;
}

// The sync mode a newly minted parent root takes. A quiet absorb of like-moded
// children changes nothing; absorbing a watched/scheduled child needs confirming.
const NEW_ROOT_SYNC_MODE: SyncMode = "manual";

function createFolderOutcome(rawPath: string, confirm: boolean): CreateFolderOutcome {
    const path = normalizePath(rawPath);
    // 1. Exact match or under an existing root → redirect to that root.
    const container = TRACKED_ROOTS.find((root) => root.path === path || isStrictDescendant(path, root.path));
    if (container !== undefined) return { kind: "already_tracked_within", folderId: container.id };

    // 2. Parent of one or more existing roots → absorb (quiet unless a
    //    watched/scheduled child's behavior would change under the new mode).
    const absorbed = TRACKED_ROOTS.filter((root) => isStrictDescendant(root.path, path));
    if (absorbed.length > 0) {
        const behaviorChanges = absorbed
            .filter((root) => root.syncMode !== NEW_ROOT_SYNC_MODE && (root.syncMode === "watched" || root.syncMode === "scheduled"))
            .map((root) => ({ folderId: root.id, folderName: root.name, currentSyncMode: root.syncMode, newSyncMode: NEW_ROOT_SYNC_MODE }));
        if (behaviorChanges.length > 0 && !confirm) {
            // No mutation — the caller re-issues with confirm=true.
            return { kind: "needs_confirmation", absorbedFolderIds: absorbed.map((root) => root.id), behaviorChanges };
        }
        folderCounter += 1;
        const parent: MockTrackedRoot = { id: `folder-new-${folderCounter}`, volumeId: absorbed[0].volumeId, name: basename(path), path, syncMode: NEW_ROOT_SYNC_MODE };
        TRACKED_ROOTS.push(parent);
        log.info("mock: folder absorbed roots", { path, absorbed: absorbed.length });
        emit("changed", { scope: "folders" });
        return { kind: "absorbed", folderId: parent.id, absorbedFolderIds: absorbed.map((root) => root.id) };
    }

    // 3. Disjoint → a brand-new tracked root (minting its volume if needed).
    folderCounter += 1;
    const root: MockTrackedRoot = { id: `folder-new-${folderCounter}`, volumeId: ensureVolumeForPath(path), name: basename(path), path, syncMode: NEW_ROOT_SYNC_MODE };
    TRACKED_ROOTS.push(root);
    log.info("mock: folder tracked", { path, id: root.id });
    emit("changed", { scope: "folders" });
    return { kind: "created", folderId: root.id };
}

// Collections. A manual collection carries an explicit membership list; a smart
// one carries a stored predicate the badge count and scope both read through
// `evaluate`. `countUnavailable` models D41's escape hatch — a smart count the
// backend declined to compute (nil on the wire, distinct from 0 = empty).
type MockCollection =
    | { id: string; name: string; parentId: string | null; kind: "manual"; memberIds: string[] }
    | { id: string; name: string; parentId: string | null; kind: "smart"; where: WhereNode; countUnavailable?: boolean };

function seededCollections(): MockCollection[] {
    return [
        { id: "col-select", name: "Selects", parentId: null, kind: "manual", memberIds: ["mock-0000", "mock-0003", "mock-0006", "mock-0012"] },
        // Nested under Selects (exercises parentId) and genuinely empty (count 0,
        // NOT null — the 0-vs-unavailable distinction).
        { id: "col-cull", name: "To Cull", parentId: "col-select", kind: "manual", memberIds: [] },
        // A smart collection with a real compiled badge.
        { id: "col-highrated", name: "High Rated", parentId: null, kind: "smart", where: leaf("rating", "gte", 4) },
        // A smart collection whose count the backend declined (nil badge), while
        // its scope membership stays computable.
        { id: "col-portfolio", name: "Portfolio", parentId: null, kind: "smart", where: leaf("fileType", "in", ["image"]), countUnavailable: true },
    ];
}

const COLLECTIONS: MockCollection[] = seededCollections();

function collectionCount(collection: MockCollection): number | null {
    if (collection.kind === "manual") return collection.memberIds.length;
    if (collection.countUnavailable === true) return null;
    return CATALOG.filter((asset) => evaluate(collection.where, asset)).length;
}

function toCollectionNode(collection: MockCollection): CollectionNode {
    const kind: CollectionKind = collection.kind;
    const node: CollectionNode = { id: collection.id, name: collection.name, kind, assetCount: collectionCount(collection) };
    if (collection.parentId !== null) node.parentId = collection.parentId;
    return node;
}

// --- the event bus (C8) ------------------------------------------------------
//
// The mock's stand-in for the Wails runtime channels: subscribers register here
// and the ticking import (below) pushes envelopes to them, so the whole event
// pump → jobs store / invalidation path runs under `bun run dev` and in tests
// with no Wails. One envelope shape, four topics (spec §Events).

type Subscriber = (envelope: Envelope) => void;
const subscribers = new Set<Subscriber>();

function emit(type: EventType, payload: JobProgress | JobDone | { scope: string }): void {
    // Topic is derived from the type the same way the Go catalog does (a type
    // can't ride the wrong topic): progress/done → jobs, changed → catalog.
    const topic = type === "changed" ? "catalog" : "jobs";
    const envelope: Envelope = { topic, type, payload, timestamp: new Date().toISOString() };
    for (const subscriber of subscribers) subscriber(envelope);
}

// --- the ticking import job (C9) ---------------------------------------------
//
// A faithful stand-in for the pipeline's progress: an indeterminate SCAN phase
// (totalKnown false, total climbing as the walk emits files), the flip to a
// known total, then a WRITE phase whose `done` climbs to the total, then a
// terminal jobs/done carrying the summary. Cancel mid-run yields a cancelled
// terminal event with the partial tally. Stages mirror the real emitter
// (pipeline.go: "scan" / "write").

interface MockImportConfig {
    /** Delay between ticks. A few hundred ms in dev (watchable); ~0 in tests. */
    tickMs: number;
}

const mockImportConfig: MockImportConfig = { tickMs: 350 };

/** Test seam: set the tick pace (and reset it) so suites run fast without fake timers. */
export function configureMockImport(config: Partial<MockImportConfig>): void {
    Object.assign(mockImportConfig, config);
}

// Running jobs → their cancel flag. Present only while ticking; deleted on any
// terminal event. CancelJob flips the flag and the next tick finishes cancelled.
const runningImports = new Map<string, { cancelled: boolean }>();
let jobCounter = 0;

const IMPORT_TOTAL = 40;

function progressFrame(jobId: string, stage: string, done: number, total: number, totalKnown: boolean): JobProgress {
    return {
        jobId,
        kind: "import",
        label: "jobs.kind.import", // i18n KEY (C14); jobLabelKey mirror
        state: "running",
        done,
        total,
        totalKnown,
        stage,
        cancelable: true,
    };
}

function doneFrame(jobId: string, state: JobState, done: number): JobDone {
    // A cancelled run still committed `done` assets; a completed run splits the
    // total across the four-count summary (added/updated/skipped/errors).
    const summary =
        state === "cancelled"
            ? { added: done, updated: 0, skipped: 0, errors: 0 }
            : { added: IMPORT_TOTAL - 6, updated: 3, skipped: 3, errors: 0 };
    return { jobId, kind: "import", state, summary };
}

function runMockImport(jobId: string): void {
    const control = { cancelled: false };
    runningImports.set(jobId, control);

    // The frame script: three indeterminate scan ticks, then determinate write
    // ticks climbing to the total.
    const frames: JobProgress[] = [
        progressFrame(jobId, "scan", 0, 8, false),
        progressFrame(jobId, "scan", 0, 22, false),
        progressFrame(jobId, "scan", 0, IMPORT_TOTAL, false),
        progressFrame(jobId, "write", 8, IMPORT_TOTAL, true),
        progressFrame(jobId, "write", 20, IMPORT_TOTAL, true),
        progressFrame(jobId, "write", 32, IMPORT_TOTAL, true),
        progressFrame(jobId, "write", IMPORT_TOTAL, IMPORT_TOTAL, true),
    ];

    let index = 0;
    let lastDone = 0;
    const step = (): void => {
        const active = runningImports.get(jobId);
        if (active === undefined) return; // defensively: already finished
        if (active.cancelled) {
            runningImports.delete(jobId);
            emit("done", doneFrame(jobId, "cancelled", lastDone));
            log.info("mock: import cancelled", { jobId, done: lastDone });
            return;
        }
        if (index >= frames.length) {
            runningImports.delete(jobId);
            emit("done", doneFrame(jobId, "done", IMPORT_TOTAL));
            // The import changed the catalog — mirror the engine's coincident
            // invalidation so the grid refetches after an import (spec §Jobs).
            emit("changed", { scope: "assets" });
            log.info("mock: import complete", { jobId });
            return;
        }
        const frame = frames[index++];
        lastDone = frame.done;
        emit("progress", frame);
        setTimeout(step, mockImportConfig.tickMs);
    };
    setTimeout(step, mockImportConfig.tickMs);
}

// Simulate seam latency so loading states get exercised.
const delay = <T>(value: T): Promise<T> => new Promise((resolve) => setTimeout(() => resolve(value), 80));

export const mockApi: AlexandriaAPI = {
    queryAssets(query, arrangement, page): Promise<AssetQueryResult> {
        const matches = orderedMatches(query, arrangement);
        const items = matches.slice(page.offset, page.offset + page.limit).map(toRow);
        return delay({ items, total: matches.length });
    },
    assetIdSlice(query, arrangement, fromIndex, toIndex): Promise<string[]> {
        const matches = orderedMatches(query, arrangement);
        return delay(matches.slice(Math.max(0, fromIndex), toIndex + 1).map((asset) => asset.id));
    },
    indexOfAsset(query, arrangement, id): Promise<number | null> {
        const index = orderedMatches(query, arrangement).findIndex((asset) => asset.id === id);
        return delay(index === -1 ? null : index);
    },
    getAsset(id): Promise<AssetDetail> {
        const asset = CATALOG.find((candidate) => candidate.id === id);
        if (!asset) {
            return delay(id).then(() => {
                throw new ApiError("domain", `asset ${id} not found`, "not_found");
            });
        }
        return delay(toDetail(asset));
    },
    updateAssets(target: UpdateTarget, patch: TriagePatch): Promise<void> {
        // Mirrors the seam's target switch exactly (asset_service.go): NON-EMPTY
        // ids win, else a query, else validation — so `{ids: []}` errors like the
        // real seam, never a silent no-op.
        if (target.ids !== undefined && target.ids.length > 0) {
            // The frontend's only form this round (task 34 ruling). Resolve every
            // id up front so an unknown id fails the whole write with not_found
            // rather than half-applying (the seam is transactional).
            const resolved: MockAsset[] = [];
            for (const id of target.ids) {
                const asset = CATALOG.find((candidate) => candidate.id === id);
                if (!asset) {
                    return delay(id).then(() => {
                        throw new ApiError("domain", `asset ${id} not found`, "not_found");
                    });
                }
                resolved.push(asset);
            }
            for (const asset of resolved) applyPatch(asset, patch);
            return delay(undefined);
        }
        // Query form: apply to the ordered matches minus exceptIds. Unused by the
        // frontend today, but kept faithful so a future undo-round consumer (and
        // the mock⇄SQL parity work) exercises the same shape the seam accepts.
        if (target.query !== undefined) {
            const excepted = new Set(target.exceptIds ?? []);
            for (const asset of orderedMatches(target.query, DEFAULT_ARRANGEMENT)) {
                if (!excepted.has(asset.id)) applyPatch(asset, patch);
            }
            return delay(undefined);
        }
        return delay(undefined).then(() => {
            throw new ApiError("domain", "update target requires either ids or a query", "validation");
        });
    },


    getFolderTree(): Promise<VolumeNode[]> {
        // Empty volumes leave the rail (D41): only volumes with a tracked root.
        const tree = VOLUMES.filter((volume) => TRACKED_ROOTS.some((root) => root.volumeId === volume.id)).map(buildVolumeTree);
        return delay(tree);
    },

    listCollections(): Promise<CollectionNode[]> {
        return delay(COLLECTIONS.map(toCollectionNode));
    },

    createFolder(path: string, confirm?: boolean): Promise<CreateFolderOutcome> {
        return delay(createFolderOutcome(path, confirm ?? false));
    },

    removeFolder(id: string): Promise<void> {
        const target = TRACKED_ROOTS.find((root) => root.id === id);
        if (target === undefined) {
            return delay(id).then(() => {
                throw new ApiError("domain", `folder ${id} not found`, "not_found");
            });
        }
        // Cascade-via-soft-delete (D41): drop the root and every root nested under
        // it. The mock has no soft-delete rows; judgments on the assets persist in
        // CATALOG, matching "judgments survive, files untouched".
        TRACKED_ROOTS = TRACKED_ROOTS.filter(
            (root) => root.id !== id && !(root.volumeId === target.volumeId && isStrictDescendant(root.path, target.path)),
        );
        log.info("mock: folder removed", { id });
        emit("changed", { scope: "folders" });
        return delay(undefined);
    },

    updateFolder(id: string, patch: FolderPatch): Promise<void> {
        const target = TRACKED_ROOTS.find((root) => root.id === id);
        if (target === undefined) {
            return delay(id).then(() => {
                throw new ApiError("domain", `folder ${id} not found`, "not_found");
            });
        }
        if (patch.name !== undefined) target.name = patch.name;
        if (patch.syncMode !== undefined) target.syncMode = patch.syncMode;
        log.info("mock: folder updated", { id });
        emit("changed", { scope: "folders" });
        return delay(undefined);
    },

    pickDirectory(): Promise<string | null> {
        // The real OS dialog lands in a later task; the fake always "picks" a
        // fresh disjoint path (never cancels), so the create-folder flow is
        // demoable end to end. `null` on the wire is the cancelled case.
        return delay("/Volumes/Untitled/New Folder");
    },

    startImport(folderId: string): Promise<string> {
        // Still keyed to the legacy SOURCES seed: the import flow's mock rebinds to
        // the folder seed when the frontend-import epic consumes the new tree reads.
        const source = SOURCES.find((candidate) => candidate.ID === folderId);
        if (source === undefined) {
            return delay(folderId).then(() => {
                throw new ApiError("domain", `folder ${folderId} not found`, "not_found");
            });
        }
        if (source.Connectivity === "offline") {
            return delay(folderId).then(() => {
                throw new ApiError("domain", `volume for folder ${folderId} is offline`, "volume_offline");
            });
        }
        jobCounter += 1;
        const jobId = `mock-job-${String(jobCounter).padStart(4, "0")}`;
        log.info("mock: import started", { jobId, folderId });
        runMockImport(jobId);
        return delay(jobId);
    },

    cancelJob(jobId: string): Promise<void> {
        // No-op for an unknown or already-terminal job (matches the seam); a
        // running job's next tick sees the flag and emits the cancelled terminal.
        const active = runningImports.get(jobId);
        if (active !== undefined) active.cancelled = true;
        return delay(undefined);
    },

    subscribe(handler: (envelope: Envelope) => void): () => void {
        subscribers.add(handler);
        return () => {
            subscribers.delete(handler);
        };
    },
};
