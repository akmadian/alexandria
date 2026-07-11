// The mock backend — a faithful `AlexandriaAPI` implementation with an in-memory
// AST query engine. `evaluate` is the SQL stand-in: it runs the same WhereNode
// tree the real compiler will, so filter / sort / paging genuinely work and the
// UI develops against real query behavior with no Wails, no Go. When the Wails
// adapter binds, the contract is unchanged — this file is simply not selected in
// ./client.ts.

import type { ColorLabel, FileStatus, FileType, Flag } from "@/_generated-types/enums";
import type { SortField, TokenField } from "@/_generated-types/vocabulary";
import type { Arrangement, Leaf, Query, WhereNode } from "@/query-model/ast";
import { isLeaf } from "@/query-model/ast";
import type { AlexandriaAPI, AssetQueryResult, AssetRow } from "./contract";

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
    sourceId: string;
    tagIds: string[];
    thumbURL: string;
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

const LABELS: (ColorLabel | null)[] = ["red", "yellow", "green", "blue", "purple", "orange", null, null, null];
const CAMERAS: [string, string][] = [
    ["Sony", "A7 IV"],
    ["Canon", "EOS R5"],
    ["Nikon", "Z8"],
    ["Fujifilm", "X-T5"],
    ["Leica", "Q3"],
];
const KIND: { type: FileType; ext: string; ratio: [number, number]; temporal: boolean }[] = [
    { type: "image", ext: "jpg", ratio: [3, 2], temporal: false },
    { type: "raw", ext: "arw", ratio: [3, 2], temporal: false },
    { type: "image", ext: "png", ratio: [1, 1], temporal: false },
    { type: "video", ext: "mp4", ratio: [16, 9], temporal: true },
    { type: "vector", ext: "svg", ratio: [4, 3], temporal: false },
];

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
            sourceId: `src-${i % 3}`,
            tagIds: [],
            thumbURL: thumbDataUri(i + 1, kind.ratio),
        });
    }
    return assets;
}

const CATALOG: MockAsset[] = seededAssets(64);

function toRow(asset: MockAsset): AssetRow {
    return {
        kind: "asset",
        id: asset.id,
        sourceId: asset.sourceId,
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
        relativePath: asset.filename,
        thumbURL: asset.thumbURL,
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
    source: (a) => a.sourceId,
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

// Scope narrowing (the extensional root). Vertical: library = all; collection/tag
// membership + folder paths arrive with the sidebar (widen).
function inScope(query: Query, asset: MockAsset): boolean {
    switch (query.scope.kind) {
        case "library":
            return true;
        case "folder":
            return asset.sourceId === query.scope.sourceId;
        case "collection":
        case "tag":
            return true; // membership tables land with the browser sidebar
    }
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
};
