// Row's contract: the intent binds structure class + slot roles by construction,
// the section's context supplies the intent, and §13's hover-reveal rides string
// values. The children-only-on-control rule is compile-time (RowProps union).

import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { Row, RowIntentProvider } from "./row";

test("each intent applies its structure class and registered slot roles", () => {
    const { container } = render(
        <>
            <Row intent="control" label="Rating" value="unrated" />
            <Row intent="list" label="Iceland" value="1,204" />
            <Row intent="text" label="ISO" value="400" />
        </>,
    );
    const [control, list, text] = [...container.querySelectorAll("div")];
    expect(control.className).toContain("control");
    expect(control.querySelector("span")?.className).toContain("alx-type-label");
    expect(list.className).toContain("list");
    expect(list.querySelectorAll("span")[1]?.className).toContain("alx-type-data-sm");
    expect(text.className).toContain("text");
    expect(text.querySelector("span")?.className).toContain("alx-type-label-sm");
});

test("rows inherit the section's intent and may override it", () => {
    const { container } = render(
        <RowIntentProvider intent="text">
            <Row label="Aperture" value="ƒ/1.8" />
            <Row intent="list" label="explicit" value="9" />
        </RowIntentProvider>,
    );
    const [inherited, overridden] = [...container.querySelectorAll("div")];
    expect(inherited.className).toContain("text");
    expect(overridden.className).toContain("list");
});

test("string values hover-reveal in full (§13)", () => {
    render(<Row intent="text" label="Lens" value="XF 56mm f/1.2 R WR with a very long name" />);
    expect(screen.getByTitle("XF 56mm f/1.2 R WR with a very long name")).toBeInTheDocument();
});

test("a bare row (no provider, no intent) defaults to control", () => {
    const { container } = render(<Row label="Rating" />);
    expect(container.querySelector("div")?.className).toContain("control");
});

test("non-string slots render without a hover title; labels reveal like values (§13)", () => {
    render(<Row intent="list" label="A very long collection name" value={<em>12</em>} />);
    expect(screen.getByTitle("A very long collection name")).toBeInTheDocument();
    expect(screen.getByText("12").closest("span[title]")).toBeNull();
});

test("control rows carry the control slot", () => {
    render(
        <Row intent="control" label="Flag">
            <button>Pick</button>
        </Row>,
    );
    expect(screen.getByRole("button", { name: "Pick" })).toBeInTheDocument();
});
