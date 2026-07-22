// SegmentedControl drives RAC's single-select ToggleButtonGroup under our friendly
// single-key API: exactly one segment is ever lit, picking one un-lights the rest,
// the already-lit one can't be un-picked (empty selection is impossible), and the pick
// surfaces through onChange as a bare key.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { Segment, SegmentedControl } from "./segmented-control";

function ViewModes(props: { onChange?: (key: string | number) => void; defaultValue?: string }) {
    return (
        <SegmentedControl aria-label="View" defaultValue={props.defaultValue ?? "grid"} onChange={props.onChange}>
            <Segment id="grid">Grid</Segment>
            <Segment id="loupe">Loupe</Segment>
            <Segment id="compare">Compare</Segment>
        </SegmentedControl>
    );
}

test("exactly one segment is lit, and picking another moves the light", async () => {
    const onChange = vi.fn();
    render(<ViewModes onChange={onChange} />);
    const grid = screen.getByRole("radio", { name: "Grid" });
    const loupe = screen.getByRole("radio", { name: "Loupe" });
    expect(grid.hasAttribute("data-selected")).toBe(true);
    expect(loupe.hasAttribute("data-selected")).toBe(false);

    await userEvent.click(loupe);
    expect(onChange).toHaveBeenLastCalledWith("loupe");
    expect(grid.hasAttribute("data-selected")).toBe(false);
    expect(loupe.hasAttribute("data-selected")).toBe(true);
});

test("the lit segment cannot be un-lit — empty selection is impossible", async () => {
    render(<ViewModes />);
    const grid = screen.getByRole("radio", { name: "Grid" });
    await userEvent.click(grid);
    expect(grid.hasAttribute("data-selected")).toBe(true);
});

test("the size class lands on the track", () => {
    render(
        <SegmentedControl aria-label="View" size="lg" defaultValue="a">
            <Segment id="a">A</Segment>
            <Segment id="b">B</Segment>
        </SegmentedControl>,
    );
    // The group element carries the size class; segments inherit height via descendant CSS.
    const group = screen.getByRole("radiogroup", { name: "View" });
    expect(group.className).toContain("controlLarge");
});

test("the sm rung lands its class on the track (the ladder floor for a track)", () => {
    render(
        <SegmentedControl aria-label="Density" size="sm" defaultValue="a">
            <Segment id="a">A</Segment>
            <Segment id="b">B</Segment>
        </SegmentedControl>,
    );
    expect(screen.getByRole("radiogroup", { name: "Density" }).className).toContain("controlSmall");
});
