// Badge encodes the tagRecipes: each style maps (style, hue) onto the right scale
// tokens. These pin that mapping — the recipe is the whole point of the primitive.

import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { Badge } from "./badge";

test("tint binds the hue's tint background + ink", () => {
    render(
        <Badge hue="blue" style="tint">
            RAW
        </Badge>,
    );
    const inline = screen.getByText("RAW").getAttribute("style") ?? "";
    expect(inline).toContain("var(--alx-color-blue-tint)");
    expect(inline).toContain("var(--alx-color-blue-tint-ink)");
});

test("outline adds the hue's line border", () => {
    render(
        <Badge hue="green" style="outline">
            Vector
        </Badge>,
    );
    expect(screen.getByText("Vector").getAttribute("style")).toContain("var(--alx-color-green-line)");
});

test("fill uses the solid + on-solid pairing", () => {
    render(
        <Badge hue="red" style="fill">
            Reject
        </Badge>,
    );
    const inline = screen.getByText("Reject").getAttribute("style") ?? "";
    expect(inline).toContain("var(--alx-color-red-solid)");
    expect(inline).toContain("var(--alx-color-red-on-solid)");
});

test("dot renders a colored mark before neutral text", () => {
    const { container } = render(
        <Badge hue="purple" style="dot">
            landscape
        </Badge>,
    );
    expect(screen.getByText("landscape")).toBeInTheDocument();
    expect(container.querySelector("span > span")?.getAttribute("style")).toContain("var(--alx-color-purple-solid)");
});

test("defaults to the tint style", () => {
    render(<Badge hue="amber">Pending</Badge>);
    expect(screen.getByText("Pending").getAttribute("style")).toContain("var(--alx-color-amber-tint)");
});

test("every size rung renders the recipe unchanged", () => {
    // Sizes are CSS-only rungs (type role + pads); the recipe colors must not
    // vary with them. The completeness gate is compile-time (SIZE_CLASSES).
    for (const size of ["inline", "standard", "prominent"] as const) {
        const { unmount } = render(
            <Badge hue="teal" size={size}>
                chip
            </Badge>,
        );
        expect(screen.getByText("chip").getAttribute("style")).toContain("var(--alx-color-teal-tint)");
        unmount();
    }
});
