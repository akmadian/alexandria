// The hash switch between the shell and the design library is the app's whole
// routing surface — these pin both directions (task-24 deliverable 3).

import { act, render, screen } from "@testing-library/react";
import { beforeEach, expect, test } from "vitest";
import { App } from "./app";

beforeEach(() => {
    window.location.hash = "";
});

test("renders the shell (with the mock catalog's count) by default", async () => {
    render(<App />);
    expect(screen.getByText("Library")).toBeInTheDocument();
    expect(screen.getByText(/The grid arrives/)).toBeInTheDocument();
    expect(await screen.findByText(/assets/)).toBeInTheDocument();
});

test("the hash switches to the design library and back", async () => {
    render(<App />);
    act(() => {
        window.location.hash = "#/design-library";
        window.dispatchEvent(new Event("hashchange"));
    });
    expect(await screen.findByRole("heading", { name: /Alexandria design library/ })).toBeInTheDocument();
    act(() => {
        window.location.hash = "#/";
        window.dispatchEvent(new Event("hashchange"));
    });
    expect(screen.getByText(/The grid arrives/)).toBeInTheDocument();
});
