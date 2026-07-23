// Pane — the §12 docked side container: resizable by dragging its edge seam, collapsible to
// hidden, width + collapsed state persisted between sessions (FR:129/130/371). Domain-blind
// chrome; its contents (Tree, Inspector) are separate consumers. The container ONLY — the
// collapse *trigger* lives with the consumer (the workspace header sidebar toggle, per the inspo).
//
// RAC has no Splitter primitive, so the handle is built on `useMove` (react-aria): one delta
// stream across mouse + touch + arrow keys, text-selection suppressed mid-drag. We conform to the
// WAI-ARIA APG "Window Splitter" contract — the handle is a focusable role="separator" carrying
// aria-value{now,min,max}; Enter collapses/restores, Home/End jump to min/max, double-click resets.
// (Battle-tested reference: bvaughn/react-resizable-panels implements the same contract; we
// hand-roll here because its PanelGroup owns %-of-flex layout, which fights our CSS-Grid shell.)
//
// ponytail: collapse is instant — animating `width` is banned layout motion (§26 is transform/
// opacity only); a token-driven slide is a later motion round.

import { type ReactNode, useCallback, useRef, useState } from "react";
import { mergeProps, useMove } from "react-aria";
import { useTranslation } from "react-i18next";
import { PaneErrorBoundary } from "@/components/error-boundary/error-boundary";
import { cx } from "@/lib/cx";
import styles from "./pane.module.css";

export interface PaneProps {
    /** Which edge it docks to — drives the handle side, the seam, and the sign of the drag delta. */
    side: "left" | "right";
    /** Uncontrolled initial width (px). Rails pass the per-side token value; 280 is the fallback. */
    defaultWidth?: number;
    /** Clamp floor/ceiling for dragging (px). Explicit collapse goes below the floor, to hidden. */
    minWidth?: number;
    maxWidth?: number;
    /** Controlled collapse (the workspace header toggle drives it). Omit for uncontrolled. */
    isCollapsed?: boolean;
    defaultCollapsed?: boolean;
    onCollapsedChange?: (collapsed: boolean) => void;
    /** When set, width (always) + collapsed (uncontrolled only) persist to localStorage, read
     *  pre-paint so there is no first-frame flash. Plane 3 (chrome pref), never engine state. */
    storageKey?: string;
    /** Names the pane landmark AND the resize handle ("Resize {name}"). Required (C14/a11y). */
    "aria-label": string;
    children: ReactNode;
}

// ponytail: 280/200/480 are the primitive's neutral fallbacks (PIN — eye-gate on the real render);
// the actual rails pass token-derived widths (--alx-size-panel-left/right + -min) at assembly.
const DEFAULT_WIDTH = 280;
const DEFAULT_MIN = 200;
const DEFAULT_MAX = 480;
// Arrow-key resize step (px per press) — pointer deltas pass through 1:1; keyboard needs a coarser
// grain to be usable. Held arrow auto-repeats.
const KEYBOARD_STEP = 16;

interface StoredPaneState {
    width?: number;
    collapsed?: boolean;
}

function readStored(key: string | undefined): StoredPaneState | null {
    if (!key) return null;
    try {
        const raw = localStorage.getItem(key);
        return raw ? (JSON.parse(raw) as StoredPaneState) : null;
    } catch {
        return null; // private mode / corrupt value — fall back to props, never throw
    }
}

function writeStored(key: string | undefined, state: StoredPaneState): void {
    if (!key) return;
    try {
        localStorage.setItem(key, JSON.stringify(state));
    } catch {
        // quota / private mode — persistence is best-effort, the pane still works
    }
}

const clamp = (value: number, low: number, high: number): number => Math.min(Math.max(value, low), high);

export function Pane({
    side,
    defaultWidth = DEFAULT_WIDTH,
    minWidth = DEFAULT_MIN,
    maxWidth = DEFAULT_MAX,
    isCollapsed,
    defaultCollapsed = false,
    onCollapsedChange,
    storageKey,
    "aria-label": label,
    children,
}: PaneProps) {
    const { t } = useTranslation();

    // Pre-paint read: the useState initializer runs during the first render, so the stored width
    // is used before the browser paints — no flash. Read once (storageKey is not expected to change).
    const [stored] = useState(() => readStored(storageKey));
    const [width, setWidth] = useState(() => clamp(stored?.width ?? defaultWidth, minWidth, maxWidth));

    // Controlled/uncontrolled collapse (the RAC toggle idiom): the prop wins when supplied.
    const [collapsedState, setCollapsedState] = useState(() => stored?.collapsed ?? defaultCollapsed);
    const collapsed = isCollapsed ?? collapsedState;

    // Refs mirror the latest values so persistence (fired at gesture end) never reads a stale closure.
    const widthRef = useRef(width);
    widthRef.current = width;
    const collapsedRef = useRef(collapsed);
    collapsedRef.current = collapsed;

    // Width is always uncontrolled → always persisted. Collapsed is persisted only when we own it;
    // a controlled consumer records it in its own chrome pref.
    const persist = useCallback(
        (nextWidth: number, nextCollapsed: boolean) => {
            writeStored(storageKey, {
                width: nextWidth,
                collapsed: isCollapsed === undefined ? nextCollapsed : undefined,
            });
        },
        [storageKey, isCollapsed],
    );

    const applyWidth = useCallback(
        (next: number) => {
            const clamped = clamp(next, minWidth, maxWidth);
            widthRef.current = clamped;
            setWidth(clamped);
            persist(clamped, collapsedRef.current);
        },
        [minWidth, maxWidth, persist],
    );

    const setCollapsed = useCallback(
        (next: boolean) => {
            if (isCollapsed === undefined) setCollapsedState(next);
            persist(widthRef.current, next);
            onCollapsedChange?.(next);
        },
        [isCollapsed, persist, onCollapsedChange],
    );

    // useMove handles pointer drag AND arrow keys (as pointerType "keyboard"), suppressing text
    // selection for the duration. We persist once, at the end of the gesture, not per pixel.
    const { moveProps } = useMove({
        onMove(event) {
            const grain = event.pointerType === "keyboard" ? KEYBOARD_STEP : 1;
            const delta = (side === "left" ? event.deltaX : -event.deltaX) * grain;
            setWidth((previous) => {
                const next = clamp(previous + delta, minWidth, maxWidth);
                widthRef.current = next;
                return next;
            });
        },
        onMoveEnd() {
            persist(widthRef.current, collapsedRef.current);
        },
    });

    // APG Window Splitter keys beyond the arrows useMove already covers.
    const onHandleKeyDown = (event: React.KeyboardEvent) => {
        if (event.key === "Enter") {
            event.preventDefault();
            setCollapsed(!collapsedRef.current);
        } else if (event.key === "Home") {
            event.preventDefault();
            applyWidth(minWidth);
        } else if (event.key === "End") {
            event.preventDefault();
            applyWidth(maxWidth);
        }
    };

    return (
        <section
            aria-label={label}
            className={cx(styles.pane, styles[side])}
            data-collapsed={collapsed || undefined}
            style={{ "--pane-width": `${collapsed ? 0 : width}px` } as React.CSSProperties}
        >
            {/* `hidden` (not just CSS) truly removes the contents from the a11y tree + tab order
             *  when collapsed, and keeps the state observable without a layout engine. */}
            <div className={styles.body} hidden={collapsed}>
                <PaneErrorBoundary>{children}</PaneErrorBoundary>
            </div>
            {/* mergeProps chains useMove's onKeyDown (arrows) with ours (Enter/Home/End) — a raw
             *  spread + onKeyDown would clobber the arrow handler. */}
            <div
                {...mergeProps(moveProps, { onKeyDown: onHandleKeyDown })}
                role="separator"
                aria-orientation="vertical"
                aria-label={t("pane.resize", { name: label })}
                aria-valuenow={collapsed ? 0 : Math.round(width)}
                aria-valuemin={0}
                aria-valuemax={Math.round(maxWidth)}
                // APG Window Splitter: a focusable separator IS an interactive resize control (jsx-a11y#577).
                // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
                tabIndex={0}
                className={styles.handle}
                onDoubleClick={() => applyWidth(defaultWidth)}
            />
        </section>
    );
}
