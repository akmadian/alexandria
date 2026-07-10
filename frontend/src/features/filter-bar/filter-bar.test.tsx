import { act, render, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { leaf } from "@/query-model/registry";
import { useCatalogDispatch, useFilter } from "@/stores/catalog-store";
import { FilterBar } from "./filter-bar";

// The store is a module singleton; grab its stable dispatch once and reset between
// tests. Reads go through a fresh renderHook so they see the current snapshot.
const dispatch = renderHook(() => useCatalogDispatch()).result.current;
const currentFilter = () => renderHook(() => useFilter()).result.current;

afterEach(() => act(() => dispatch({ type: "filter-replaced", filter: null })));

describe("FilterBar", () => {
    it("renders an enum pill with its i18n-formatted membership value", () => {
        act(() => dispatch({ type: "filter-replaced", filter: leaf("fileType", "in", ["image", "raw"]) }));
        render(<FilterBar />);
        expect(screen.getByText("File type")).toBeInTheDocument();
        expect(screen.getByText("Image, RAW")).toBeInTheDocument();
        expect(screen.getByText("any")).toBeInTheDocument(); // opShort.in
    });

    it("renders a numeric pill (operator symbol + formatted number)", () => {
        act(() => dispatch({ type: "filter-replaced", filter: leaf("rating", "gte", 3) }));
        render(<FilterBar />);
        expect(screen.getByText("Rating")).toBeInTheDocument();
        expect(screen.getByText("≥")).toBeInTheDocument(); // opShort.gte
        expect(screen.getByText("3")).toBeInTheDocument();
    });

    it("renders a text pill with the raw value", () => {
        act(() => dispatch({ type: "filter-replaced", filter: leaf("filename", "contains", "beach") }));
        render(<FilterBar />);
        expect(screen.getByText("Filename")).toBeInTheDocument();
        expect(screen.getByText("has")).toBeInTheDocument(); // opShort.contains
        expect(screen.getByText("beach")).toBeInTheDocument();
    });

    it("drops the value segment for a valueless operator (empty)", () => {
        act(() => dispatch({ type: "filter-replaced", filter: leaf("rating", "empty", null) }));
        render(<FilterBar />);
        expect(screen.getByText("Rating")).toBeInTheDocument();
        expect(screen.getByText("empty")).toBeInTheDocument(); // opShort.empty, no value segment
    });

    it("clears the filter when a pill's remove button is pressed", async () => {
        const user = userEvent.setup();
        act(() => dispatch({ type: "filter-replaced", filter: leaf("fileType", "in", ["image"]) }));
        render(<FilterBar />);
        await user.click(screen.getByRole("button", { name: /remove filter/i }));
        expect(currentFilter()).toBeNull();
    });
});
