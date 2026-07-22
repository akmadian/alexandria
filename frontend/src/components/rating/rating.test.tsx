// Rating pins the controlled contract: display mode is a pure readout (zero
// tab stops — content surfaces reserve keys for grid navigation), interactive
// mode encodes the gesture grammar once — star n proposes n, the current value
// proposes null (the clear).

import { fireEvent, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { Rating } from "./rating";

test("display mode: five positions, accurate state label, zero tab stops", () => {
    render(<Rating value={3} />);
    const readout = screen.getByLabelText("Rated 3");
    expect(readout.querySelectorAll("svg")).toHaveLength(5);
    expect(readout.querySelectorAll("button")).toHaveLength(0);
});

test("display mode: null reads unrated and still shows the five positions", () => {
    render(<Rating value={null} />);
    expect(screen.getByLabelText("Unrated").querySelectorAll("svg")).toHaveLength(5);
});

test("a defensive 0 renders like null — unrated (the contract: 0 is not a rating)", () => {
    render(<Rating value={0} />);
    expect(screen.getByLabelText("Unrated").querySelectorAll("svg")).toHaveLength(5);
});

test("interactive: clicking star n proposes n", () => {
    const onChange = vi.fn();
    render(<Rating value={null} onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Rate 3 stars" }));
    expect(onChange).toHaveBeenCalledWith(3);
});

test("interactive: clicking the current value proposes null (the clear)", () => {
    const onChange = vi.fn();
    render(<Rating value={4} onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Clear rating" }));
    expect(onChange).toHaveBeenCalledWith(null);
});

test("the size prop maps to its tier class", () => {
    const { container } = render(<Rating value={3} size="xs" />);
    expect((container.firstChild as HTMLElement).className).toContain("sizeXs");
});
