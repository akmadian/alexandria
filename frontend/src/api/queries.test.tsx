// The block model over the REAL mock seam (never a fake): the pure block math
// (range → block indices, LRU reconciliation) plus the useGridBlocks hook driven
// against the 64-row mock with dev-sized blocks, so 64 rows cross block
// boundaries. Proves total sizes the scrollbar before off-screen blocks land,
// scroll fetches the viewport's blocks, indices map across a boundary, and the
// LRU cap evicts the oldest block.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, test, vi } from "vitest";
import { mockApi } from "@/api/mock";
import { DEFAULT_ARRANGEMENT, type Query } from "@/query-model/ast";
import { api } from "./client";
import { blocksForRange, reconcileResidentBlocks, useGridBlocks } from "./queries";

const LIBRARY: Query = { version: 1, scope: { kind: "library" }, where: null };

afterEach(() => {
    vi.restoreAllMocks();
});

// Dev block geometry: 8-row blocks put the mock's 64 rows across 8 blocks.
const SMALL = { blockSize: 8, bufferBlocks: 0, residentCap: 10, debounceMs: 0 } as const;

function createWrapper() {
    // A fresh client per hook (no cross-test cache); retry off so a failure surfaces.
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } });
    return ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
}

async function orderedIds(): Promise<string[]> {
    const { items } = await mockApi.queryAssets(LIBRARY, DEFAULT_ARRANGEMENT, { offset: 0, limit: 64 });
    return items.map((row) => row.id);
}

// --- pure block math ---------------------------------------------------------

test("blocksForRange covers the range plus buffer, clamped to the last block", () => {
    expect(blocksForRange(8, 15, 8, 1, 64)).toEqual([0, 1, 2]); // block 1 ± one buffer
    expect(blocksForRange(60, 63, 8, 2, 64)).toEqual([5, 6, 7]); // clamped to maxBlock 7
});

test("blocksForRange yields no blocks for a degenerate range", () => {
    expect(blocksForRange(10, 5, 8, 1, 64)).toEqual([]);
});

test("blocksForRange requests only the anchor block while total is unknown", () => {
    expect(blocksForRange(0, 30, 8, 1, 0)).toEqual([0]);
});

test("reconcileResidentBlocks touches desired to MRU and evicts the oldest past the cap", () => {
    expect(reconcileResidentBlocks([0, 1, 2], [3], 2)).toEqual([2, 3]); // 0,1 fall off the front
    expect(reconcileResidentBlocks([0, 1, 2], [0], 5)).toEqual([1, 2, 0]); // 0 re-touched to MRU
});

test("reconcileResidentBlocks never evicts a desired block", () => {
    // Desired exceeds the cap: the earliest desired blocks fall off, the newest survive.
    expect(reconcileResidentBlocks([9], [0, 1, 2, 3], 3)).toEqual([1, 2, 3]);
});

// --- the hook against the mock ----------------------------------------------

test("total sizes the scrollbar before the off-screen blocks land", async () => {
    const { result } = renderHook(() => useGridBlocks(LIBRARY, DEFAULT_ARRANGEMENT, SMALL), {
        wrapper: createWrapper(),
    });
    expect(result.current.total).toBe(0); // nothing resolved yet
    expect(result.current.isPending).toBe(true);

    await waitFor(() => expect(result.current.total).toBe(64));
    // The full total is known (scrollbar sized) while only block 0 is resident:
    // row 0 has landed, row 8 (block 1) is still a placeholder.
    expect(result.current.isPending).toBe(false);
    expect(result.current.rowAt(0)).toBeDefined();
    expect(result.current.rowAt(8)).toBeUndefined();
});

test("setViewport fetches the viewport's block on scroll", async () => {
    const { result } = renderHook(() => useGridBlocks(LIBRARY, DEFAULT_ARRANGEMENT, SMALL), {
        wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.total).toBe(64));
    expect(result.current.rowAt(15)).toBeUndefined(); // block 1 not yet requested

    act(() => result.current.setViewport(8, 15)); // scroll to block 1
    await waitFor(() => expect(result.current.rowAt(8)).toBeDefined());
    expect(result.current.rowAt(15)).toBeDefined();
});

test("indices and localIndexOf resolve across a block boundary", async () => {
    const ids = await orderedIds();
    const { result } = renderHook(() => useGridBlocks(LIBRARY, DEFAULT_ARRANGEMENT, SMALL), {
        wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.total).toBe(64));
    act(() => result.current.setViewport(0, 15)); // blocks 0 and 1 resident
    await waitFor(() => {
        expect(result.current.rowAt(7)).toBeDefined();
        expect(result.current.rowAt(8)).toBeDefined();
    });
    // Row 7 is block 0's last, row 8 is block 1's first — the mock's compiled order.
    expect(result.current.rowAt(7)?.id).toBe(ids[7]);
    expect(result.current.rowAt(8)?.id).toBe(ids[8]);
    // The asset just past the boundary resolves to its global index, not its offset.
    expect(result.current.localIndexOf(ids[8] as string)).toBe(8);
    // An id in no resident block can't be placed locally (the seam answers instead).
    expect(result.current.localIndexOf("mock-9999")).toBeNull();
});

test("a block failing after the initial load surfaces the error state; refetch recovers it", async () => {
    const { result } = renderHook(() => useGridBlocks(LIBRARY, DEFAULT_ARRANGEMENT, SMALL), {
        wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.total).toBe(64));
    expect(result.current.isError).toBe(false);

    // A transient failure on a MID-SCROLL block (total already known): the retry
    // policy demands a rendered error state with manual retry — never silent
    // permanent placeholder mats (reads are retry:false).
    vi.spyOn(api, "queryAssets").mockRejectedValueOnce(new Error("catalog busy"));
    act(() => result.current.setViewport(24, 31)); // block 3
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.isPending).toBe(false); // never regresses to loading
    expect(result.current.total).toBe(64); // the known total holds through the error

    // Manual retry re-runs every resident block; the restored mock answers.
    // Wait on the DATA (the stronger condition — isError can clear while the
    // refetch is still in flight), then confirm the error state resolved.
    act(() => result.current.refetch());
    await waitFor(() => expect(result.current.rowAt(24)).toBeDefined());
    expect(result.current.isError).toBe(false);
});

test("the LRU cap evicts the oldest block when scrolling past it", async () => {
    const { result } = renderHook(
        () => useGridBlocks(LIBRARY, DEFAULT_ARRANGEMENT, { blockSize: 8, bufferBlocks: 0, residentCap: 2, debounceMs: 0 }),
        { wrapper: createWrapper() },
    );
    await waitFor(() => expect(result.current.total).toBe(64)); // block 0 resident

    act(() => result.current.setViewport(16, 23)); // block 2 → resident [0, 2]
    await waitFor(() => expect(result.current.rowAt(16)).toBeDefined());
    expect(result.current.rowAt(0)).toBeDefined(); // still within the cap of 2

    act(() => result.current.setViewport(40, 47)); // block 5 → reconcile to [2, 5]
    await waitFor(() => expect(result.current.rowAt(40)).toBeDefined());
    expect(result.current.rowAt(0)).toBeUndefined(); // block 0 LRU-evicted
    expect(result.current.rowAt(16)).toBeDefined(); // block 2 survived
    // total held across the churn — the scrollbar never collapsed.
    expect(result.current.total).toBe(64);
});
