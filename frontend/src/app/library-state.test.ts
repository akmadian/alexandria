import { describe, expect, it } from "vitest";
import { deriveListQuery, initialState, libraryReducer, type LibraryState } from "./library-state";

const withSel = (ids: string[], last: string | null = ids[ids.length - 1] ?? null): LibraryState => ({
    ...initialState,
    selection: new Set(ids),
    lastSelectedId: last,
});

describe("libraryReducer selection", () => {
    it("plain select replaces the selection", () => {
        const s = libraryReducer(withSel(["a", "b"]), { type: "select", id: "c" });
        expect([...s.selection]).toEqual(["c"]);
        expect(s.lastSelectedId).toBe("c");
    });

    it("additive select toggles membership and tracks the anchor", () => {
        const added = libraryReducer(withSel(["a"]), { type: "select", id: "b", additive: true });
        expect([...added.selection].sort()).toEqual(["a", "b"]);
        expect(added.lastSelectedId).toBe("b");

        const removed = libraryReducer(added, { type: "select", id: "b", additive: true });
        expect([...removed.selection]).toEqual(["a"]);
    });

    it("range select unions the passed ids without dropping the anchor", () => {
        const s = libraryReducer(withSel(["a"]), { type: "select", id: "d", rangeIds: ["a", "b", "c", "d"] });
        expect([...s.selection].sort()).toEqual(["a", "b", "c", "d"]);
    });

    it("changing target clears selection and search but keeps sort/density", () => {
        const dirty: LibraryState = { ...withSel(["a", "b"]), filters: { ...initialState.filters, search: "beach", sort: "rating-desc" } };
        const s = libraryReducer(dirty, { type: "selectTarget", target: { kind: "collection", id: "col-1" } });
        expect(s.selection.size).toBe(0);
        expect(s.filters.search).toBe("");
        expect(s.filters.sort).toBe("rating-desc");
    });
});

describe("deriveListQuery", () => {
    it("maps a collection target to a scope, not a filter", () => {
        const q = deriveListQuery({ ...initialState, target: { kind: "collection", id: "col-1" } });
        expect(q.scope).toEqual({ kind: "collection", id: "col-1" });
        expect(q.filter?.tagIds).toBeUndefined();
    });

    it("maps source/tag targets to filter fields over the library scope", () => {
        const src = deriveListQuery({ ...initialState, target: { kind: "source", id: "src-1" } });
        expect(src.scope).toEqual({ kind: "library" });
        expect(src.filter?.sourceIds).toEqual(["src-1"]);

        const tag = deriveListQuery({ ...initialState, target: { kind: "tag", id: "t-1" } });
        expect(tag.filter?.tagIds).toEqual(["t-1"]);
    });

    it("folds filter-bar state into the predicate", () => {
        const q = deriveListQuery({
            ...initialState,
            filters: { ...initialState.filters, search: " beach ", fileType: "raw", minRating: 3 },
        });
        expect(q.filter?.searchText).toBe("beach");
        expect(q.filter?.fileTypes).toEqual(["raw"]);
        expect(q.filter?.ratingMin).toBe(3);
    });
});
