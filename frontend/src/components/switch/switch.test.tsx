// Switch drives RAC's switch semantics: clicking toggles the switch role both
// ways, defaultSelected renders on, disabled blocks — and the state lands as
// the data-attribute the CSS keys on.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { Switch } from "./switch";

test("clicking toggles the switch both ways", async () => {
    const onChange = vi.fn();
    const { container } = render(<Switch onChange={onChange}>Auto-advance</Switch>);
    const input = screen.getByRole("switch", { name: "Auto-advance" });
    expect(input).not.toBeChecked();
    await userEvent.click(input);
    expect(input).toBeChecked();
    expect(container.querySelector("label")?.hasAttribute("data-selected")).toBe(true);
    await userEvent.click(input);
    expect(input).not.toBeChecked();
    expect(onChange).toHaveBeenCalledTimes(2);
});

test("defaultSelected renders on", () => {
    const { container } = render(<Switch defaultSelected>Watch folder</Switch>);
    expect(screen.getByRole("switch", { name: "Watch folder" })).toBeChecked();
    expect(container.querySelector("label")?.hasAttribute("data-selected")).toBe(true);
});

test("disabled blocks the toggle and keeps an ON track readable", async () => {
    const onChange = vi.fn();
    const { container } = render(
        <Switch isDisabled defaultSelected onChange={onChange}>
            Locked
        </Switch>,
    );
    await userEvent.click(screen.getByRole("switch", { name: "Locked" }));
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("switch", { name: "Locked" })).toBeChecked();
    expect(container.querySelector("label")?.hasAttribute("data-disabled")).toBe(true);
});

test("the size prop maps to its tier class", () => {
    const { container } = render(<Switch size="xs">Dense</Switch>);
    expect(container.querySelector("label")?.className).toContain("sizeXs");
});
