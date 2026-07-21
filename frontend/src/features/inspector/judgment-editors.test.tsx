// The inspector Judgment editors turn a gesture into an absolute triage patch
// against their single subject. Rating and the note are plain elements driven by
// fireEvent; the label/flag pads compose the RAC ToggleButton, driven by
// userEvent like the primitive's own suite. i18n is initialized by the test
// setup, so aria-labels resolve to English.

import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { FlagEditor, LabelEditor, NoteEditor, RatingEditor } from "./judgment-editors";

describe("RatingEditor", () => {
    it("proposes the clicked rating, and null when re-clicking the current value", () => {
        const write = vi.fn();
        const { rerender } = render(<RatingEditor value={null} write={write} />);
        fireEvent.click(screen.getByRole("button", { name: "Rate 3 stars" }));
        expect(write).toHaveBeenCalledWith({ rating: 3 });

        rerender(<RatingEditor value={4} write={write} />);
        fireEvent.click(screen.getByRole("button", { name: "Clear rating" }));
        expect(write).toHaveBeenCalledWith({ rating: null });
    });
});

describe("LabelEditor", () => {
    it("sets the clicked label, and clears when the active one is re-clicked (toggle)", async () => {
        const write = vi.fn();
        const { rerender } = render(<LabelEditor value={null} write={write} />);
        await userEvent.click(screen.getByRole("button", { name: "Red" }));
        expect(write).toHaveBeenCalledWith({ colorLabel: "red" });

        rerender(<LabelEditor value="red" write={write} />);
        const active = screen.getByRole("button", { name: "Red" });
        expect(active).toHaveAttribute("aria-pressed", "true");
        await userEvent.click(active);
        expect(write).toHaveBeenCalledWith({ colorLabel: null });
    });
});

describe("FlagEditor", () => {
    it("sets pick / reject, and clears when the active flag is re-clicked", async () => {
        const write = vi.fn();
        const { rerender } = render(<FlagEditor value={null} write={write} />);
        await userEvent.click(screen.getByRole("button", { name: "Flag as pick" }));
        expect(write).toHaveBeenCalledWith({ flag: "pick" });
        await userEvent.click(screen.getByRole("button", { name: "Flag as reject" }));
        expect(write).toHaveBeenCalledWith({ flag: "reject" });

        rerender(<FlagEditor value="pick" write={write} />);
        await userEvent.click(screen.getByRole("button", { name: "Flag as pick" }));
        expect(write).toHaveBeenCalledWith({ flag: null });
    });
});

describe("NoteEditor", () => {
    it("commits a changed note on blur", () => {
        const write = vi.fn();
        render(<NoteEditor value={null} write={write} />);
        const input = screen.getByRole("textbox");
        fireEvent.change(input, { target: { value: "check focus" } });
        fireEvent.blur(input);
        expect(write).toHaveBeenCalledWith({ note: "check focus" });
    });

    it("skips an unchanged blur (seeded from the current value)", () => {
        const write = vi.fn();
        render(<NoteEditor value="keep" write={write} />);
        fireEvent.blur(screen.getByRole("textbox"));
        expect(write).not.toHaveBeenCalled();
    });

    it("clears with null when emptied", () => {
        const write = vi.fn();
        render(<NoteEditor value="keep" write={write} />);
        const input = screen.getByRole("textbox");
        fireEvent.change(input, { target: { value: "" } });
        fireEvent.blur(input);
        expect(write).toHaveBeenCalledWith({ note: null });
    });
});
