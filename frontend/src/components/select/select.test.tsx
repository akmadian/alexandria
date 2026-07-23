// The trigger shows the selected value (or the placeholder) and carries its accessible
// name. Opening the popover + selecting is RAC overlay behavior — verified in the real
// browser (frontend/CLAUDE.md §8: portals/overlays are unreliable in happy-dom).

import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { Select, SelectItem } from "./select";

test("the trigger shows the selected value", () => {
    render(
        <Select label="View" defaultSelectedKey="loupe">
            <SelectItem id="grid">Grid</SelectItem>
            <SelectItem id="loupe">Loupe</SelectItem>
            <SelectItem id="compare">Compare</SelectItem>
        </Select>,
    );
    expect(screen.getByRole("button")).toHaveTextContent("Loupe");
});

test("an empty select shows the placeholder", () => {
    render(
        <Select label="Axis" placeholder="Choose…">
            <SelectItem id="date">Date</SelectItem>
        </Select>,
    );
    expect(screen.getByRole("button")).toHaveTextContent("Choose…");
});

test("the size class lands on the trigger", () => {
    render(
        <Select label="View" size="xs" defaultSelectedKey="grid">
            <SelectItem id="grid">Grid</SelectItem>
        </Select>,
    );
    expect(screen.getByRole("button").className).toContain("controlXsmall");
});
