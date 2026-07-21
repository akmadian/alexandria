// The grid over the real mock seam: cells render from the fetched page, the
// click grammar lands in the store (asserted through the cells' own rendered
// data-attributes — the C2 loop closed end-to-end), and the echo seeds the
// cursor to the first row. fireEvent (not userEvent) carries the modifier
// flags — these are bespoke divs, no RAC press routing involved.

import { act, cleanup, fireEvent, render, renderHook, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { api } from "@/api/client";
import { Providers } from "@/app/providers";
import { log } from "@/lib/logger";
import { DEFAULT_ARRANGEMENT, type Query } from "@/query-model/ast";
import { leaf } from "@/query-model/registry";
import { type CatalogAction, useCatalogDispatch } from "@/stores/catalog-store";
import { columnsForWidth, Grid } from "./grid";
import { GridCell } from "./grid-cell";

let dispatch: (action: CatalogAction) => void;

beforeEach(() => {
    // The store is module-global; filter-replaced clears selection AND cursor,
    // so each test starts from the blank grammar state.
    const { result } = renderHook(() => useCatalogDispatch());
    dispatch = result.current;
    act(() => dispatch({ type: "filter-replaced", filter: null }));
    render(
        <Providers>
            <Grid />
        </Providers>,
    );
});

afterEach(() => {
    vi.restoreAllMocks();
});

async function loadedCells(): Promise<HTMLElement[]> {
    // gridcells carry the selection/cursor state attributes; they only exist once
    // the page loads (the pending/error/empty states render no cells).
    return screen.findAllByRole("gridcell");
}

test("cells render from the mock page and the echo seeds the cursor to row one", async () => {
    const cells = await loadedCells();
    // EXACTLY the measured column count — this test runs against a cold cache,
    // so it catches the pending-branch measurement bug (width stuck at 0 → a
    // one-column grid) that warm-cache renders mask.
    const firstRow = screen.getAllByRole("row")[0] as HTMLElement;
    expect(within(firstRow).getAllByRole("gridcell")).toHaveLength(columnsForWidth(800));
    await waitFor(() => expect(cells[0]?.hasAttribute("data-cursor")).toBe(true));
    expect(cells[0]?.hasAttribute("data-selected")).toBe(false);
});

test("plain click selects one; cmd-click adds; a second cmd-click removes", async () => {
    const cells = await loadedCells();
    fireEvent.click(cells[0] as HTMLElement);
    expect(cells[0]?.hasAttribute("data-selected")).toBe(true);
    expect(cells[0]?.hasAttribute("data-cursor")).toBe(true);
    fireEvent.click(cells[2] as HTMLElement, { metaKey: true });
    expect(cells[0]?.hasAttribute("data-selected")).toBe(true);
    expect(cells[2]?.hasAttribute("data-selected")).toBe(true);
    expect(cells[2]?.hasAttribute("data-cursor")).toBe(true);
    fireEvent.click(cells[2] as HTMLElement, { metaKey: true });
    expect(cells[2]?.hasAttribute("data-selected")).toBe(false);
});

test("shift-click materializes the range through the seam and lands the cursor on the clicked end", async () => {
    const cells = await loadedCells();
    fireEvent.click(cells[1] as HTMLElement);
    fireEvent.click(cells[4] as HTMLElement, { shiftKey: true });
    await waitFor(() => {
        for (const index of [1, 2, 3, 4]) {
            expect(cells[index]?.hasAttribute("data-selected")).toBe(true);
        }
    });
    expect(cells[4]?.hasAttribute("data-cursor")).toBe(true);
    expect(cells[0]?.hasAttribute("data-selected")).toBe(false);
});

test("a shift-click anchor outside the resident blocks resolves through the seam", async () => {
    // Re-render with dev-sized blocks (the Grid's test seam) so the 64-row mock
    // spans eight blocks and an asset can genuinely live OUTSIDE residency.
    cleanup();
    render(
        <Providers>
            <Grid blockModelOptions={{ blockSize: 8, bufferBlocks: 0, residentCap: 10, debounceMs: 0 }} />
        </Providers>,
    );
    // Wait past block 0 alone: an index-8+ cell rendering proves block 1 landed.
    await waitFor(() => expect(screen.getAllByRole("gridcell").length).toBeGreaterThanOrEqual(12));
    const cells = screen.getAllByRole("gridcell");
    const library: Query = { version: 1, scope: { kind: "library" }, where: null };
    const { items } = await api.queryAssets(library, DEFAULT_ARRANGEMENT, { offset: 0, limit: 64 });
    const anchorId = items[60]?.id;
    if (anchorId === undefined) throw new Error("mock catalog shrank below 61 rows");
    // The anchor sits at index 60 — block 7, resident nowhere near the viewport's
    // blocks — so localIndexOf misses and indexOfAsset places it over the seam.
    act(() => dispatch({ type: "cursor-set", id: anchorId, select: false }));
    fireEvent.click(cells[2] as HTMLElement, { shiftKey: true });
    await waitFor(() => expect(cells[2]?.hasAttribute("data-selected")).toBe(true));
    // The full 2..60 range landed: every rendered cell from the clicked end on is
    // selected, the rows before it are not, and the clicked end carries the cursor.
    expect(cells[2]?.hasAttribute("data-cursor")).toBe(true);
    expect(cells[10]?.hasAttribute("data-selected")).toBe(true);
    expect(cells[0]?.hasAttribute("data-selected")).toBe(false);
    expect(cells[1]?.hasAttribute("data-selected")).toBe(false);
});

test("a failed anchor lookup degrades the shift-click to a plain single select", async () => {
    const cells = await loadedCells();
    const errorSpy = vi.spyOn(log, "error");
    // The cursor's id is in no resident block, and the seam lookup REJECTS —
    // the gesture must not die as an unhandled rejection.
    vi.spyOn(api, "indexOfAsset").mockRejectedValueOnce(new Error("catalog busy"));
    act(() => dispatch({ type: "cursor-set", id: "not-a-loaded-asset", select: false }));
    fireEvent.click(cells[2] as HTMLElement, { shiftKey: true });
    await waitFor(() => expect(cells[2]?.hasAttribute("data-selected")).toBe(true));
    expect(cells[2]?.hasAttribute("data-cursor")).toBe(true);
    expect(cells.filter((cell) => cell.hasAttribute("data-selected"))).toHaveLength(1);
    expect(errorSpy).toHaveBeenCalledWith("grid: anchor index lookup failed — degrading to single select", {
        error: "Error: catalog busy",
    });
});

test("shift-click with the anchor absent falls back to a plain single select", async () => {
    const cells = await loadedCells();
    // A cursor id no resident block carries AND the seam can't place — the block
    // model resolves the anchor via indexOfAsset (mock returns null here), so the
    // gesture has no other end and degrades to a plain single select.
    act(() => dispatch({ type: "cursor-set", id: "not-a-loaded-asset", select: false }));
    fireEvent.click(cells[2] as HTMLElement, { shiftKey: true });
    await waitFor(() => expect(cells[2]?.hasAttribute("data-selected")).toBe(true));
    expect(cells[2]?.hasAttribute("data-cursor")).toBe(true);
    const selected = cells.filter((cell) => cell.hasAttribute("data-selected"));
    expect(selected).toHaveLength(1);
});

test("a failed fetch renders the error state; Retry recovers into the empty state", async () => {
    await loadedCells();
    // A fresh filter = a fresh query key = a real fetch — which fails once.
    vi.spyOn(api, "queryAssets").mockRejectedValueOnce(new Error("catalog busy"));
    act(() => dispatch({ type: "filter-replaced", filter: leaf("filename", "contains", "zzz-no-match") }));
    expect(await screen.findByText("The catalog didn’t answer.")).toBeVisible();
    // Retry refetches through the restored mock; the filter matches nothing,
    // so recovery lands in the explicit EMPTY state — three branches, one flow.
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("No assets match.")).toBeVisible();
});

test("an unloaded row renders the placeholder mat — no image, no interactivity", () => {
    const { container } = render(<GridCell row={undefined} index={0} onCellClick={() => {}} />);
    expect(container.querySelector("img")).toBeNull();
    expect(container.firstElementChild?.className).toContain("cell");
});
