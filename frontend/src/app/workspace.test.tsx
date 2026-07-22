// The workspace tab strip: the registry stays complete, selecting a tab swaps the
// body, and — the load-bearing one — C3 holds because the Catalog panel is
// force-mounted (kept in the DOM, inert + hidden) across a switch away and back,
// while a task view (Import) mounts on entry and unmounts on leave.
//
// happy-dom caveat (frontend/CLAUDE.md §8): RAC keyboard roving isn't portaled, so
// arrow-key nav is testable here; the actual scroll-pixel restore rides on real
// layout, which happy-dom lacks — that half needs in-browser eyeballing. What this
// file proves is the mechanism underneath it: the panel's DOM node is never
// recreated, so its virtualizer (and thus scroll offset) is never reset.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test } from "vitest";
import { Providers } from "./providers";
import { TAB_ORDER, WORKSPACE_TABS, Workspace } from "./workspace";

function renderWorkspace() {
    return render(
        <Providers>
            <Workspace />
        </Providers>,
    );
}

test("the registry and the render order name exactly the same tabs, each entry complete", () => {
    // The `satisfies Record<WorkspaceTabKey, WorkspaceTab>` gate is compile-time;
    // this pins the runtime shape the render loop depends on and keeps TAB_ORDER
    // in lockstep with the registry keys.
    expect([...TAB_ORDER].sort()).toEqual(Object.keys(WORKSPACE_TABS).sort());
    for (const key of TAB_ORDER) {
        const entry = WORKSPACE_TABS[key];
        expect(typeof entry.labelKey).toBe("string");
        expect(typeof entry.Panel).toBe("function");
        expect(typeof entry.keepMounted).toBe("boolean");
    }
});

test("Catalog is the default panel; the strip renders every tab and a settings gear", () => {
    renderWorkspace();
    expect(screen.getByRole("tab", { name: "Catalog" })).toHaveAttribute("data-selected");
    expect(screen.getByRole("tab", { name: "Import" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Settings" })).toBeInTheDocument();
    // Only the Catalog panel exists at rest — Import is a task view, mounted on entry.
    expect(screen.getByTestId("catalog-panel")).toBeInTheDocument();
    expect(screen.queryByTestId("import-panel")).toBeNull();
});

test("selecting Import swaps the body; the Catalog panel stays mounted but inert", async () => {
    const user = userEvent.setup();
    renderWorkspace();
    const catalogPanel = screen.getByTestId("catalog-panel");

    await user.click(screen.getByRole("tab", { name: "Import" }));

    expect(screen.getByRole("tab", { name: "Import" })).toHaveAttribute("data-selected");
    expect(screen.getByTestId("import-panel")).toBeInTheDocument();
    // C3: the SAME Catalog node persists — force-mounted, not recreated — and its
    // wrapping tab panel is now inert (RAC's data-inert), which the CSS hides.
    expect(screen.getByTestId("catalog-panel")).toBe(catalogPanel);
    expect(catalogPanel.parentElement).toHaveAttribute("data-inert");
});

test("C3: returning to Catalog restores the very same panel node; Import unmounts", async () => {
    const user = userEvent.setup();
    renderWorkspace();
    const catalogPanel = screen.getByTestId("catalog-panel");

    await user.click(screen.getByRole("tab", { name: "Import" }));
    await user.click(screen.getByRole("tab", { name: "Catalog" }));

    // Identity preserved across the whole round trip → React never unmounted it,
    // so the virtualizer's scroll offset (component-local state) survives.
    expect(screen.getByTestId("catalog-panel")).toBe(catalogPanel);
    expect(catalogPanel.parentElement).not.toHaveAttribute("data-inert");
    // The task view, not force-mounted, is gone once left.
    expect(screen.queryByTestId("import-panel")).toBeNull();
});

test("arrow keys move the selection along the strip (RAC roving, automatic activation)", async () => {
    const user = userEvent.setup();
    renderWorkspace();
    const catalogTab = screen.getByRole("tab", { name: "Catalog" });
    catalogTab.focus();

    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "Import" })).toHaveAttribute("data-selected");

    await user.keyboard("{ArrowLeft}");
    expect(screen.getByRole("tab", { name: "Catalog" })).toHaveAttribute("data-selected");
});
