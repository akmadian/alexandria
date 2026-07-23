// Kbd is presentational: it renders a native <kbd>, applies the style class, and KbdGroup keeps
// each key its own <kbd>. These pin the element + the style-class dispatch (the completeness gate
// itself is compile-time — STYLE_CLASSES).

import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { Kbd, KbdGroup } from "./kbd";

test("renders a native <kbd> carrying the key", () => {
    render(<Kbd>⌘</Kbd>);
    expect(screen.getByText("⌘").tagName).toBe("KBD");
});

test("defaults to the flat style at the sm (menu) size", () => {
    const { container } = render(<Kbd>E</Kbd>);
    const cap = container.querySelector("kbd");
    expect(cap?.className).toContain("flat");
    expect(cap?.className).not.toContain("keycap");
    expect(cap?.className).toContain("sm");
});

test("keycap style swaps the class", () => {
    const { container } = render(<Kbd style="keycap">E</Kbd>);
    expect(container.querySelector("kbd")?.className).toContain("keycap");
});

test("icon renders a vector glyph inside the cap (the modifier path)", () => {
    // Modifier symbols mush as 11px font text, so they render as icons instead.
    const { container } = render(<Kbd icon="command" />);
    const cap = container.querySelector("kbd");
    expect(cap?.querySelector("svg")).toBeTruthy();
});

test("KbdGroup renders each key as its own <kbd>", () => {
    const { container } = render(
        <KbdGroup>
            <Kbd>⌘</Kbd>
            <Kbd>⇧</Kbd>
            <Kbd>P</Kbd>
        </KbdGroup>,
    );
    expect(container.querySelectorAll("kbd")).toHaveLength(3);
});
