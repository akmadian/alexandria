// The grid over the real mock seam: cells render from the fetched page, the
// click grammar lands in the store (asserted through the cells' own rendered
// data-attributes — the C2 loop closed end-to-end), and the echo seeds the
// cursor to the first row. fireEvent (not userEvent) carries the modifier
// flags — these are bespoke divs, no RAC press routing involved.

import { act, fireEvent, render, renderHook, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { api } from "@/api/client";
import type { AssetRow } from "@/api/contract";
import { Providers } from "@/app/providers";
import { log } from "@/lib/logger";
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

test("shift-click with the anchor gone falls back to a plain single select", async () => {
    const cells = await loadedCells();
    // A cursor id no loaded row carries — the block-model scenario the
    // ponytail'd findIndex lookup cannot resolve.
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

test("the page-cap truncation logs loudly — the UI never pretends", async () => {
    await loadedCells();
    const truncatedRow: AssetRow = {
        kind: "asset",
        id: "cap-1",
        sourceId: "cap",
        filename: "cap.jpg",
        fileType: "image",
        fileStatus: "online",
        rating: null,
        colorLabel: null,
        flag: null,
        width: 100,
        height: 100,
        durationSecs: null,
        cameraModel: null,
        capturedAt: null,
        ingestedAt: "2026-07-01T00:00:00Z",
        thumbnailAt: null,
        relativePath: "cap.jpg",
        sizeBytes: 1,
        thumbURL: "data:image/svg+xml,cap",
    };
    const warn = vi.spyOn(log, "warn");
    vi.spyOn(api, "queryAssets").mockResolvedValueOnce({ items: [truncatedRow], total: 2 });
    act(() => dispatch({ type: "filter-replaced", filter: leaf("filename", "contains", "cap-probe") }));
    await waitFor(() =>
        expect(warn).toHaveBeenCalledWith("api: page cap truncates the working set", { total: 2, loaded: 1 }),
    );
});

test("an unloaded row renders the placeholder mat — no image, no interactivity", () => {
    const { container } = render(<GridCell row={undefined} index={0} onCellClick={() => {}} />);
    expect(container.querySelector("img")).toBeNull();
    expect(container.firstElementChild?.className).toContain("cell");
});
