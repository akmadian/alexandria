import { describe, expect, it } from "vitest";
import { addLeaf, removeLeaf, replaceLeaf, topLevelLeaves } from "./assemble";
import type { WhereNode } from "./ast";
import { leaf } from "./registry";

const a = leaf("fileType", "in", ["image"]);
const b = leaf("rating", "gte", 3);
const c = leaf("flag", "in", ["pick"]);
const and = (...children: WhereNode[]): WhereNode => ({ op: "and", children });

describe("topLevelLeaves", () => {
    it("is empty for a null filter", () => {
        expect(topLevelLeaves(null)).toEqual([]);
    });
    it("wraps a single leaf", () => {
        expect(topLevelLeaves(a)).toEqual([a]);
    });
    it("flattens a top-level AND", () => {
        expect(topLevelLeaves(and(a, b))).toEqual([a, b]);
    });
    it("has no flat form for a top-level OR (group-pill territory)", () => {
        expect(topLevelLeaves({ op: "or", children: [a, b] })).toEqual([]);
    });
});

describe("addLeaf", () => {
    it("null → the leaf itself", () => {
        expect(addLeaf(null, a)).toEqual(a);
    });
    it("leaf → a two-child AND", () => {
        expect(addLeaf(a, b)).toEqual(and(a, b));
    });
    it("AND → appended child", () => {
        expect(addLeaf(and(a, b), c)).toEqual(and(a, b, c));
    });
});

describe("removeLeaf", () => {
    it("collapses a two-child AND back to a single leaf", () => {
        expect(removeLeaf(and(a, b), 0)).toEqual(b);
    });
    it("removing the last leaf yields null", () => {
        expect(removeLeaf(a, 0)).toBeNull();
    });
    it("keeps an AND when three become two", () => {
        expect(removeLeaf(and(a, b, c), 1)).toEqual(and(a, c));
    });
});

describe("replaceLeaf", () => {
    it("swaps the leaf at the index and leaves the rest", () => {
        const edited = leaf("fileType", "notIn", ["image"]);
        expect(replaceLeaf(and(a, b), 0, edited)).toEqual(and(edited, b));
    });
    it("replaces the sole leaf in place", () => {
        expect(replaceLeaf(a, 0, b)).toEqual(b);
    });
});
