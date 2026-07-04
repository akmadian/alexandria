// The app shell: one CSS Grid, three resizable regions, a boundary per pane.
// Also the single place the active ListQuery is derived and fetched — GridView
// gets rows, FilterBar/StatusBar get the total, Inspector reads selection.
// (When sparse windowing lands, page fetching moves INTO GridView; the derived
// query stays here.)

import { useEffect, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { installKeyboardDispatch, type KeyContext } from "@/lib/keys";
import { Button } from "@/components/button/button";
import { Toasts } from "@/components/toast/toast";
import { BrowserView } from "@/features/browser/browser-view";
import { FilterBar } from "@/features/filter-bar/filter-bar";
import { GridView } from "@/features/grid/grid-view";
import { InspectorView } from "@/features/inspector/inspector-view";
import { StatusBar } from "@/features/jobs/status-bar";
import { useAssets, useCatalogSync } from "@/api/queries";
import { ErrorBoundary } from "./error-boundary";
import { deriveListQuery, useLibraryState } from "./library-state";
import s from "./shell.module.css";

// --- pane resizing: a drag handle writes a CSS variable; widths persist ---

function usePaneWidth(name: string, fallback: number) {
    const ref = useRef<HTMLDivElement>(null);
    useEffect(() => {
        const stored = localStorage.getItem(`alexandria.pane.${name}`);
        ref.current?.style.setProperty(`--pane-${name}`, `${stored ?? fallback}px`);
    }, [name, fallback]);

    const onPointerDown = (e: React.PointerEvent) => {
        const shell = ref.current;
        if (!shell) return;
        const startX = e.clientX;
        const startW = parseFloat(getComputedStyle(shell).getPropertyValue(`--pane-${name}`)) || fallback;
        const dir = name === "inspector" ? -1 : 1;
        const move = (ev: PointerEvent) => {
            const w = Math.min(560, Math.max(160, startW + dir * (ev.clientX - startX)));
            shell.style.setProperty(`--pane-${name}`, `${w}px`);
        };
        const up = () => {
            localStorage.setItem(`alexandria.pane.${name}`, String(parseFloat(getComputedStyle(shell).getPropertyValue(`--pane-${name}`))));
            window.removeEventListener("pointermove", move);
            window.removeEventListener("pointerup", up);
        };
        window.addEventListener("pointermove", move);
        window.addEventListener("pointerup", up);
    };

    return { ref, onPointerDown };
}

const PaneFallback = ({ retry, message, label }: { retry: () => void; message: string; label: string }) => (
    <div className={s.paneFallback}>
        <p>{message}</p>
        <Button onPress={retry}>{label}</Button>
    </div>
);

export const Shell = () => {
    const { t } = useTranslation();
    const state = useLibraryState();
    useCatalogSync(); // backend events → cache invalidation, mounted once

    // Keyboard context derives from view state; a ref keeps the listener stable.
    const ctxRef = useRef<KeyContext>("grid");
    ctxRef.current = state.viewMode === "loupe" ? "detail" : "grid";
    useEffect(() => installKeyboardDispatch(() => ctxRef.current), []);

    const query = useMemo(() => deriveListQuery(state), [state]);
    const { data, isPending } = useAssets(query);
    const rows = data?.items ?? [];
    const total = data?.total ?? 0;

    const browser = usePaneWidth("browser", 240);
    const inspector = usePaneWidth("inspector", 300);

    const paneFallback = (_error: Error, retry: () => void) => <PaneFallback retry={retry} message={t("errors.panelCrashed")} label={t("errors.reloadPanel")} />;

    return (
        <div className={s.shell} ref={browser.ref}>
            <div style={{ display: "contents" }} ref={inspector.ref}>
                <header className={s.filterbar}>
                    <ErrorBoundary fallback={paneFallback}>
                        <FilterBar total={total} />
                    </ErrorBoundary>
                </header>

                <aside className={s.browser}>
                    <ErrorBoundary fallback={paneFallback}>
                        <BrowserView />
                    </ErrorBoundary>
                </aside>
                <div
                    className={s.handle}
                    role="separator"
                    aria-orientation="vertical"
                    aria-label="Resize browser"
                    onPointerDown={browser.onPointerDown}
                />

                <main className={s.main}>
                    <ErrorBoundary fallback={paneFallback}>
                        <GridView rows={rows} isPending={isPending} />
                    </ErrorBoundary>
                </main>

                <div
                    className={s.handle}
                    role="separator"
                    aria-orientation="vertical"
                    aria-label="Resize inspector"
                    onPointerDown={inspector.onPointerDown}
                />
                <aside className={s.inspector}>
                    <ErrorBoundary fallback={paneFallback}>
                        <InspectorView />
                    </ErrorBoundary>
                </aside>

                <footer className={s.status}>
                    <ErrorBoundary fallback={paneFallback}>
                        <StatusBar total={total} />
                    </ErrorBoundary>
                </footer>

                <Toasts />
            </div>
        </div>
    );
};
