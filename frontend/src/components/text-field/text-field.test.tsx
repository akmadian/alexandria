// TextField drives RAC's field semantics: the label names the textbox, typing
// reaches onChange, the description wires through aria-describedby, FieldError
// renders ONLY while invalid, and disabled blocks input.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { TextField } from "./text-field";

test("typing drives onChange and the label names the textbox", async () => {
    const onChange = vi.fn();
    render(<TextField label="Collection name" onChange={onChange} />);
    const input = screen.getByRole("textbox", { name: "Collection name" });
    await userEvent.type(input, "Iceland");
    expect(input).toHaveValue("Iceland");
    expect(onChange).toHaveBeenLastCalledWith("Iceland");
});

test("description renders and is wired through aria-describedby", () => {
    render(<TextField label="Name" description="Shown in the panel tree" />);
    const input = screen.getByRole("textbox", { name: "Name" });
    const description = screen.getByText("Shown in the panel tree");
    expect(input.getAttribute("aria-describedby")).toContain(description.id);
});

test("the error message renders only while invalid", () => {
    const { rerender, container } = render(
        <TextField label="Name" errorMessage="Name is taken" />,
    );
    expect(screen.queryByText("Name is taken")).toBeNull();
    rerender(<TextField label="Name" errorMessage="Name is taken" isInvalid />);
    expect(screen.getByText("Name is taken")).toBeVisible();
    expect(container.firstElementChild?.hasAttribute("data-invalid")).toBe(true);
});

test("disabled blocks typing and the size class applies", async () => {
    const onChange = vi.fn();
    render(<TextField label="Locked" size="control-lg" isDisabled onChange={onChange} />);
    const input = screen.getByRole("textbox", { name: "Locked" });
    await userEvent.type(input, "nope");
    expect(onChange).not.toHaveBeenCalled();
    expect(input).toBeDisabled();
    expect(input.className).toContain("controlLarge");
});
