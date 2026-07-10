// The grid: a bespoke content surface (NOT React Aria — store-owned selection),
// virtualized rows of DS GridCells over the mock's real queryAssets result. Mouse
// and keyboard dispatch the same catalog-store actions.
//
// ponytail: the whole result set arrives in one page (64 mock rows). The windowed
// block model (fetch on scroll, keyed by visible range) is the widen step and
// lands in the api/queries hook — this component keeps its shape.

import { useVirtualizer } from "@tanstack/react-virtual";
import { type KeyboardEvent, type PointerEvent, useCallback, useRef, useState } from "react";
import type { AssetRow } from "@/api/contract";
import { useQueryAssets } from "@/api/queries";
import { useCatalogDispatch, useCatalogQuery, useCursorId } from "@/stores/catalog-store";
import { GridCell } from "./grid-cell";
import s from "./grid.module.css";

const TILE = 200; // target cell width
const GAP = 16; // matches --sp-5, and the virtualizer row gap

export function Grid() {
    const { query, arrangement } = useCatalogQuery();
    const { data, isPending, isError, refetch } = useQueryAssets(query, arrangement);

    // ponytail: loading/error/Retry copy is literal until the i18n key catalog
    // lands (widen); C14. The RAC Button port replaces the bare <button> then too.
    if (isPending) return <div className={s.state}>Loading…</div>;
    if (isError) {
        return (
            <div className={s.state}>
                <p>Couldn’t load the catalog.</p>
                <button type="button" onClick={() => void refetch()}>
                    Retry
                </button>
            </div>
        );
    }
    return <GridBody items={data.items} />;
}

function GridBody({ items }: { items: AssetRow[] }) {
    const dispatch = useCatalogDispatch();
    const cursorId = useCursorId();

    // Columns from the content width. A callback ref wires the observer exactly
    // when the scroll element mounts (after the isPending return); React 19 ref
    // cleanup disconnects it.
    const scrollRef = useRef<HTMLDivElement | null>(null);
    const [width, setWidth] = useState(0);
    const attachScroll = useCallback((el: HTMLDivElement | null) => {
        scrollRef.current = el;
        if (!el) return;
        setWidth(el.clientWidth - 2 * GAP);
        const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
        observer.observe(el);
        return () => observer.disconnect();
    }, []);

    const cols = Math.max(1, Math.floor((width + GAP) / (TILE + GAP)));
    const rowCount = Math.ceil(items.length / cols);

    const virtualizer = useVirtualizer({
        count: rowCount,
        getScrollElement: () => scrollRef.current,
        estimateSize: () => TILE + 72, // thumb + foot; measured live after mount
        overscan: 3,
        gap: GAP,
    });

    const indexOfCursor = useCallback(() => items.findIndex((row) => row.id === cursorId), [items, cursorId]);
    // ponytail: ranges materialize from the loaded `items` — correct while the whole
    // result is one page. TRIGGER: when windowed fetch lands, a range interior may be
    // unloaded — switch to the `assetIdSlice(query, arrangement, from, to)` seam call
    // (already built + tested in the mock) to materialize the id span (frontend/09).
    const rangeIds = useCallback(
        (from: number, to: number) => items.slice(Math.min(from, to), Math.max(from, to) + 1).map((row) => row.id),
        [items],
    );

    const handleClick = useCallback(
        (index: number, event: PointerEvent) => {
            const row = items[index];
            if (event.shiftKey) {
                const anchor = indexOfCursor();
                dispatch({ type: "range-committed", ids: rangeIds(anchor < 0 ? index : anchor, index) });
            } else {
                dispatch({ type: "asset-clicked", id: row.id, additive: event.metaKey || event.ctrlKey });
            }
        },
        [items, dispatch, indexOfCursor, rangeIds],
    );

    const handleKeyDown = useCallback(
        (event: KeyboardEvent) => {
            if (event.key === "Escape") return dispatch({ type: "selection-cleared" });
            if ((event.metaKey || event.ctrlKey) && event.key === "a") {
                event.preventDefault();
                return dispatch({ type: "select-all" });
            }
            const deltas: Record<string, number> = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -cols, ArrowDown: cols };
            const step = deltas[event.key];
            if (step === undefined) return;
            event.preventDefault();
            const current = indexOfCursor();
            const target = Math.max(0, Math.min(items.length - 1, (current < 0 ? 0 : current) + step));
            if (event.shiftKey) dispatch({ type: "range-committed", ids: rangeIds(current < 0 ? 0 : current, target) });
            else dispatch({ type: "cursor-set", id: items[target].id, select: true });
        },
        [cols, dispatch, indexOfCursor, items, rangeIds],
    );

    return (
        <div
            ref={attachScroll}
            className={s.scroll}
            role="grid"
            aria-multiselectable
            tabIndex={0}
            onKeyDown={handleKeyDown}
        >
            <div className={s.inner} style={{ height: virtualizer.getTotalSize() }}>
                {virtualizer.getVirtualItems().map((vRow) => (
                    <div
                        key={vRow.key}
                        role="row"
                        data-index={vRow.index}
                        ref={virtualizer.measureElement}
                        className={s.row}
                        style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`, transform: `translateY(${vRow.start}px)` }}
                    >
                        {items.slice(vRow.index * cols, vRow.index * cols + cols).map((row, k) => (
                            <GridCell key={row.id} asset={row} index={vRow.index * cols + k} onSelect={handleClick} />
                        ))}
                    </div>
                ))}
            </div>
        </div>
    );
}
