// Checkbox drives RAC's checkbox semantics: clicking toggles, indeterminate is
// the §25 mixed state with its own registered glyph, the glyph is ABSENT (not
// hidden) when unchecked, and disabled/invalid land as the data-attributes the
// CSS keys on. Lucide stamps a per-glyph class (lucide-check / lucide-minus),
// which is how the tests tell the marks apart.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { Checkbox } from "./checkbox";

test("clicking toggles the checked state and renders the check mark", async () => {
    const onChange = vi.fn();
    const { container } = render(<Checkbox onChange={onChange}>Reject</Checkbox>);
    const input = screen.getByRole("checkbox", { name: "Reject" });
    expect(input).not.toBeChecked();
    expect(container.querySelector("svg")).toBeNull();
    await userEvent.click(input);
    expect(input).toBeChecked();
    expect(container.querySelector("label")?.hasAttribute("data-selected")).toBe(true);
    expect(container.querySelector(".lucide-check")).not.toBeNull();
    await userEvent.click(input);
    expect(input).not.toBeChecked();
    expect(onChange).toHaveBeenCalledTimes(2);
});

test("indeterminate renders the mixed glyph and reports mixed to ARIA", () => {
    const { container } = render(<Checkbox isIndeterminate>Some selected</Checkbox>);
    expect(screen.getByRole("checkbox", { name: "Some selected" })).toBePartiallyChecked();
    expect(container.querySelector("label")?.hasAttribute("data-indeterminate")).toBe(true);
    expect(container.querySelector(".lucide-minus")).not.toBeNull();
    expect(container.querySelector(".lucide-check")).toBeNull();
});

test("disabled blocks the toggle and keeps a checked mark readable", async () => {
    const onChange = vi.fn();
    const { container } = render(
        <Checkbox isDisabled defaultSelected onChange={onChange}>
            Locked
        </Checkbox>,
    );
    await userEvent.click(screen.getByRole("checkbox", { name: "Locked" }));
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("checkbox", { name: "Locked" })).toBeChecked();
    expect(container.querySelector(".lucide-check")).not.toBeNull();
});

test("invalid lands as the data-attribute the error hairline keys on", () => {
    const { container } = render(<Checkbox isInvalid>Required</Checkbox>);
    expect(container.querySelector("label")?.hasAttribute("data-invalid")).toBe(true);
});
