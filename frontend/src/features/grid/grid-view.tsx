// The grid: virtualized rows of AssetCards over the shell-fetched result set,
// selection interactions (click / mod / shift-range), and the keyboard triage
// handlers — mouse and keys dispatch the same LibraryProvider actions.
//
// ponytail: rows arrive whole from the shell (mock catalog is small). The
// sparse-window upgrade (seam doc §5 — pages on demand keyed by visible range)
// replaces `rows` with a windowed fetch INSIDE this component and touches
// nothing outside it. That's the designed next step, not a redesign.

import { useVirtualizer } from "@tanstack/react-virtual";
import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { registerHandlers } from "@/lib/keys";
import { useLibraryDispatch, useLibraryState } from "@/app/library-state";
import { usePatchAssets } from "@/api/queries";
import type { AssetPatch, AssetRow } from "@/api/contract";
import { AssetCard } from "./asset-card";
import s from "./grid-view.module.css";

const TILE = { comfortable: 176, compact: 128 } as const;
const GAP = 8;

export const GridView = ({ rows, isPending }: { rows: AssetRow[]; isPending: boolean }) => {
    const { t } = useTranslation();
    const state = useLibraryState();
    const dispatch = useLibraryDispatch();
    const patchAssets = usePatchAssets();

    const tile = TILE[state.filters.density];

    // Columns from container width. A callback ref (not an effect) sets up the
    // observer exactly when the scroll element attaches — the element only
    // mounts once data arrives, after the isPending early-return, so an
    // empty-dep effect would run too early and never see it. React 19 ref
    // cleanup disconnects the observer on unmount.
    const scrollRef = useRef<HTMLDivElement | null>(null);
    const [width, setWidth] = useState(0);
    const attachScroll = useCallback((el: HTMLDivElement | null) => {
        scrollRef.current = el;
        if (!el) return;
        setWidth(el.clientWidth);
        const ro = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
        ro.observe(el);
        return () => ro.disconnect();
    }, []);
    const cols = Math.max(1, Math.floor((width - GAP) / (tile + GAP)));
    const rowCount = Math.ceil(rows.length / cols);

    const virtualizer = useVirtualizer({
        count: rowCount,
        getScrollElement: () => scrollRef.current,
        estimateSize: () => tile + 28 + GAP, // tile + meta strip + gap
        overscan: 4,
    });

    // --- selection: mouse path -------------------------------------------------
    const indexOf = (id: string) => rows.findIndex((r) => r.id === id);
    const onCardPointerDown = (row: AssetRow, e: React.PointerEvent) => {
        if (e.shiftKey && state.lastSelectedId) {
            const a = indexOf(state.lastSelectedId);
            const b = indexOf(row.id);
            if (a !== -1 && b !== -1) {
                const rangeIds = rows.slice(Math.min(a, b), Math.max(a, b) + 1).map((r) => r.id);
                dispatch({ type: "select", id: row.id, rangeIds });
                return;
            }
        }
        dispatch({ type: "select", id: row.id, additive: e.metaKey || e.ctrlKey });
    };

    // --- keyboard path: same actions, via the dispatcher. Refs keep the handler
    // registration stable (registered once) while handlers always read current
    // state. Refs are written in an effect, not during render. --------------------
    const latest = useRef({ rows, selection: state.selection, mutate: patchAssets.mutate, dispatch });
    useEffect(() => {
        latest.current = { rows, selection: state.selection, mutate: patchAssets.mutate, dispatch };
    });

    useEffect(() => {
        const patchSelected = (patch: AssetPatch) => {
            const ids = [...latest.current.selection];
            if (ids.length > 0) latest.current.mutate({ target: { ids }, patch });
        };
        return registerHandlers({
            rate_0: () => patchSelected({ rating: null }),
            rate_1: () => patchSelected({ rating: 1 }),
            rate_2: () => patchSelected({ rating: 2 }),
            rate_3: () => patchSelected({ rating: 3 }),
            rate_4: () => patchSelected({ rating: 4 }),
            rate_5: () => patchSelected({ rating: 5 }),
            flag_pick: () => patchSelected({ flag: "pick" }),
            flag_reject: () => patchSelected({ flag: "reject" }),
            clear_flag: () => patchSelected({ flag: null }),
            select_all: () => latest.current.dispatch({ type: "selectMany", ids: latest.current.rows.map((r) => r.id) }),
            clear_selection: () => latest.current.dispatch({ type: "clearSelection" }),
        });
    }, []);

    if (isPending) return <div className={s.state}>{t("grid.loading")}</div>;
    if (rows.length === 0) return <div className={s.state}>{t("grid.empty")}</div>;

    return (
        <div ref={attachScroll} className={s.scroll} role="grid" aria-multiselectable style={{ "--tile-size": `${tile}px` } as React.CSSProperties}>
            <div className={s.inner} style={{ height: virtualizer.getTotalSize() }}>
                {virtualizer.getVirtualItems().map((vRow) => (
                    <div key={vRow.key} role="row" className={s.row} style={{ transform: `translateY(${vRow.start}px)` }}>
                        {rows.slice(vRow.index * cols, vRow.index * cols + cols).map((row) => (
                            <AssetCard key={row.id} row={row} selected={state.selection.has(row.id)} onPointerDown={onCardPointerDown} />
                        ))}
                    </div>
                ))}
            </div>
        </div>
    );
};
