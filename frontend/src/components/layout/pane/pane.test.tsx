// Pane — the resize/collapse/persist contract, driven through the keyboard (the APG Window
// Splitter path). Pointer drag can't be exercised in happy-dom (no layout), but arrow keys and
// Enter/Home/End go through the same width state, so they cover the logic.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { Pane } from "./pane";

afterEach(() => localStorage.clear());

function renderPane(props: Partial<React.ComponentProps<typeof Pane>> = {}) {
    return render(
        <Pane side="left" aria-label="Sources" defaultWidth={260} minWidth={200} maxWidth={480} {...props}>
            <div>rail body</div>
        </Pane>,
    );
}

test("renders the separator with the APG value contract and shows its children", () => {
    renderPane();
    const handle = screen.getByRole("separator", { name: "Resize Sources" });
    expect(handle).toHaveAttribute("aria-orientation", "vertical");
    expect(handle).toHaveAttribute("aria-valuenow", "260");
    expect(handle).toHaveAttribute("aria-valuemin", "0");
    expect(handle).toHaveAttribute("aria-valuemax", "480");
    expect(screen.getByText("rail body")).toBeVisible();
});

test("initial width is clamped into [min,max]", () => {
    renderPane({ defaultWidth: 9000 });
    expect(screen.getByRole("separator").getAttribute("aria-valuenow")).toBe("480");
});

test("arrow keys resize; sign follows the docked side", async () => {
    renderPane();
    const handle = screen.getByRole("separator");
    handle.focus();
    // Left pane: ArrowRight grows the pane (handle on the right edge).
    await userEvent.keyboard("{ArrowRight}");
    expect(Number(handle.getAttribute("aria-valuenow"))).toBeGreaterThan(260);
    const grown = Number(handle.getAttribute("aria-valuenow"));
    await userEvent.keyboard("{ArrowLeft}");
    expect(Number(handle.getAttribute("aria-valuenow"))).toBeLessThan(grown);
});

test("Home jumps to min width, End to max", async () => {
    renderPane();
    const handle = screen.getByRole("separator");
    handle.focus();
    await userEvent.keyboard("{Home}");
    expect(handle.getAttribute("aria-valuenow")).toBe("200");
    await userEvent.keyboard("{End}");
    expect(handle.getAttribute("aria-valuenow")).toBe("480");
});

test("Enter collapses (uncontrolled) and restores", async () => {
    const { container } = renderPane();
    const pane = container.querySelector("section")!;
    const handle = screen.getByRole("separator");
    handle.focus();
    await userEvent.keyboard("{Enter}");
    expect(pane).toHaveAttribute("data-collapsed");
    expect(handle.getAttribute("aria-valuenow")).toBe("0");
    expect(screen.getByText("rail body")).not.toBeVisible();
    await userEvent.keyboard("{Enter}");
    expect(pane).not.toHaveAttribute("data-collapsed");
    expect(screen.getByText("rail body")).toBeVisible();
});

test("controlled collapse: the prop wins and onCollapsedChange fires without self-collapsing", async () => {
    const onCollapsedChange = vi.fn();
    const { container } = renderPane({ isCollapsed: false, onCollapsedChange });
    const pane = container.querySelector("section")!;
    screen.getByRole("separator").focus();
    await userEvent.keyboard("{Enter}");
    // Prop pins it open despite the toggle; the consumer is told to flip it.
    expect(pane).not.toHaveAttribute("data-collapsed");
    expect(onCollapsedChange).toHaveBeenCalledWith(true);
});

test("width and collapsed persist to localStorage and are read back pre-paint", async () => {
    const first = renderPane({ storageKey: "alx.pane.test" });
    const handle = screen.getByRole("separator");
    handle.focus();
    await userEvent.keyboard("{End}"); // -> 480, persisted
    await userEvent.keyboard("{Enter}"); // -> collapsed, persisted
    expect(JSON.parse(localStorage.getItem("alx.pane.test")!)).toEqual({ width: 480, collapsed: true });
    first.unmount();

    // A fresh mount ignores its own defaultWidth and restores the stored state.
    renderPane({ storageKey: "alx.pane.test", defaultWidth: 260 });
    const remounted = screen.getByRole("separator");
    expect(remounted.getAttribute("aria-valuenow")).toBe("0"); // collapsed
    expect(screen.getByText("rail body")).not.toBeVisible();
});

test("a throwing child is caught by the built-in boundary, not the caller", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    function Bomb(): React.ReactNode {
        throw new Error("boom");
    }
    expect(() =>
        render(
            <Pane side="left" aria-label="Sources">
                <Bomb />
            </Pane>,
        ),
    ).not.toThrow();
    expect(screen.getByRole("button", { name: "Reload panel" })).toBeInTheDocument();
    spy.mockRestore();
});
