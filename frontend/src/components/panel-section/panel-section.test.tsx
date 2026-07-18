// PanelSection drives RAC Disclosure: the head toggles the panel, the chevron
// concept renders, and rows inside inherit the section's intent (§8).

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test } from "vitest";
import { Row } from "@/components/row/row";
import { PanelSection } from "./panel-section";

test("renders the head, the disclose concept, and intent-inheriting rows", () => {
    const { container } = render(
        <PanelSection head="Metadata" intent="text">
            <Row label="ISO" value="400" />
        </PanelSection>,
    );
    expect(screen.getByRole("button", { name: /Metadata/ })).toBeInTheDocument();
    expect(container.querySelector("svg")).not.toBeNull();
    expect(screen.getByText("ISO").parentElement?.className).toContain("text");
    // The omitted-prop default is EXPANDED — visibility, not DOM presence, is
    // the assertion (RAC keeps collapsed content under hidden="until-found").
    expect(screen.getByText("ISO")).toBeVisible();
});

test("the head toggles the panel and the expanded state", async () => {
    render(
        <PanelSection head="Judgment" defaultExpanded={false}>
            <Row intent="control" label="Rating" value="unrated" />
        </PanelSection>,
    );
    const head = screen.getByRole("button", { name: /Judgment/ });
    // RAC keeps collapsed content in the DOM under [hidden] — visibility is the contract.
    expect(screen.getByText("Rating")).not.toBeVisible();
    await userEvent.click(head);
    expect(screen.getByText("Rating")).toBeVisible();
    expect(head.getAttribute("aria-expanded")).toBe("true");
});
