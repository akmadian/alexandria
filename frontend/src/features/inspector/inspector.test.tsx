// The inspector over the real mock seam: the cursor is the subject (§15), rows
// render only for present values, and the states (empty / error) are explicit.
// PanelSection collapse is content-visibility based — assert VISIBILITY, never
// DOM presence (frontend/CLAUDE.md §8).

import { act, render, renderHook, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test } from "vitest";
import { mockApi } from "@/api/mock";
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
    // The Judgment section is interactive now (task 34): the seeded flag pick and
    // red label read as pressed buttons; the note is an editable field.
    expect(screen.getByRole("button", { name: "Flag as pick" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Red" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByDisplayValue("Check focus on the eyes before export.")).toBeVisible();
    expect(screen.getByText(/47\.6/)).toBeVisible();
    // The rating readout is ALWAYS present — the interactive group still labels its state.
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

// LAST in the file: this test WRITES through the real lane into the module-global
// mock catalog (mutating mock-0000/0001's labels), so it must not precede the
// read-only assertions above.
test("panel editors write the C5 target — the whole selection, not just the subject", async () => {
    // Plain click selects mock-0000; additive click grows the selection and moves
    // the cursor (the panel's subject) to mock-0001. C5: the write targets BOTH.
    act(() => dispatch({ type: "asset-clicked", id: "mock-0000", additive: false }));
    act(() => dispatch({ type: "asset-clicked", id: "mock-0001", additive: true }));
    await screen.findByTitle("DSC_04821.arw");

    // Purple: a label NEITHER seed carries (mock-0000 red, mock-0001 yellow), so
    // the toggle can only SET — a clear would mean the toggle misread its state.
    await userEvent.click(screen.getByRole("button", { name: "Purple" }));

    await waitFor(async () => {
        const [first, second] = await Promise.all([mockApi.getAsset("mock-0000"), mockApi.getAsset("mock-0001")]);
        expect(first.colorLabel).toBe("purple");
        expect(second.colorLabel).toBe("purple");
    });
});

test("MiddleTruncate leaves short names whole and splits long ones", () => {
    const { container } = render(<MiddleTruncate text="short.jpg" />);
    expect(container.textContent).toBe("short.jpg");
    const { container: long } = render(<MiddleTruncate text="Boulder River Wilderness 1-.jpg" />);
    expect(long.textContent).toBe("Boulder River Wilderness 1-.jpg");
    expect(long.querySelector("[title='Boulder River Wilderness 1-.jpg']")).not.toBeNull();
});
