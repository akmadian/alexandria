// StatusBar is a thin docked shell — the contract is: it renders its slot content in a labelled
// footer band. (The data-sm voice + 24px height are token-driven look, verified in Storybook;
// `composes`-from-global isn't inlined by the test CSS transform, so it isn't asserted here.)

import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { StatusBar } from "./status-bar";

test("renders its slot content in a labelled footer band", () => {
    render(
        <StatusBar aria-label="Status">
            <span>1,204 assets</span>
        </StatusBar>,
    );
    const bar = screen.getByRole("contentinfo", { name: "Status" });
    expect(bar.tagName).toBe("FOOTER");
    expect(screen.getByText("1,204 assets")).toBeInTheDocument();
});
