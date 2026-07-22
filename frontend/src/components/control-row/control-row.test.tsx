// ControlRow is presentational: it renders the label + the hosted content, steps its
// height on the control ladder, and steps its label role with the tier. The hosted
// control keeps its own accessible name (asserted via the button it hosts).

import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { Button } from "@/components/button/button";
import { ControlRow, type ControlRowSize } from "./control-row";

test("renders the label and the hosted control (which keeps its own name)", () => {
    render(
        <ControlRow label="Sharpen">
            <Button aria-label="Toggle sharpen">On</Button>
        </ControlRow>,
    );
    expect(screen.getByText("Sharpen")).toBeVisible();
    expect(screen.getByRole("button", { name: "Toggle sharpen" })).toBeVisible();
});

test("defaults to the md height tier", () => {
    const { container } = render(<ControlRow label="Quality">86</ControlRow>);
    expect(container.firstElementChild?.className).toContain("controlMedium");
});

test("each size lands its height class and steps the label role", () => {
    const cases: readonly [ControlRowSize, string, string][] = [
        ["xs", "controlXsmall", "alx-type-control-xs"],
        ["sm", "controlSmall", "alx-type-control-sm"],
        ["md", "controlMedium", "alx-type-control"],
        ["lg", "controlLarge", "alx-type-control-lg"],
    ];
    for (const [size, heightClass, labelRole] of cases) {
        const { container } = render(
            <ControlRow label="Label" size={size}>
                <Button>Go</Button>
            </ControlRow>,
        );
        const row = container.firstElementChild;
        expect(row?.className).toContain(heightClass);
        expect(row?.querySelector("span")?.className).toContain(labelRole);
    }
});
