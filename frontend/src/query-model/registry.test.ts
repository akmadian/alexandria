import { describe, expect, it } from "vitest";
import { DEFAULT_ARRANGEMENT, type Query } from "./ast";
import { leaf, tokens, validate } from "./registry";
import { serializeQuery } from "./serialize";

describe("token registry", () => {
    it("seeds every generated field", () => {
        // Completeness is inherited from the generated fieldGrammar; spot-check it materialized.
        expect(tokens.rating.kind).toBe("numeric");
        expect(tokens.fileType.kind).toBe("enum");
        expect(tokens.filename.operators).toContain("contains");
    });
});

describe("validate", () => {
    it("accepts well-formed leaves", () => {
        expect(validate(leaf("rating", "gte", 3))).toBe(true);
        expect(validate(leaf("fileType", "in", ["raw", "image"]))).toBe(true);
        expect(validate(leaf("filename", "contains", "DSC"))).toBe(true);
        expect(validate(leaf("rating", "empty", undefined))).toBe(true); // absence
    });

    it("rejects an operator the field does not allow", () => {
        expect(validate(leaf("fileType", "gte", 3))).toBe(false); // enum has no gte
    });

    it("rejects a value of the wrong kind", () => {
        expect(validate(leaf("rating", "gte", "three"))).toBe(false);
        expect(validate(leaf("fileType", "in", []))).toBe(false); // empty membership set
        expect(validate(leaf("filename", "contains", ""))).toBe(false);
    });

    it("rejects an unknown field", () => {
        expect(validate({ field: "bogus" as never, cmp: "eq", value: 1 })).toBe(false);
    });
});

describe("serializeQuery", () => {
    const base: Query = { version: 1, scope: { kind: "library" }, where: null };

    it("is stable regardless of object key order", () => {
        const a: Query = { version: 1, scope: { kind: "collection", id: "c1" }, where: null };
        const b: Query = { scope: { id: "c1", kind: "collection" }, version: 1, where: null };
        expect(serializeQuery(a, DEFAULT_ARRANGEMENT)).toBe(serializeQuery(b, DEFAULT_ARRANGEMENT));
    });

    it("distinguishes different queries and arrangements", () => {
        expect(serializeQuery(base, DEFAULT_ARRANGEMENT)).not.toBe(
            serializeQuery({ ...base, scope: { kind: "collection", id: "c1" } }, DEFAULT_ARRANGEMENT),
        );
        expect(serializeQuery(base, DEFAULT_ARRANGEMENT)).not.toBe(
            serializeQuery(base, { ...DEFAULT_ARRANGEMENT, sortDir: "asc" }),
        );
    });
});
