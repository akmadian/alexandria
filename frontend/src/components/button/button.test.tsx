// Button drives RAC's press semantics under our rung/size API — these pin the
// contract: rung/size classes land on the element, presses fire, disabled blocks.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { Button } from "./button";

test("applies the rung and size classes on the rendered button", () => {
    render(<Button rung="fill" size="lg">Import</Button>);
    const button = screen.getByRole("button", { name: "Import" });
    expect(button.className).toContain("fill");
    expect(button.className).toContain("controlLarge");
});

test("defaults to the outline rung at control size", () => {
    render(<Button>Import</Button>);
    const button = screen.getByRole("button", { name: "Import" });
    expect(button.className).toContain("outline");
    expect(button.className).toContain("control");
});

test("onPress fires through RAC's press semantics", async () => {
    const onPress = vi.fn();
    render(<Button onPress={onPress}>Import</Button>);
    await userEvent.click(screen.getByRole("button", { name: "Import" }));
    expect(onPress).toHaveBeenCalledTimes(1);
});

test("isDisabled blocks the press and marks the element", async () => {
    const onPress = vi.fn();
    render(<Button isDisabled onPress={onPress}>Import</Button>);
    const button = screen.getByRole("button", { name: "Import" });
    await userEvent.click(button);
    expect(onPress).not.toHaveBeenCalled();
    expect(button).toBeDisabled();
});

test("the xs rung lands its class (the dense-inline tier)", () => {
    render(<Button size="xs">Import</Button>);
    expect(screen.getByRole("button", { name: "Import" }).className).toContain("controlXsmall");
});
