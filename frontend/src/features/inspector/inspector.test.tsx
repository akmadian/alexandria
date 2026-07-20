// The inspector over the real mock seam: the cursor is the subject (§15), rows
// render only for present values, and the states (empty / error) are explicit.
// PanelSection collapse is content-visibility based — assert VISIBILITY, never
// DOM presence (frontend/CLAUDE.md §8).

import { act, render, renderHook, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, test } from "vitest";
import { Providers } from "@/app/providers";
import { type CatalogAction, useCatalogDispatch } from "@/stores/catalog-store";
import { Inspector, MiddleTruncate } from "./inspector";

let dispatch: (action: CatalogAction) => void;

beforeEach(() => {
    // The store is module-global; filter-replaced clears selection AND cursor,
    // so each test starts subject-less.
    const { result } = renderHook(() => useCatalogDispatch());
    dispatch = result.current;
    act(() => dispatch({ type: "filter-replaced", filter: null }));
    render(
        <Providers>
            <Inspector />
        </Providers>,
    );
});

// Seed facts for mock-0000 (i = 0): jpg, Sony A7 IV, unrated, flag pick, red
// label, exposure 1/1000 · ƒ/1.8 · ISO 100 · 24 mm, GPS, sRGB, a note, and the
// extended EXIF blob. See mock.ts seededAssets.
const SUBJECT = "mock-0000";

function setCursor(id: string): void {
    act(() => dispatch({ type: "cursor-set", id, select: false }));
}

test("no cursor renders the empty invitation, not a fetch", () => {
    expect(screen.getByText("Select an asset to inspect it")).toBeInTheDocument();
});

test("the cursor asset's metadata renders in sections", async () => {
    setCursor(SUBJECT);
    // Filename middle-truncates: the full string survives on the title reveal.
    await waitFor(() => expect(screen.getByTitle("DSC_04820.jpg")).toBeInTheDocument());
    expect(screen.getByText("2026")).toBeVisible(); // the Folder row — dirname of the relative path
    expect(screen.getByText("1/1000 at ƒ/1.8")).toBeVisible();
    expect(screen.getByText("ISO 100")).toBeVisible();
    expect(screen.getByText("Sony A7 IV")).toBeVisible();
    expect(screen.getByText("Pick")).toBeVisible();
    expect(screen.getByText("Red")).toBeVisible();
    expect(screen.getByText("Check focus on the eyes before export.")).toBeVisible();
    expect(screen.getByText(/47\.6/)).toBeVisible();
    // The rating readout is ALWAYS present — unrated renders the hollow five.
    expect(screen.getByLabelText("Unrated")).toBeInTheDocument();
    // Online is silent (§10): no status row for a healthy file.
    expect(screen.queryByText("Status")).not.toBeInTheDocument();
});

test("the all-metadata section ships collapsed but present", async () => {
    setCursor(SUBJECT);
    await screen.findByText("All metadata");
    const flashRow = screen.getByText("EXIF:Flash");
    expect(flashRow).not.toBeVisible();
    // Structured values render as compact JSON, never "[object Object]"
    // (mock-0000 seeds the importer's extension_mismatch map).
    expect(screen.getByText('{"declared":"jpg","detected":"png"}')).toBeInTheDocument();
    expect(screen.queryByText(/object Object/)).not.toBeInTheDocument();
});

test("the subject follows the cursor", async () => {
    setCursor(SUBJECT);
    await screen.findByTitle("DSC_04820.jpg");
    setCursor("mock-0001");
    await waitFor(() => expect(screen.getByTitle("DSC_04821.arw")).toBeInTheDocument());
});

test("an unknown id renders the explicit error state with retry", async () => {
    setCursor("mock-nope");
    expect(await screen.findByText("Couldn't load this asset.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
});

test("MiddleTruncate leaves short names whole and splits long ones", () => {
    const { container } = render(<MiddleTruncate text="short.jpg" />);
    expect(container.textContent).toBe("short.jpg");
    const { container: long } = render(<MiddleTruncate text="Boulder River Wilderness 1-.jpg" />);
    expect(long.textContent).toBe("Boulder River Wilderness 1-.jpg");
    expect(long.querySelector("[title='Boulder River Wilderness 1-.jpg']")).not.toBeNull();
});
