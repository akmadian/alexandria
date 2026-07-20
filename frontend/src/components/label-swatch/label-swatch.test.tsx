// The one ColorLabel → swatch dispatch (C10): every generated label renders
// its registered color, and the accessible name passes through.

import { render } from "@testing-library/react";
import { expect, test } from "vitest";
import type { ColorLabel } from "@/_generated-types/enums";
import { LabelSwatch } from "./label-swatch";

const LABELS: ColorLabel[] = ["red", "orange", "yellow", "green", "blue", "purple"];

test("every label renders a colored mark", () => {
    for (const label of LABELS) {
        const { container, unmount } = render(<LabelSwatch label={label} />);
        const swatch = container.firstElementChild as HTMLElement;
        expect(swatch.style.backgroundColor).not.toBe("");
        unmount();
    }
});

test("the accessible name passes through", () => {
    const { getByLabelText } = render(<LabelSwatch label="red" aria-label="Red label" />);
    expect(getByLabelText("Red label")).toBeInTheDocument();
});
