// The optimistic-patch mechanics (frontend-architecture point 4): a patch reaches
// BOTH caches (list + detail), carries absolute values, and a snapshot restores
// the exact prior state on rollback. The pure patchers are tested directly; the
// QueryClient wrappers are tested against a real client (no network, no mock API).

import { QueryClient } from "@tanstack/react-query";
import { beforeEach, describe, expect, it } from "vitest";
import type { AssetDetail } from "@/_generated-types/models";
import { applyOptimisticPatch, patchDetail, patchRow, rollbackCatalog } from "./catalog-cache";
import type { AssetQueryResult, AssetRow } from "./contract";

function makeRow(id: string, over: Partial<AssetRow> = {}): AssetRow {
    return {
        kind: "asset",
        thumbURL: `/thumbnails/512/${id.slice(0, 2)}/${id}.jpg`,
        id,
        volumeId: "src-0",
        filename: `${id}.jpg`,
        fileType: "image",
        fileStatus: "online",
        rating: null,
        colorLabel: null,
        flag: null,
        width: 4000,
        height: 3000,
        durationSecs: null,
        cameraModel: "A7 IV",
        capturedAt: "2025-01-01T00:00:00Z",
        ingestedAt: "2026-06-01T00:00:00Z",
        thumbnailAt: null,
        relativePath: `2026/${id}.jpg`,
        sizeBytes: 1024,
        ...over,
    };
}

function makeDetail(id: string, over: Partial<AssetDetail> = {}): AssetDetail {
    return {
        id,
        volumeId: "src-0",
        filename: `${id}.jpg`,
        extension: "jpg",
        mimeType: "image/jpeg",
        fileType: "image",
        fileStatus: "online",
        relativePath: `2026/${id}.jpg`,
        sizeBytes: 1024,
        mtime: "2026-05-01T00:00:00Z",
        ingestedAt: "2026-06-01T00:00:00Z",
        width: 4000,
        height: 3000,
        durationSecs: null,
        capturedAt: "2025-01-01T00:00:00Z",
        cameraMake: "Sony",
        cameraModel: "A7 IV",
        lensModel: null,
        focalLengthMm: null,
        aperture: null,
        shutterSpeed: null,
        iso: null,
        gpsLat: null,
        gpsLon: null,
        colorSpace: null,
        bitDepth: null,
        title: null,
        caption: null,
        creator: null,
        copyright: null,
        rating: null,
        colorLabel: null,
        flag: null,
        note: null,
        ...over,
    };
}

describe("patchRow / patchDetail (pure)", () => {
    it("sets present fields, leaves absent fields untouched", () => {
        const patched = patchRow(makeRow("a", { rating: 2, colorLabel: "red" }), { rating: 5 });
        expect(patched.rating).toBe(5);
        expect(patched.colorLabel).toBe("red"); // absent in the patch
    });

    it("clears a field with an explicit null", () => {
        expect(patchRow(makeRow("a", { rating: 4 }), { rating: null }).rating).toBeNull();
    });

    it("patches note only on the detail (the row has no note)", () => {
        const detail = patchDetail(makeDetail("a"), { note: "check focus" });
        expect(detail.note).toBe("check focus");
        // patchRow accepts the note field but has nowhere to put it — no crash, row unchanged.
        expect("note" in patchRow(makeRow("a"), { note: "x" })).toBe(false);
    });

    it("does not mutate its input", () => {
        const row = makeRow("a", { rating: 1 });
        patchRow(row, { rating: 5 });
        expect(row.rating).toBe(1);
    });
});

describe("applyOptimisticPatch / rollbackCatalog (both caches)", () => {
    const listKey = ["assets", "serialized-query"];
    let queryClient: QueryClient;

    beforeEach(() => {
        queryClient = new QueryClient();
        const list: AssetQueryResult = { items: [makeRow("a", { rating: 1 }), makeRow("b", { rating: 2 })], total: 2 };
        queryClient.setQueryData(listKey, list);
        queryClient.setQueryData(["asset", "a"], makeDetail("a", { rating: 1 }));
    });

    it("patches the targeted row in the list cache and the detail cache", () => {
        applyOptimisticPatch(queryClient, ["a"], { rating: 5, flag: "pick" });

        const list = queryClient.getQueryData<AssetQueryResult>(listKey);
        expect(list?.items.find((row) => row.id === "a")?.rating).toBe(5);
        expect(list?.items.find((row) => row.id === "a")?.flag).toBe("pick");
        expect(list?.items.find((row) => row.id === "b")?.rating).toBe(2); // untargeted row untouched

        const detail = queryClient.getQueryData<AssetDetail>(["asset", "a"]);
        expect(detail?.rating).toBe(5);
        expect(detail?.flag).toBe("pick");
    });

    it("restores both caches exactly on rollback (forced-failure path)", () => {
        const snapshot = applyOptimisticPatch(queryClient, ["a"], { rating: 5 });
        expect(queryClient.getQueryData<AssetQueryResult>(listKey)?.items[0].rating).toBe(5);

        rollbackCatalog(queryClient, snapshot);
        expect(queryClient.getQueryData<AssetQueryResult>(listKey)?.items[0].rating).toBe(1);
        expect(queryClient.getQueryData<AssetDetail>(["asset", "a"])?.rating).toBe(1);
    });
});
