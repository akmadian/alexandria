// useUpdateAssets end to end (frontend-architecture §Optimistic mutation): the
// happy path leaves the optimistic patch standing; a forced failure — after the
// quiet retry — rolls BOTH caches back and raises a loud notice. The seam is
// mocked at the client boundary so this exercises the hook's own discipline
// (onMutate patch → onError rollback → notice), not the mock engine.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AssetDetail } from "@/_generated-types/models";
import type { AssetQueryResult, AssetRow } from "./contract";

vi.mock("./client", () => ({ api: { updateAssets: vi.fn() } }));
vi.mock("@/stores/notices", () => ({ pushNotice: vi.fn() }));

import { api } from "./client";
import { ApiError } from "./contract";
import { resetLaneForTests } from "./mutation-lane";
import { useUpdateAssets } from "./mutations";
import { pushNotice } from "@/stores/notices";

const updateAssets = vi.mocked(api.updateAssets);
const notice = vi.mocked(pushNotice);

const LIST_KEY = ["assets", "q"];
const ROW: AssetRow = {
    kind: "asset",
    thumbURL: "/thumbnails/512/aa/a.jpg",
    id: "a",
    sourceId: "src-0",
    filename: "a.jpg",
    fileType: "image",
    fileStatus: "online",
    rating: 1,
    colorLabel: null,
    flag: null,
    width: 4000,
    height: 3000,
    durationSecs: null,
    cameraModel: null,
    capturedAt: null,
    ingestedAt: "2026-06-01T00:00:00Z",
    thumbnailAt: null,
    relativePath: "a.jpg",
    sizeBytes: 1024,
};
const DETAIL = { id: "a", rating: 1, colorLabel: null, flag: null, note: null } as unknown as AssetDetail;

function makeClient(): QueryClient {
    const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
    queryClient.setQueryData<AssetQueryResult>(LIST_KEY, { items: [{ ...ROW }], total: 1 });
    queryClient.setQueryData<AssetDetail>(["asset", "a"], { ...DETAIL });
    return queryClient;
}

function rowRating(queryClient: QueryClient): number | null {
    return queryClient.getQueryData<AssetQueryResult>(LIST_KEY)?.items[0].rating ?? null;
}

beforeEach(() => {
    resetLaneForTests();
    updateAssets.mockReset();
    notice.mockReset();
});
afterEach(() => {
    vi.useRealTimers();
});

describe("useUpdateAssets", () => {
    it("patches both caches optimistically and keeps them on success", async () => {
        updateAssets.mockResolvedValue(undefined);
        const queryClient = makeClient();
        const wrapper = ({ children }: { children: ReactNode }) => (
            <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        );
        const { result } = renderHook(() => useUpdateAssets(), { wrapper });

        act(() => result.current.writeTriage({ ids: ["a"] }, { rating: 5 }));

        await waitFor(() => expect(rowRating(queryClient)).toBe(5));
        expect(queryClient.getQueryData<AssetDetail>(["asset", "a"])?.rating).toBe(5);
        await waitFor(() => expect(updateAssets).toHaveBeenCalledWith({ ids: ["a"] }, { rating: 5 }));
        expect(notice).not.toHaveBeenCalled();
    });

    it("rolls both caches back and raises a notice when the write fails after retries", async () => {
        updateAssets.mockRejectedValue(new Error("boom"));
        const queryClient = makeClient();
        const wrapper = ({ children }: { children: ReactNode }) => (
            <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        );
        const { result } = renderHook(() => useUpdateAssets(), { wrapper });

        act(() => result.current.writeTriage({ ids: ["a"] }, { rating: 5 }));

        // The optimistic value shows first…
        await waitFor(() => expect(rowRating(queryClient)).toBe(5));
        // …then the write exhausts its retries and reverts, loudly.
        await waitFor(() => expect(rowRating(queryClient)).toBe(1));
        expect(queryClient.getQueryData<AssetDetail>(["asset", "a"])?.rating).toBe(1);
        expect(notice).toHaveBeenCalledWith("errors.writeFailed");
        expect(updateAssets.mock.calls.length).toBeGreaterThanOrEqual(2); // original + at least one retry
    });

    it("does not retry a deterministic domain rejection — one call, straight to rollback", async () => {
        updateAssets.mockRejectedValue(new ApiError("domain", "no such asset", "not_found"));
        const queryClient = makeClient();
        const wrapper = ({ children }: { children: ReactNode }) => (
            <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        );
        const { result } = renderHook(() => useUpdateAssets(), { wrapper });

        act(() => result.current.writeTriage({ ids: ["a"] }, { rating: 5 }));

        await waitFor(() => expect(rowRating(queryClient)).toBe(1)); // rolled back
        expect(updateAssets).toHaveBeenCalledTimes(1); // a retry cannot fix not_found
        expect(notice).toHaveBeenCalledWith("errors.writeFailed");
    });

    it("skips the rollback when another write is in flight (concurrent optimism survives)", async () => {
        // Write 1 fails (domain — fast, no retry); write 2 succeeds behind it in
        // the lane. Write 1's snapshot predates write 2's optimistic flag patch,
        // so restoring it would clobber that patch — the onError guard skips the
        // revert (the settle-gate invalidation reconciles instead) but stays loud.
        updateAssets
            .mockRejectedValueOnce(new ApiError("domain", "no such asset", "not_found"))
            .mockResolvedValue(undefined);
        const queryClient = makeClient();
        const wrapper = ({ children }: { children: ReactNode }) => (
            <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        );
        const { result } = renderHook(() => useUpdateAssets(), { wrapper });

        act(() => {
            result.current.writeTriage({ ids: ["a"] }, { rating: 5 });
            result.current.writeTriage({ ids: ["a"] }, { flag: "pick" });
        });

        await waitFor(() => expect(updateAssets).toHaveBeenCalledTimes(2));
        await waitFor(() => expect(notice).toHaveBeenCalledTimes(1));
        // Write 2's optimism survived write 1's failure; nothing snapped back to
        // the pre-write-1 world.
        const row = queryClient.getQueryData<AssetQueryResult>(LIST_KEY)?.items[0];
        expect(row?.flag).toBe("pick");
        expect(row?.rating).toBe(5); // stale optimism — the gate's refetch owns reconciling it
    });
});
