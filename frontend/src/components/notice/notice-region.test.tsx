// The loud-failure floor end to end: pushNotice renders the resolved i18n string
// in the alert region, dismiss removes it, and the auto-expiry sweeps it without
// user action. The store is module-global; each case cleans up what it raised.

import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pushNotice } from "@/stores/notices";
import { NoticeRegion } from "./notice-region";

const MESSAGE = "Couldn't save that change. It was reverted.";

afterEach(() => {
    vi.useRealTimers();
});

describe("NoticeRegion", () => {
    it("renders a raised notice as a visible alert, resolved through i18n, until dismissed", async () => {
        render(<NoticeRegion />);
        act(() => pushNotice("errors.writeFailed"));

        expect(screen.getByRole("alert")).toBeInTheDocument();
        expect(screen.getByText(MESSAGE)).toBeVisible();

        // Dismiss removes it (and leaves the global store empty for the next case).
        await userEvent.click(screen.getByRole("button", { name: "Close" }));
        expect(screen.queryByText(MESSAGE)).not.toBeInTheDocument();
    });

    it("renders nothing at all while no notice is live", () => {
        render(<NoticeRegion />);
        expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("auto-expires a notice after its window", () => {
        vi.useFakeTimers();
        render(<NoticeRegion />);
        act(() => pushNotice("errors.writeFailed"));
        expect(screen.getByText(MESSAGE)).toBeVisible();

        act(() => vi.advanceTimersByTime(6001));
        expect(screen.queryByText(MESSAGE)).not.toBeInTheDocument();
    });
});
