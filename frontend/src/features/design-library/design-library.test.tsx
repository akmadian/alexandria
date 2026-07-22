// Smoke: the library renders entirely from the compiler's reference output — if
// the emitted shape drifts (paths, sections, eligibility), this page is the first
// consumer to break, and this test is where it shows before the browser does.

import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test } from "vitest";
import { DesignLibrary } from "./design-library";

beforeEach(() => {
    delete document.documentElement.dataset.theme;
    localStorage.clear();
});

test("renders every section from the reference output", () => {
    render(<DesignLibrary />);
    expect(screen.getByRole("heading", { name: /Sizing system/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /Button — rungs × states/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /Type roles/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /Chrome roles/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /accent-eligible/ })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /Space · radius · shadows/ })).toBeInTheDocument();
    // The matrices alone put well over thirty press-family controls on the page.
    expect(screen.getAllByRole("button").length).toBeGreaterThan(30);
    // The generated theme vocabulary drives the switcher — now a SegmentedControl,
    // so each theme is a radio, not a button.
    for (const theme of ["paper", "linen", "graphite", "carbon"]) {
        expect(screen.getByRole("radio", { name: theme })).toBeInTheDocument();
    }
});

test("the theme segment stamps the attribute and persists the preference", async () => {
    render(<DesignLibrary />);
    await userEvent.click(screen.getByRole("radio", { name: "carbon" }));
    expect(document.documentElement.dataset.theme).toBe("carbon");
    expect(localStorage.getItem("alexandria.theme")).toBe("carbon");
});

test("the live matrix column presses for real", async () => {
    render(<DesignLibrary />);
    // Scope to the Button section so the sizing showcase's Import buttons don't shift
    // the index. In the rung×state matrix the ghost row is first: 4 forced cells +
    // disabled + LIVE — the live specimen is the sixth Import; only it carries a handler.
    const buttonSection = screen
        .getByRole("heading", { name: /Button — rungs × states/ })
        .closest("section") as HTMLElement;
    const importButtons = within(buttonSection).getAllByRole("button", { name: "Import" });
    await userEvent.click(importButtons[5]);
    expect(screen.getByText(/1 presses/)).toBeInTheDocument();
});
