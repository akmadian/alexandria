import { describe, expect, it } from "vitest";
import type { WhereNode } from "@/query-model/ast";
import { leaf } from "@/query-model/registry";
import { reduce, type Selection, selectionHas, selectionSize } from "./catalog-store";

const ids = (...values: string[]): Selection => ({ kind: "ids", ids: new Set(values) });
const all = (...except: string[]): Selection => ({ kind: "all", except: new Set(except) });
const state = (selection: Selection = ids(), cursorId: string | null = null, filter: WhereNode | null = null) => ({
    selection,
    cursorId,
    filter,
});
const idList = (selection: Selection | undefined) => (selection?.kind === "ids" ? [...selection.ids] : null);

describe("reduce", () => {
    it("plain click replaces the selection and moves the cursor", () => {
        const next = reduce(state(ids("x", "y")), { type: "asset-clicked", id: "a", additive: false });
        expect(idList(next.selection)).toEqual(["a"]);
        expect(next.cursorId).toBe("a");
    });

    it("cmd-click toggles within an ids selection", () => {
        const added = reduce(state(ids("a")), { type: "asset-clicked", id: "b", additive: true });
        expect(idList(added.selection)?.sort()).toEqual(["a", "b"]);
        const removed = reduce(state(ids("a", "b")), { type: "asset-clicked", id: "b", additive: true });
        expect(idList(removed.selection)).toEqual(["a"]);
    });

    it("cmd-click on an all-selection toggles the except set (never enumerates)", () => {
        const next = reduce(state(all()), { type: "asset-clicked", id: "a", additive: true });
        expect(next.selection).toEqual({ kind: "all", except: new Set(["a"]) });
    });

    it("range-committed sets ids and anchors the cursor at the range end", () => {
        const next = reduce(state(), { type: "range-committed", ids: ["a", "b", "c"] });
        expect(idList(next.selection)).toEqual(["a", "b", "c"]);
        expect(next.cursorId).toBe("c");
    });

    it("cursor-set with select replaces selection; without select moves only the cursor", () => {
        const selecting = reduce(state(), { type: "cursor-set", id: "a", select: true });
        expect(idList(selecting.selection)).toEqual(["a"]);
        expect(selecting.cursorId).toBe("a");

        const moving = reduce(state(ids("x"), "old"), { type: "cursor-set", id: "a", select: false });
        expect(moving.cursorId).toBe("a");
        expect(moving.selection).toBeUndefined(); // selection untouched
    });

    it("select-all flips to the all-kind with an empty except set", () => {
        const next = reduce(state(ids("a")), { type: "select-all" });
        expect(next.selection).toEqual({ kind: "all", except: new Set() });
    });

    it("selection-cleared empties the selection", () => {
        const next = reduce(state(all("a")), { type: "selection-cleared" });
        expect(next.selection).toEqual({ kind: "ids", ids: new Set() });
    });

    it("filter-replaced sets the filter and resets the ephemeral tiers", () => {
        const where = leaf("fileType", "in", ["image"]);
        const next = reduce(state(ids("a", "b"), "a"), { type: "filter-replaced", filter: where });
        expect(next.filter).toBe(where);
        expect(next.selection).toEqual({ kind: "ids", ids: new Set() });
        expect(next.cursorId).toBeNull();
    });
});

describe("reduce — working-set-changed (cursor invariant)", () => {
    it("seeds the cursor to the first row when none is held", () => {
        const next = reduce(state(ids(), null), { type: "working-set-changed", total: 64, firstId: "first" });
        expect(next.cursorId).toBe("first");
    });
    it("clears the cursor when the working set empties", () => {
        const next = reduce(state(ids(), "old"), { type: "working-set-changed", total: 0, firstId: null });
        expect(next.cursorId).toBeNull();
    });
    it("leaves an existing cursor in place (no jump on refetch)", () => {
        const next = reduce(state(ids(), "held"), { type: "working-set-changed", total: 64, firstId: "first" });
        expect(next.cursorId).toBeUndefined(); // no change emitted
    });
});

describe("selectionHas", () => {
    it("reads membership for the ids kind", () => {
        expect(selectionHas(ids("a"), "a")).toBe(true);
        expect(selectionHas(ids("a"), "b")).toBe(false);
    });
    it("inverts the except set for the all kind", () => {
        expect(selectionHas(all("a"), "a")).toBe(false);
        expect(selectionHas(all("a"), "b")).toBe(true);
    });
});

describe("selectionSize", () => {
    it("counts ids directly", () => {
        expect(selectionSize(ids("a", "b"), 10)).toBe(2);
    });
    it("sizes an all-selection against the working-set total", () => {
        expect(selectionSize(all("a"), 10)).toBe(9);
        expect(selectionSize(all(), 10)).toBe(10);
    });
});
