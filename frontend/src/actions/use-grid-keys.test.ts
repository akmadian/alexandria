// The grid-key dispatcher end to end: a mounted hook + REAL window keydown
// events, driving the actual catalog store (C5 targeting) with the write hook
// mocked at its module boundary — so every guard branch executes for real:
// modifier bail, text-entry bail, all-shaped gate (+ debug log), empty target,
// and the happy path's exact target/patch + preventDefault.

import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { log } from "@/lib/logger";
import { useCatalogDispatch } from "@/stores/catalog-store";
import { useGridKeys } from "./use-grid-keys";

const writeTriage = vi.hoisted(() => vi.fn());
vi.mock("@/api/mutations", () => ({ useUpdateAssets: () => ({ writeTriage }) }));

let dispatch: ReturnType<typeof useCatalogDispatch>;

function press(key: string, init: KeyboardEventInit = {}, target: EventTarget = window): KeyboardEvent {
    const event = new KeyboardEvent("keydown", { key, cancelable: true, bubbles: true, ...init });
    act(() => {
        target.dispatchEvent(event);
    });
    return event;
}

beforeEach(() => {
    writeTriage.mockClear();
    // The store is module-global: filter-replaced clears selection AND cursor,
    // so each case starts from a known-empty state.
    const { result } = renderHook(() => useCatalogDispatch());
    dispatch = result.current;
    act(() => dispatch({ type: "filter-replaced", filter: null }));
});
afterEach(() => {
    vi.restoreAllMocks();
});

describe("useGridKeys", () => {
    it("happy path: a triage key writes the C5 target with the absolute patch and consumes the event", () => {
        renderHook(() => useGridKeys());
        act(() => dispatch({ type: "cursor-set", id: "asset-a", select: false }));

        const event = press("3");
        expect(writeTriage).toHaveBeenCalledExactlyOnceWith({ ids: ["asset-a"] }, { rating: 3 });
        expect(event.defaultPrevented).toBe(true);
    });

    it("targets the selection over the cursor when one exists", () => {
        renderHook(() => useGridKeys());
        act(() => dispatch({ type: "range-committed", ids: ["a", "b"] }));

        press("p");
        expect(writeTriage).toHaveBeenCalledExactlyOnceWith({ ids: ["a", "b"] }, { flag: "pick" });
    });

    it("bails on modified keys (Cmd/Ctrl/Alt belong to other shortcuts)", () => {
        renderHook(() => useGridKeys());
        act(() => dispatch({ type: "cursor-set", id: "asset-a", select: false }));

        press("3", { metaKey: true });
        press("3", { ctrlKey: true });
        press("3", { altKey: true });
        expect(writeTriage).not.toHaveBeenCalled();
    });

    it("bails when the keystroke targets text entry (the note field owns its digits)", () => {
        renderHook(() => useGridKeys());
        act(() => dispatch({ type: "cursor-set", id: "asset-a", select: false }));

        for (const tag of ["input", "textarea", "select"] as const) {
            const element = document.createElement(tag);
            document.body.appendChild(element);
            try {
                press("3", {}, element);
            } finally {
                element.remove();
            }
        }
        expect(writeTriage).not.toHaveBeenCalled();
    });

    it("gates an all-shaped selection (no mass write until the undo round) and says so at debug", () => {
        const debugSpy = vi.spyOn(log, "debug");
        renderHook(() => useGridKeys());
        act(() => dispatch({ type: "select-all" }));

        const event = press("3");
        expect(writeTriage).not.toHaveBeenCalled();
        expect(event.defaultPrevented).toBe(false);
        expect(debugSpy).toHaveBeenCalledWith(
            "triage key ignored: all-shaped selection gated until the undo round",
            { action: "rate-3" },
        );
    });

    it("writes nothing when there is no selection and no cursor", () => {
        renderHook(() => useGridKeys());
        const event = press("3");
        expect(writeTriage).not.toHaveBeenCalled();
        expect(event.defaultPrevented).toBe(false);
    });

    it("lets non-triage keys fall through untouched", () => {
        renderHook(() => useGridKeys());
        act(() => dispatch({ type: "cursor-set", id: "asset-a", select: false }));

        const event = press("j");
        expect(writeTriage).not.toHaveBeenCalled();
        expect(event.defaultPrevented).toBe(false);
    });

    it("removes the listener on unmount", () => {
        const { unmount } = renderHook(() => useGridKeys());
        act(() => dispatch({ type: "cursor-set", id: "asset-a", select: false }));
        unmount();

        press("3");
        expect(writeTriage).not.toHaveBeenCalled();
    });
});
