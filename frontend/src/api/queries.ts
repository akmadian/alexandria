// TanStack read hooks — the only door features use to reach the backend.
//
// The grid reads its working set as an AG-Grid-style infinite row model
// (frontend-architecture §Fetching and performance): fixed-size BLOCKS keyed by
// (query+arrangement, blockIndex) via useQueries. Only the viewport + a buffer
// stays resident; an LRU cap bounds memory; `total` — carried on every block —
// sizes the scrollbar before the off-screen blocks land, so the grid is random-
// access, never a linear useInfiniteQuery. Arrangement is in the key because a
// block is a window into an ORDERED result (C4): client-side re-sorting is
// impossible, we never hold the whole set.

import { keepPreviousData, useQueries, useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";
import { log } from "@/lib/logger";
import type { Arrangement, Page, Query } from "@/query-model/ast";
import { serializeQuery } from "@/query-model/serialize";
import { api } from "./client";
import type { AssetQueryResult, AssetRow } from "./contract";

// Production block geometry. BLOCK_SIZE mirrors AG-Grid's cacheBlockSize default
// (a screenful is tens of rows; 100 keeps fetch counts low without over-reading a
// wide window); BUFFER_BLOCKS prefetches one block past each viewport edge so a
// slow scroll never exposes a placeholder; RESIDENT_CAP bounds resident rows
// (~1200) and LRU-evicts beyond it; VIEWPORT_DEBOUNCE_MS coalesces the block set
// during a fling so a fast scroll fires one fetch wave at rest, not one per
// intermediate row. Tests inject smaller values (BlockModelOptions).
const BLOCK_SIZE = 100;
const BUFFER_BLOCKS = 1;
const RESIDENT_CAP = 12;
const VIEWPORT_DEBOUNCE_MS = 120;

export interface BlockModelOptions {
    blockSize?: number;
    bufferBlocks?: number;
    residentCap?: number;
    debounceMs?: number;
}

export interface GridBlocks {
    /** Rows matching the query, ignoring paging — sizes the scrollbar. 0 until block 0 lands. */
    total: number;
    /** The row at a working-set index, or undefined when its block isn't resident (placeholder mat). */
    rowAt: (index: number) => AssetRow | undefined;
    /** The index of an id WITHIN the resident blocks, or null when it isn't loaded (ask the seam). */
    localIndexOf: (id: string) => number | null;
    /** No block has resolved yet — nothing to render but the loading state. */
    isPending: boolean;
    /** The first fetch failed before any total was known — the pane's error state. */
    isError: boolean;
    /** Refetch every resident block — the error state's manual retry. */
    refetch: () => void;
    /** Report the visible working-set index span; fetches viewport + buffer blocks (debounced). */
    setViewport: (startIndex: number, endIndex: number) => void;
}

/** Block indices covering [startIndex, endIndex] plus `bufferBlocks` each side, clamped to `total`. Pure. */
export function blocksForRange(
    startIndex: number,
    endIndex: number,
    blockSize: number,
    bufferBlocks: number,
    total: number,
): number[] {
    if (endIndex < startIndex || endIndex < 0) return [];
    const maxBlock = total > 0 ? Math.ceil(total / blockSize) - 1 : 0;
    const firstBlock = Math.max(0, Math.floor(Math.max(0, startIndex) / blockSize) - bufferBlocks);
    const lastBlock = Math.min(maxBlock, Math.floor(endIndex / blockSize) + bufferBlocks);
    const blocks: number[] = [];
    for (let block = firstBlock; block <= lastBlock; block++) blocks.push(block);
    return blocks;
}

/**
 * Merge `desired` into the LRU list (`current`, most-recent LAST), capping at `cap`. Desired
 * blocks are touched to the MRU end so the viewport survives eviction; the oldest untouched
 * blocks fall off the front when over the cap. Pure — the caller bails on an unchanged set.
 */
export function reconcileResidentBlocks(current: readonly number[], desired: readonly number[], cap: number): number[] {
    const desiredSet = new Set(desired);
    const kept = current.filter((block) => !desiredSet.has(block));
    const merged = [...kept, ...desired];
    return merged.length > cap ? merged.slice(merged.length - cap) : merged;
}

async function fetchBlock(
    query: Query,
    arrangement: Arrangement,
    blockIndex: number,
    blockSize: number,
): Promise<AssetQueryResult> {
    const page: Page = { offset: blockIndex * blockSize, limit: blockSize };
    try {
        const result = await api.queryAssets(query, arrangement, page);
        // Block 0 carries the working-set resolution (the milestone, at Info); the
        // rest is per-scroll play-by-play at Debug (frontend/CLAUDE §6).
        log[blockIndex === 0 ? "info" : "debug"]("api: block resolved", {
            blockIndex,
            total: result.total,
            returned: result.items.length,
        });
        return result;
    } catch (error) {
        log.error("api: block fetch failed", { blockIndex, error: String(error) });
        throw error;
    }
}

/**
 * The full-asset detail read — the inspector's server state, keyed by id so a
 * revisited subject is a cache hit. `keepPreviousData` holds the outgoing
 * asset's rows on screen during arrow-key navigation (no flicker); `enabled`
 * gates the fetch off while no cursor exists (empty working set).
 */
export function useAsset(id: string | null) {
    return useQuery({
        queryKey: ["asset", id],
        enabled: id !== null,
        placeholderData: keepPreviousData,
        queryFn: async () => {
            if (id === null) throw new Error("useAsset queryFn ran without an id");
            try {
                const detail = await api.getAsset(id);
                log.debug("api: getAsset resolved", { id });
                return detail;
            } catch (error) {
                log.error("api: getAsset failed", { id, error: String(error) });
                throw error;
            }
        },
    });
}

/**
 * The working-set total for consumers that need only the count (the shell's
 * header). Keyed identically to the grid's block 0, so it shares that fetch —
 * one round trip answers both.
 */
export function useAssetTotal(query: Query, arrangement: Arrangement): number | undefined {
    const key = serializeQuery(query, arrangement);
    const { data } = useQuery({
        queryKey: ["assets", key, "block", BLOCK_SIZE, 0],
        queryFn: () => fetchBlock(query, arrangement, 0, BLOCK_SIZE),
    });
    return data?.total;
}

/**
 * The grid's block-model read. Subscribes to the resident block set via useQueries
 * and exposes a random-access face over it: `rowAt` resolves a working-set index to
 * its row (or undefined for a not-yet-resident block), `total` sizes the scrollbar,
 * `setViewport` feeds the residency LRU as the grid scrolls.
 */
export function useGridBlocks(query: Query, arrangement: Arrangement, options: BlockModelOptions = {}): GridBlocks {
    const {
        blockSize = BLOCK_SIZE,
        bufferBlocks = BUFFER_BLOCKS,
        residentCap = RESIDENT_CAP,
        debounceMs = VIEWPORT_DEBOUNCE_MS,
    } = options;
    const key = serializeQuery(query, arrangement);

    // Residency is stored WITH the query key so a query change resets to the anchor
    // block atomically — no during-render setState, no stale-key subscription window.
    const [residency, setResidency] = useState<{ key: string; blocks: readonly number[] }>({ key, blocks: [0] });
    const resident = residency.key === key ? residency.blocks : [0];

    const results = useQueries({
        queries: resident.map((blockIndex) => ({
            // Block geometry is part of a block's identity: the same index at a
            // different size is a different window, so size is in the key (tests
            // run small blocks against the same query; production size is constant).
            queryKey: ["assets", key, "block", blockSize, blockIndex] as const,
            queryFn: () => fetchBlock(query, arrangement, blockIndex, blockSize),
        })),
    });

    // Total persists across the brief all-pending window a fling opens (every block
    // reports the same total; hold the last so the scrollbar never collapses). The
    // ref is a cache keyed by query — reset with a render-time write, not state.
    const lastTotal = useRef<{ key: string; total: number | null }>({ key, total: null });
    if (lastTotal.current.key !== key) lastTotal.current = { key, total: null };
    const reported = results.find((result) => result.data !== undefined)?.data?.total;
    if (reported !== undefined) lastTotal.current.total = reported;
    const total = lastTotal.current.total ?? 0;
    const anyError = results.some((result) => result.isError);
    const isPending = lastTotal.current.total === null && !anyError;
    // ponytail: ANY resident block failure escalates to the pane-level error
    // state + manual Retry — the pre-widen semantics, honoring the retry policy
    // (frontend-architecture §Fetching: "everything degrades to a rendered
    // state" with manual retry; reads are retry:false, so silent placeholder
    // mats would sit unrecoverable forever). Ceiling: one transient mid-scroll
    // failure flattens the whole grid to the error pane. TRIGGER: per-block
    // error mats + targeted retry once a notice/toast surface exists.
    const isError = anyError;

    // block index → its rows, for this render's rowAt; also mirrored to a ref so the
    // gesture-time localIndexOf reads current blocks WITHOUT re-subscribing the click
    // handler (the readCursorId non-reactive pattern, applied to block data).
    const blocks = new Map<number, readonly AssetRow[]>();
    resident.forEach((blockIndex, index) => {
        const data = results[index]?.data;
        if (data !== undefined) blocks.set(blockIndex, data.items);
    });
    const blocksReference = useRef(blocks);
    blocksReference.current = blocks;

    const rowAt = (index: number): AssetRow | undefined => {
        const blockIndex = Math.floor(index / blockSize);
        return blocks.get(blockIndex)?.[index - blockIndex * blockSize];
    };

    const localIndexOf = useCallback(
        (id: string): number | null => {
            for (const [blockIndex, items] of blocksReference.current) {
                const offset = items.findIndex((row) => row.id === id);
                if (offset !== -1) return blockIndex * blockSize + offset;
            }
            return null;
        },
        [blockSize],
    );

    // The grid reports its visible span as plain state (a stable setter, no memoized
    // ref-mutation); the debounce and residency update live in the effect below,
    // where a timer belongs.
    const [viewport, setViewportState] = useState<{ start: number; end: number } | null>(null);
    const setViewport = useCallback((startIndex: number, endIndex: number) => {
        setViewportState((previous) =>
            previous?.start === startIndex && previous.end === endIndex ? previous : { start: startIndex, end: endIndex },
        );
    }, []);

    useEffect(() => {
        if (viewport === null) return;
        const desired = blocksForRange(viewport.start, viewport.end, blockSize, bufferBlocks, total);
        if (desired.length === 0) return;
        // Deferred through the timer (debounceMs 0 still defers a macrotask), so a
        // fling coalesces to one fetch wave at rest and the effect never sets state
        // synchronously.
        const timer = setTimeout(() => {
            setResidency((previous) => {
                const base = previous.key === key ? previous.blocks : [0];
                const next = reconcileResidentBlocks(base, desired, residentCap);
                const unchanged =
                    previous.key === key &&
                    next.length === base.length &&
                    next.every((block, index) => block === base[index]);
                return unchanged ? previous : { key, blocks: next };
            });
        }, debounceMs);
        return () => clearTimeout(timer);
    }, [viewport, total, key, blockSize, bufferBlocks, residentCap, debounceMs]);

    // A plain closure, deliberately: `results` is a fresh array every render, so
    // a useCallback here would never actually memoize — don't ship a memo that
    // can't hold. Covers every resident block, errored ones included (Retry).
    const refetch = () => {
        for (const result of results) void result.refetch();
    };

    return { total, rowAt, localIndexOf, isPending, isError, refetch, setViewport };
}
