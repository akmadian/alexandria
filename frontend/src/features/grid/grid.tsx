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
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type MouseEvent } from "react";
import { api } from "@/api/client";
import { useQueryAssets } from "@/api/queries";
import { Button } from "@/components/button/button";
import { useCatalogDispatch, useCatalogQuery, useCursorId } from "@/stores/catalog-store";
import { GridCell } from "./grid-cell";
import { commitRange } from "./select-range";
import styles from "./grid.module.css";

// ponytail: one structural cell size; the density round mints the size-step
// tokens (task-31 disposition in ideation/frontend-token-gaps.md).
const TARGET_CELL_WIDTH = 160;

/** Columns for a measured width — exported for tests; 0-width (unmeasured) → 1. */
export function columnsForWidth(width: number): number {
    return Math.max(1, Math.floor(width / TARGET_CELL_WIDTH));
}

export function Grid() {
    const { query, arrangement } = useCatalogQuery();
    const { data, isPending, isError, refetch } = useQueryAssets(query, arrangement);
    const dispatch = useCatalogDispatch();
    const cursorId = useCursorId();

    // The data → store echo (frontend/09): seeds/clears the cursor with the
    // working set. The reducer no-ops when nothing changes, so this cannot loop.
    useEffect(() => {
        if (data === undefined) return;
        dispatch({
            type: "working-set-changed",
            total: data.total,
            firstId: data.items[0]?.id ?? null,
        });
    }, [data, dispatch]);

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

    const total = data?.total ?? 0;
    const items = useMemo(() => data?.items ?? [], [data]);
    const columns = columnsForWidth(width);
    const rowHeight = width > 0 ? width / columns : TARGET_CELL_WIDTH;
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

    const onCellClick = useCallback(
        (event: MouseEvent, id: string, index: number) => {
            if (event.shiftKey && cursorId !== null) {
                // ponytail: anchor index from the loaded page (single-page model);
                // the block-model widen swaps this findIndex for IndexOfAsset.
                const anchorIndex = items.findIndex((row) => row.id === cursorId);
                if (anchorIndex !== -1) {
                    void commitRange(api, query, arrangement, anchorIndex, index, dispatch);
                    return;
                }
            }
            dispatch({ type: "asset-clicked", id, additive: event.metaKey || event.ctrlKey });
        },
        [cursorId, items, query, arrangement, dispatch],
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
                {virtualizer.getVirtualItems().map((virtualRow) => (
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
                                    row={items[index]}
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
