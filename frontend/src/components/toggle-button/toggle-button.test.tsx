// ToggleButton drives RAC's toggle semantics under our size API: pressing flips
// the selected state, disabled blocks it, and the state lands as the
// data-attribute the CSS keys on.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { ToggleButton } from "./toggle-button";

test("pressing toggles the selected state both ways", async () => {
    const onChange = vi.fn();
    render(<ToggleButton onChange={onChange}>Flagged</ToggleButton>);
    const toggle = screen.getByRole("button", { name: "Flagged" });
    expect(toggle.getAttribute("aria-pressed")).toBe("false");
    await userEvent.click(toggle);
    expect(toggle.getAttribute("aria-pressed")).toBe("true");
    expect(toggle.hasAttribute("data-selected")).toBe(true);
    await userEvent.click(toggle);
    expect(toggle.getAttribute("aria-pressed")).toBe("false");
    expect(onChange).toHaveBeenCalledTimes(2);
});

test("defaultSelected renders on, and the size class applies", () => {
    render(<ToggleButton defaultSelected size="lg">Raw only</ToggleButton>);
    const toggle = screen.getByRole("button", { name: "Raw only" });
    expect(toggle.hasAttribute("data-selected")).toBe(true);
    expect(toggle.className).toContain("controlLarge");
});

test("disabled blocks the toggle and keeps a selected fill readable", async () => {
    const onChange = vi.fn();
    render(
        <ToggleButton isDisabled defaultSelected onChange={onChange}>
            Locked
        </ToggleButton>,
    );
    const toggle = screen.getByRole("button", { name: "Locked" });
    await userEvent.click(toggle);
    expect(onChange).not.toHaveBeenCalled();
    expect(toggle.hasAttribute("data-selected")).toBe(true);
    expect(toggle).toBeDisabled();
});

test("the xs rung lands its class", () => {
    render(<ToggleButton size="xs">Raw</ToggleButton>);
    expect(screen.getByRole("button", { name: "Raw" }).className).toContain("controlXsmall");
});
