import { afterEach, describe, expect, it, vi } from "vitest";
import type { AssetRow as AssetRowModel } from "@/_generated-types/models";
import { DEFAULT_ARRANGEMENT } from "@/query-model/ast";
import { ApiError } from "./contract";
import type { Page, Query } from "./contract";
import { toApiError, wailsApi } from "./wails-api";

const LIBRARY: Query = { version: 1, scope: { kind: "library" }, where: null };
const PAGE: Page = { offset: 0, limit: 10 };

// The generated bindings read window['go'] at call time, so a per-test stub of
// the bridge is all it takes to fake the backend.
function stubAssetService(methods: Record<string, unknown>) {
    vi.stubGlobal("go", { seam: { AssetService: methods } });
}

function modelRow(id: string): AssetRowModel {
    return {
        id,
        sourceId: "source-1",
        filename: `${id}.jpg`,
        fileType: "image",
        fileStatus: "online",
        rating: null,
        colorLabel: null,
        flag: null,
        width: 800,
        height: 600,
        durationSecs: null,
        cameraModel: null,
        capturedAt: null,
        ingestedAt: "2026-07-01T00:00:00Z",
        thumbnailAt: "2026-07-01T00:00:00Z",
        relativePath: `${id}.jpg`,
        sizeBytes: 1024,
    };
}

afterEach(() => {
    vi.unstubAllGlobals();
});

describe("wailsApi", () => {
    it("maps rows onto grid rows: kind + sharded thumbURL", async () => {
        const queryAssets = vi.fn().mockResolvedValue({ items: [modelRow("abcd1234")], total: 1 });
        stubAssetService({ QueryAssets: queryAssets });

        const { items, total } = await wailsApi.queryAssets(LIBRARY, DEFAULT_ARRANGEMENT, PAGE);
        expect(total).toBe(1);
        expect(items[0].kind).toBe("asset");
        expect(items[0].thumbURL).toBe("/thumbnails/512/ab/abcd1234.jpg");
        expect(items[0].filename).toBe("abcd1234.jpg");
        expect(queryAssets).toHaveBeenCalledWith(LIBRARY, DEFAULT_ARRANGEMENT, PAGE);
    });

    it("normalizes a bound-method rejection into ApiError", async () => {
        stubAssetService({
            QueryAssets: vi.fn().mockRejectedValue('{"kind":"domain","code":"query_invalid","detail":"bad tree"}'),
        });

        const failure = await wailsApi.queryAssets(LIBRARY, DEFAULT_ARRANGEMENT, PAGE).catch((error: unknown) => error);
        expect(failure).toBeInstanceOf(ApiError);
        expect((failure as ApiError).kind).toBe("domain");
        expect((failure as ApiError).code).toBe("query_invalid");
    });

    it("passes assetIdSlice through untouched", async () => {
        const assetIdSlice = vi.fn().mockResolvedValue(["a", "b"]);
        stubAssetService({ AssetIDSlice: assetIdSlice });

        await expect(wailsApi.assetIdSlice(LIBRARY, DEFAULT_ARRANGEMENT, 0, 2)).resolves.toEqual(["a", "b"]);
        expect(assetIdSlice).toHaveBeenCalledWith(LIBRARY, DEFAULT_ARRANGEMENT, 0, 2);
    });

    it("normalizes rejections from the other two read methods identically", async () => {
        const rejection = '{"kind":"domain","code":"query_invalid","detail":"bad tree"}';
        stubAssetService({
            AssetIDSlice: vi.fn().mockRejectedValue(rejection),
            IndexOfAsset: vi.fn().mockRejectedValue(rejection),
        });

        const sliceFailure = await wailsApi.assetIdSlice(LIBRARY, DEFAULT_ARRANGEMENT, 0, 2).catch((error: unknown) => error);
        expect(sliceFailure).toBeInstanceOf(ApiError);
        expect((sliceFailure as ApiError).code).toBe("query_invalid");

        const indexFailure = await wailsApi.indexOfAsset(LIBRARY, DEFAULT_ARRANGEMENT, "id").catch((error: unknown) => error);
        expect(indexFailure).toBeInstanceOf(ApiError);
        expect((indexFailure as ApiError).code).toBe("query_invalid");
    });

    it("normalizes indexOfAsset's absent index to null", async () => {
        stubAssetService({ IndexOfAsset: vi.fn().mockResolvedValue(undefined) });
        await expect(wailsApi.indexOfAsset(LIBRARY, DEFAULT_ARRANGEMENT, "nope")).resolves.toBeNull();

        stubAssetService({ IndexOfAsset: vi.fn().mockResolvedValue(7) });
        await expect(wailsApi.indexOfAsset(LIBRARY, DEFAULT_ARRANGEMENT, "hit")).resolves.toBe(7);
    });

    it("passes getAsset through and normalizes its rejection", async () => {
        const detail = { id: "a1", filename: "a1.raf" };
        const getAsset = vi.fn().mockResolvedValue(detail);
        stubAssetService({ GetAsset: getAsset });
        await expect(wailsApi.getAsset("a1")).resolves.toBe(detail);
        expect(getAsset).toHaveBeenCalledWith("a1");

        stubAssetService({
            GetAsset: vi.fn().mockRejectedValue('{"kind":"domain","code":"not_found","detail":"asset a1"}'),
        });
        const failure = await wailsApi.getAsset("a1").catch((error: unknown) => error);
        expect(failure).toBeInstanceOf(ApiError);
        expect((failure as ApiError).code).toBe("not_found");
    });

    it("forwards updateAssets target + patch to the binding unchanged", async () => {
        const updateAssets = vi.fn().mockResolvedValue(undefined);
        stubAssetService({ UpdateAssets: updateAssets });
        await expect(wailsApi.updateAssets({ ids: ["a", "b"] }, { rating: 3, colorLabel: null })).resolves.toBeUndefined();
        // The contract shape crosses verbatim — a present key is a value or null,
        // absent keys are omitted; Go's RawMessage fields decode the three states.
        expect(updateAssets).toHaveBeenCalledWith({ ids: ["a", "b"] }, { rating: 3, colorLabel: null });
    });

    it("normalizes an updateAssets rejection into ApiError", async () => {
        stubAssetService({
            UpdateAssets: vi.fn().mockRejectedValue('{"kind":"domain","code":"not_found","detail":"asset z"}'),
        });
        const failure = await wailsApi.updateAssets({ ids: ["z"] }, { rating: 1 }).catch((error: unknown) => error);
        expect(failure).toBeInstanceOf(ApiError);
        expect((failure as ApiError).code).toBe("not_found");
    });
});

describe("wailsApi sources, import, and events", () => {
    it("lists and creates sources through the SourceService bindings", async () => {
        const source = { ID: "src-9", Name: "New", Kind: "local", BasePath: "/x" };
        const listSources = vi.fn().mockResolvedValue([source]);
        const createSource = vi.fn().mockResolvedValue(source);
        vi.stubGlobal("go", { seam: { SourceService: { ListSources: listSources, CreateSource: createSource } } });

        await expect(wailsApi.listSources()).resolves.toEqual([source]);
        const input = { name: "New", basePath: "/x" };
        await expect(wailsApi.createSource(input)).resolves.toBe(source);
        expect(createSource).toHaveBeenCalledWith(input);
    });

    it("starts and cancels imports through the ImportService bindings", async () => {
        const startImport = vi.fn().mockResolvedValue("job-42");
        const cancelJob = vi.fn().mockResolvedValue(undefined);
        vi.stubGlobal("go", { seam: { ImportService: { StartImport: startImport, CancelJob: cancelJob } } });

        await expect(wailsApi.startImport("src-0")).resolves.toBe("job-42");
        expect(startImport).toHaveBeenCalledWith("src-0");
        await expect(wailsApi.cancelJob("job-42")).resolves.toBeUndefined();
        expect(cancelJob).toHaveBeenCalledWith("job-42");
    });

    it("normalizes a source rejection into ApiError", async () => {
        vi.stubGlobal("go", {
            seam: { ImportService: { StartImport: vi.fn().mockRejectedValue('{"kind":"domain","code":"source_offline","detail":"src-0"}') } },
        });
        const failure = await wailsApi.startImport("src-0").catch((error: unknown) => error);
        expect(failure).toBeInstanceOf(ApiError);
        expect((failure as ApiError).code).toBe("source_offline");
    });

    it("subscribes one listener per topic channel and tears them all down", () => {
        // Wails' EventsOn delegates to EventsOnMultiple(name, cb, -1) on the
        // runtime global, so that's the seam to stub.
        const unsubscribe = vi.fn();
        const eventsOnMultiple =
            vi.fn<(name: string, callback: (data: unknown) => void, max: number) => () => void>().mockReturnValue(unsubscribe);
        vi.stubGlobal("runtime", { EventsOnMultiple: eventsOnMultiple });

        const handler = vi.fn();
        const teardown = wailsApi.subscribe(handler);
        // One listener per topic: catalog, jobs, watcher, sync.
        expect(eventsOnMultiple).toHaveBeenCalledTimes(4);
        expect(eventsOnMultiple.mock.calls.map((call) => call[0])).toEqual(["catalog", "jobs", "watcher", "sync"]);

        // The channel callback forwards the whole envelope to the handler.
        const envelope = { topic: "jobs", type: "progress", payload: { jobId: "j1" }, timestamp: "t" };
        (eventsOnMultiple.mock.calls[1][1] as (data: unknown) => void)(envelope);
        expect(handler).toHaveBeenCalledWith(envelope);

        teardown();
        expect(unsubscribe).toHaveBeenCalledTimes(4);
    });
});

describe("toApiError", () => {
    it("parses the seam's JSON error shape from a string rejection", () => {
        const error = toApiError('{"kind":"degraded","detail":"engine catching up"}');
        expect(error.kind).toBe("degraded");
        expect(error.detail).toBe("engine catching up");
    });

    it("parses the shape out of an Error's message", () => {
        const error = toApiError(new Error('{"kind":"unexpected","detail":"boom"}'));
        expect(error.kind).toBe("unexpected");
    });

    it("treats non-JSON rejections as transport failures", () => {
        const error = toApiError("connection lost");
        expect(error.kind).toBe("transport");
        expect(error.message).toBe("connection lost");
    });

    it("treats an unknown kind as transport (forward-compatible enums)", () => {
        const error = toApiError('{"kind":"brand-new-kind","detail":"x"}');
        expect(error.kind).toBe("transport");
    });
});
