// The crash path is the component's whole job: a throwing child renders the
// fallback (logged), and reset remounts the subtree clean via the epoch key.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { I18nextProvider } from "react-i18next";
import i18n from "@/i18n";
import * as logger from "@/lib/logger";
import { PaneErrorBoundary } from "./error-boundary";

function Bomb({ defused }: { defused: boolean }) {
    if (!defused) throw new Error("boom");
    return <div>healthy pane</div>;
}

test("a throwing child renders the logged fallback; reset remounts clean", async () => {
    const logSpy = vi.spyOn(logger.log, "error").mockImplementation(() => undefined);
    // React logs boundary-caught errors to console.error; silence the noise.
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => undefined);

    let defused = false;
    const { rerender } = render(
        <I18nextProvider i18n={i18n}>
            <PaneErrorBoundary>
                <Bomb defused={defused} />
            </PaneErrorBoundary>
        </I18nextProvider>,
    );

    expect(screen.getByText("This panel hit an error.")).toBeInTheDocument();
    expect(logSpy).toHaveBeenCalledWith("pane crashed", { error: "Error: boom" });

    // The child's failure condition clears (the analog of fresh data after an
    // invalidation), then the user resets — the subtree must remount healthy.
    defused = true;
    rerender(
        <I18nextProvider i18n={i18n}>
            <PaneErrorBoundary>
                <Bomb defused={defused} />
            </PaneErrorBoundary>
        </I18nextProvider>,
    );
    await userEvent.click(screen.getByRole("button", { name: "Reload panel" }));
    expect(screen.getByText("healthy pane")).toBeInTheDocument();

    logSpy.mockRestore();
    consoleSpy.mockRestore();
});
