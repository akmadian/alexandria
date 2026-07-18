// Smoke: the library renders entirely from the compiler's reference output — if
// the emitted shape drifts (paths, sections, eligibility), this page is the first
// consumer to break, and this test is where it shows before the browser does.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test } from "vitest";
import { DesignLibrary } from "./design-library";

beforeEach(() => {
    delete document.documentElement.dataset.theme;
    localStorage.clear();
});

test("renders every section from the reference output", () => {
    render(<DesignLibrary />);
    expect(screen.getByRole("heading", { name: /Button — rungs × states/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /Type roles/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /Chrome roles/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /accent-eligible/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /Space · radius · shadows/ })).toBeInTheDocument();
    // One button per rung × 6 matrix columns + 4 theme chips + 2 size specimens.
    expect(screen.getAllByRole("button").length).toBeGreaterThan(30);
    // The generated theme vocabulary drives the switcher.
    for (const theme of ["paper", "linen", "graphite", "carbon"]) {
        expect(screen.getByRole("button", { name: theme })).toBeInTheDocument();
    }
});

test("theme buttons stamp the attribute and persist the preference", async () => {
    render(<DesignLibrary />);
    await userEvent.click(screen.getByRole("button", { name: "carbon" }));
    expect(document.documentElement.dataset.theme).toBe("carbon");
    expect(localStorage.getItem("alexandria.theme")).toBe("carbon");
});

test("the live matrix column presses for real", async () => {
    render(<DesignLibrary />);
    // Row order: 4 forced cells + disabled + LIVE — the live specimen is the
    // sixth Import of the first (ghost) row; forced/disabled cells carry no handler.
    const importButtons = screen.getAllByRole("button", { name: "Import" });
    await userEvent.click(importButtons[5]);
    expect(screen.getByText(/1 presses/)).toBeInTheDocument();
});
