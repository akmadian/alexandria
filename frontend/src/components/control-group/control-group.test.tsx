// ControlGroup renders its rows and publishes the shared label-column width as the
// --control-row-label custom property its rows read.

import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { ControlRow } from "@/components/control-row/control-row";
import { ControlGroup } from "./control-group";

test("renders its rows", () => {
    render(
        <ControlGroup>
            <ControlRow label="ISO">3200</ControlRow>
            <ControlRow label="Aperture">ƒ/1.8</ControlRow>
        </ControlGroup>,
    );
    expect(screen.getByText("ISO")).toBeVisible();
    expect(screen.getByText("Aperture")).toBeVisible();
});

test("sets the shared label-column width, defaulting to 40%", () => {
    const { container } = render(
        <ControlGroup>
            <ControlRow label="ISO">3200</ControlRow>
        </ControlGroup>,
    );
    expect((container.firstElementChild as HTMLElement).style.getPropertyValue("--control-row-label")).toBe("40%");
});

test("labelWidth overrides the shared column", () => {
    const { container } = render(
        <ControlGroup labelWidth="120px">
            <ControlRow label="ISO">3200</ControlRow>
        </ControlGroup>,
    );
    expect((container.firstElementChild as HTMLElement).style.getPropertyValue("--control-row-label")).toBe("120px");
});

test("gap lands the spaced class for filled chip-lists", () => {
    const { container } = render(
        <ControlGroup gap>
            <ControlRow filled label="Salesperson" />
        </ControlGroup>,
    );
    expect((container.firstElementChild as HTMLElement).className).toContain("gapped");
});
