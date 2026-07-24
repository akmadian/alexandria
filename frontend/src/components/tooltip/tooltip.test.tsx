// Tooltip is presentational chrome over RAC's Tooltip: RAC owns behavior (hover/focus/dismiss);
// this pins the variant→class dispatch (the completeness gate itself is compile-time —
// VARIANT_CLASSES). Forced open via `isOpen` so the portal mounts under happy-dom.

import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { Button } from "@/components/button/button";
import { Tooltip, TooltipTrigger } from "./tooltip";

test("renders the label, dark variant by default", () => {
    render(
        <TooltipTrigger isOpen>
            <Button>trigger</Button>
            <Tooltip>Rotate 90° left</Tooltip>
        </TooltipTrigger>,
    );
    const tip = screen.getByText("Rotate 90° left");
    expect(tip.className).toContain("dark");
    expect(tip.className).not.toContain("inverse");
});

test("inverse variant swaps the class", () => {
    render(
        <TooltipTrigger isOpen>
            <Button>trigger</Button>
            <Tooltip variant="inverse">Rotate 90° left</Tooltip>
        </TooltipTrigger>,
    );
    expect(screen.getByText("Rotate 90° left").className).toContain("inverse");
});
