// The hash switch between the workspace and the design library is the app's whole
// routing surface — these pin both directions (task-24 deliverable 3). The
// workspace itself is the tab strip; its behavior lives in workspace.test.tsx.

import { act, render, screen } from "@testing-library/react";
import { beforeEach, expect, test } from "vitest";
import { App } from "./app";

beforeEach(() => {
    window.location.hash = "";
});

test("renders the workspace with the Catalog grid over the mock catalog", async () => {
    render(<App />);
    expect(screen.getByRole("tab", { name: "Catalog" })).toBeInTheDocument();
    expect(await screen.findByText(/assets/)).toBeInTheDocument();
    expect((await screen.findAllByRole("img")).length).toBeGreaterThan(0);
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
    expect(screen.getByRole("tab", { name: "Catalog" })).toBeInTheDocument();
});
