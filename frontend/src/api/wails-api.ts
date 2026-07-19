// The Wails adapter — AlexandriaAPI over the generated seam bindings. Rows come
// back as the generated AssetRow model (C13); this adapter layers the two
// presentation facts on top (kind, thumbURL) and normalizes every rejection into
// ApiError. Nothing outside api/ imports this — the swap lives in client.ts.
//
// Type note: the generated d.ts under wailsjs/ mirrors Go's *typed* ast structs
// (untagged, capitalized fields), but the wire truth is ast/json.go's lowercase
// shadow structs — exactly our Query/Arrangement/Page shapes. Each binding is
// rebound once to that wire truth; the JSON that crosses is identical.

import type { ApiErrorKind, ErrorCode } from "@/_generated-types/errors";
import type { AssetRow as AssetRowModel } from "@/_generated-types/models";
import * as AssetServiceBinding from "../../wailsjs/go/seam/AssetService";
import type { AlexandriaAPI, Arrangement, AssetQueryResult, AssetRow, Page, Query } from "./contract";
import { ApiError } from "./contract";

const queryAssetsBound = AssetServiceBinding.QueryAssets as unknown as (
    query: Query,
    arrangement: Arrangement,
    page: Page,
) => Promise<{ items: AssetRowModel[]; total: number }>;

const assetIdSliceBound = AssetServiceBinding.AssetIDSlice as unknown as (
    query: Query,
    arrangement: Arrangement,
    fromIndex: number,
    toIndex: number,
) => Promise<string[]>;

const indexOfAssetBound = AssetServiceBinding.IndexOfAsset as unknown as (
    query: Query,
    arrangement: Arrangement,
    id: string,
) => Promise<number | null | undefined>;

// Runtime bridge for the types-only generated union (C10 completeness): a new
// kind in Go fails to compile here until it's added.
const API_ERROR_KINDS = {
    degraded: true,
    domain: true,
    transport: true,
    unexpected: true,
} as const satisfies Record<ApiErrorKind, true>;

function isApiErrorKind(value: unknown): value is ApiErrorKind {
    return typeof value === "string" && value in API_ERROR_KINDS;
}

/**
 * A bound-method rejection carries the Go error's Error() string. The seam's
 * ApiError JSON-encodes itself there (apierror.go), so the wire shape survives
 * the string channel; anything unparseable never left the seam properly and is
 * a transport failure by definition.
 */
export function toApiError(rejection: unknown): ApiError {
    if (rejection instanceof ApiError) return rejection;
    const text =
        typeof rejection === "string"
            ? rejection
            : rejection instanceof Error
              ? rejection.message
              : String(rejection);
    try {
        const parsed: unknown = JSON.parse(text);
        if (parsed !== null && typeof parsed === "object" && "kind" in parsed) {
            const { kind, code, detail } = parsed as { kind: unknown; code?: ErrorCode; detail?: string };
            if (isApiErrorKind(kind)) {
                return new ApiError(kind, detail ?? kind, code, detail);
            }
        }
    } catch {
        // Not the seam's JSON shape — fall through to transport.
    }
    return new ApiError("transport", text);
}

/**
 * Mirrors thumbnailer.Path: <catalog>/thumbnails/<size>/<2-char shard>/<id>.jpg,
 * served by the app host's asset-server fallback handler.
 * ponytail: 512 is the only generated tier (thumbnailer.New); a second tier
 * makes the size a parameter here, nothing else changes.
 */
function thumbnailURL(id: string): string {
    return `/thumbnails/512/${id.slice(0, 2)}/${id}.jpg`;
}

function toGridRow(row: AssetRowModel): AssetRow {
    return { ...row, kind: "asset", thumbURL: thumbnailURL(row.id) };
}

export const wailsApi: AlexandriaAPI = {
    async queryAssets(query: Query, arrangement: Arrangement, page: Page): Promise<AssetQueryResult> {
        try {
            const result = await queryAssetsBound(query, arrangement, page);
            return { items: result.items.map(toGridRow), total: result.total };
        } catch (rejection) {
            throw toApiError(rejection);
        }
    },

    async assetIdSlice(query: Query, arrangement: Arrangement, fromIndex: number, toIndex: number): Promise<string[]> {
        try {
            return await assetIdSliceBound(query, arrangement, fromIndex, toIndex);
        } catch (rejection) {
            throw toApiError(rejection);
        }
    },

    async indexOfAsset(query: Query, arrangement: Arrangement, id: string): Promise<number | null> {
        try {
            return (await indexOfAssetBound(query, arrangement, id)) ?? null;
        } catch (rejection) {
            throw toApiError(rejection);
        }
    },
};
