// The grid — the first content surface (frontend-architecture: bespoke on
// tanstack-virtual, NEVER RAC collections; the store owns selection). A pure
// renderer over the C2 state: it derives the query from the store, fetches
// through the one TanStack door, echoes working-set-changed back, and turns
// clicks into reducer actions. Rows are virtualized; columns derive from the
// measured container width; `total` sizes the scrollbar before blocks land.
//
// ponytail: chrome strings here are literals until the shell adopts i18n (C14),
// same sanction as app.tsx.

import { useVirtualizer } from "@tanstack/react-virtual";
import { useCallback, useEffect, useLayoutEffect, useRef, useState, type MouseEvent } from "react";
import { api } from "@/api/client";
import { type BlockModelOptions, useGridBlocks } from "@/api/queries";
import { Button } from "@/components/button/button";
import { log } from "@/lib/logger";
import { readCursorId, useCatalogDispatch, useCatalogQuery } from "@/stores/catalog-store";
import { GridCell } from "./grid-cell";
import { commitRange } from "./select-range";
import styles from "./grid.module.css";

// ponytail: one structural cell size; the density round mints the size-step
// tokens (task-31 disposition in ideation/frontend-token-gaps.md). Wider than the
// task-31 default (160) so the expanded cell's metadata header fits without heavy
// truncation — the density slider owns the real scale.
const TARGET_CELL_WIDTH = 210;

// The expanded cell adds fixed-height bands above and below the square thumbnail
// area — the metadata header (2 lines) + the rating footer. ponytail: structural
// until the density round tokenizes cell geometry (token-gaps item 2).
const CELL_BAND_HEIGHT = 52;

/** Columns for a measured width — exported for tests; 0-width (unmeasured) → 1. */
export function columnsForWidth(width: number): number {
    return Math.max(1, Math.floor(width / TARGET_CELL_WIDTH));
}

export function Grid({
    blockModelOptions,
}: {
    /** Test seam mirroring useGridBlocks' own: small blocks so the 64-row mock crosses block boundaries. */
    blockModelOptions?: BlockModelOptions;
} = {}) {
    const { query, arrangement } = useCatalogQuery();
    const { total, rowAt, localIndexOf, isPending, isError, refetch, setViewport } = useGridBlocks(
        query,
        arrangement,
        blockModelOptions,
    );
    const dispatch = useCatalogDispatch();

    // The data → store echo (frontend/09): seeds/clears the cursor with the
    // working set. The reducer no-ops when nothing changes, so this cannot loop.
    const firstId = rowAt(0)?.id ?? null;
    useEffect(() => {
        dispatch({ type: "working-set-changed", total, firstId });
    }, [total, firstId, dispatch]);

    const scrollReference = useRef<HTMLDivElement>(null);
    const [width, setWidth] = useState(0);
    useLayoutEffect(() => {
        const element = scrollReference.current;
        if (element === null) return;
        const measure = () => setWidth(element.clientWidth);
        measure();
        const observer = new ResizeObserver(measure);
        observer.observe(element);
        return () => observer.disconnect();
    }, []);

    const columns = columnsForWidth(width);
    const thumbEdge = width > 0 ? width / columns : TARGET_CELL_WIDTH;
    const rowHeight = thumbEdge + CELL_BAND_HEIGHT;
    const rowCount = Math.ceil(total / columns);

    const virtualizer = useVirtualizer({
        count: rowCount,
        getScrollElement: () => scrollReference.current,
        estimateSize: () => rowHeight,
        overscan: 3,
        initialRect: { width: 800, height: 600 },
    });
    // Column/row geometry changes (resize) invalidate every measured row.
    useEffect(() => {
        virtualizer.measure();
    }, [rowHeight, virtualizer]);

    // Report the visible working-set index span so the block model fetches the
    // viewport's blocks (+buffer). The virtual row range × columns → asset indices.
    const virtualItems = virtualizer.getVirtualItems();
    const firstVirtualRow = virtualItems[0]?.index ?? 0;
    const lastVirtualRow = virtualItems.at(-1)?.index ?? 0;
    useEffect(() => {
        if (total === 0) return;
        setViewport(firstVirtualRow * columns, lastVirtualRow * columns + (columns - 1));
    }, [firstVirtualRow, lastVirtualRow, columns, total, setViewport]);

    // A shift-click needs the cursor's index to name the range's other end. Resident
    // blocks answer locally; outside them the seam does (indexOfAsset) — the anchor
    // index no longer has to be on screen (the single-page model's constraint).
    const beginRange = useCallback(
        async (cursorId: string, clickedId: string, targetIndex: number) => {
            let anchorIndex: number | null;
            try {
                anchorIndex = localIndexOf(cursorId) ?? (await api.indexOfAsset(query, arrangement, cursorId));
            } catch (error) {
                // A failed seam lookup must not kill the gesture silently (the
                // select-range discipline): log loudly, then degrade below.
                log.error("grid: anchor index lookup failed — degrading to single select", { error: String(error) });
                anchorIndex = null;
            }
            if (anchorIndex === null) {
                // No anchor — the cursor's asset isn't placeable in this working
                // set — so the shift-click degrades to a plain single select.
                dispatch({ type: "asset-clicked", id: clickedId, additive: false });
                return;
            }
            await commitRange(api, query, arrangement, anchorIndex, targetIndex, dispatch);
        },
        [localIndexOf, query, arrangement, dispatch],
    );

    // The gesture layer: platform modifiers translate to semantic intent at the
    // DOM edge; the reducer owns what the intent MEANS. The cursor anchor is
    // read NON-reactively (readCursorId) so this handler — and with it every
    // memoized cell — keeps its identity across cursor moves: a click
    // re-renders two cells, not the viewport.
    const onCellClick = useCallback(
        (event: MouseEvent, id: string, index: number) => {
            const cursorId = readCursorId();
            if (event.shiftKey && cursorId !== null) {
                void beginRange(cursorId, id, index);
                return;
            }
            dispatch({ type: "asset-clicked", id, additive: event.metaKey || event.ctrlKey });
        },
        [beginRange, dispatch],
    );

    // The states render INSIDE the scroll container so the measured element is
    // unconditional — the one-shot measuring effect must find it on first
    // mount, before data lands (width 0 = a one-column grid, a real bug the
    // browser pass caught).
    let content = null;
    if (isPending) {
        content = <div className={styles.state}>Loading…</div>;
    } else if (isError) {
        content = (
            <div className={styles.state}>
                <div className={styles.stateStack}>
                    <span>The catalog didn’t answer.</span>
                    <Button onPress={() => void refetch()}>Retry</Button>
                </div>
            </div>
        );
    } else if (total === 0) {
        content = <div className={styles.state}>No assets match.</div>;
    }

    if (content !== null) {
        return (
            <div ref={scrollReference} className={styles.scroll}>
                {content}
            </div>
        );
    }

    return (
        <div ref={scrollReference} className={styles.scroll} role="grid" aria-rowcount={rowCount}>
            <div className={styles.canvas} style={{ height: virtualizer.getTotalSize() }}>
                {virtualItems.map((virtualRow) => (
                    <div
                        key={virtualRow.key}
                        role="row"
                        aria-rowindex={virtualRow.index + 1}
                        className={styles.row}
                        style={{
                            transform: `translateY(${String(virtualRow.start)}px)`,
                            height: virtualRow.size,
                            gridTemplateColumns: `repeat(${String(columns)}, 1fr)`,
                        }}
                    >
                        {Array.from({ length: columns }, (_, column) => {
                            const index = virtualRow.index * columns + column;
                            if (index >= total) return null;
                            return (
                                <GridCell
                                    key={index}
                                    row={rowAt(index)}
                                    index={index}
                                    onCellClick={onCellClick}
                                />
                            );
                        })}
                    </div>
                ))}
            </div>
        </div>
    );
}
