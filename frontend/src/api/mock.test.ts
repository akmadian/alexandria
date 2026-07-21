import { describe, expect, it } from "vitest";
import { DEFAULT_ARRANGEMENT, type Arrangement, type Query } from "@/query-model/ast";
import { leaf } from "@/query-model/registry";
import type { Page } from "./contract";
import { mockApi } from "./mock";

const LIBRARY: Query = { version: 1, scope: { kind: "library" }, where: null };
const ALL: Page = { offset: 0, limit: 1000 };
const withWhere = (where: Query["where"]): Query => ({ ...LIBRARY, where });

describe("mock query engine", () => {
    it("returns the whole catalog with a null predicate", async () => {
        const { items, total } = await mockApi.queryAssets(LIBRARY, DEFAULT_ARRANGEMENT, ALL);
        expect(total).toBe(64);
        expect(items).toHaveLength(64);
        expect(items[0].kind).toBe("asset");
    });

    it("filters on a numeric predicate (rating gte 3)", async () => {
        const { items, total } = await mockApi.queryAssets(withWhere(leaf("rating", "gte", 3)), DEFAULT_ARRANGEMENT, ALL);
        expect(total).toBe(24);
        expect(items.every((row) => row.rating !== null && row.rating >= 3)).toBe(true);
    });

    it("filters on an enum membership predicate (fileType in [video])", async () => {
        const { items, total } = await mockApi.queryAssets(
            withWhere(leaf("fileType", "in", ["video"])),
            DEFAULT_ARRANGEMENT,
            ALL,
        );
        expect(total).toBe(13);
        expect(items.every((row) => row.fileType === "video")).toBe(true);
    });

    it("filters on a text predicate (filename contains, case-insensitive)", async () => {
        const { total } = await mockApi.queryAssets(withWhere(leaf("filename", "contains", "dsc")), DEFAULT_ARRANGEMENT, ALL);
        expect(total).toBe(51); // the non-video seed
    });

    it("combines predicates through boolean groups (and)", async () => {
        const where = { op: "and" as const, children: [leaf("fileType", "in", ["image"]), leaf("rating", "gte", 4)] };
        const { items } = await mockApi.queryAssets(withWhere(where), DEFAULT_ARRANGEMENT, ALL);
        expect(items.every((row) => row.fileType === "image" && row.rating !== null && row.rating >= 4)).toBe(true);
    });

    // Null ratings sort smallest, matching SQLite's NULL ordering.
    const ratingKey = (rating: number | null): number => rating ?? -Infinity;

    it("orders deterministically with an id tiebreaker", async () => {
        const arrangement: Arrangement = { sortField: "rating", sortDir: "asc", groupBy: null };
        const { items } = await mockApi.queryAssets(LIBRARY, arrangement, ALL);
        for (let i = 1; i < items.length; i++) {
            expect(ratingKey(items[i - 1].rating)).toBeLessThanOrEqual(ratingKey(items[i].rating));
            if (items[i - 1].rating === items[i].rating) {
                expect(items[i - 1].id < items[i].id).toBe(true); // stable within equal keys
            }
        }
    });

    it("pages a window and reports the full total", async () => {
        const first = await mockApi.queryAssets(LIBRARY, DEFAULT_ARRANGEMENT, { offset: 0, limit: 10 });
        const tail = await mockApi.queryAssets(LIBRARY, DEFAULT_ARRANGEMENT, { offset: 60, limit: 10 });
        expect(first.items).toHaveLength(10);
        expect(first.total).toBe(64);
        expect(tail.items).toHaveLength(4);
        expect(first.items[0].id).not.toBe(tail.items[0].id);
    });

    it("locates an asset's index in the ordered result", async () => {
        const { items } = await mockApi.queryAssets(LIBRARY, DEFAULT_ARRANGEMENT, ALL);
        expect(await mockApi.indexOfAsset(LIBRARY, DEFAULT_ARRANGEMENT, items[5].id)).toBe(5);
        expect(await mockApi.indexOfAsset(LIBRARY, DEFAULT_ARRANGEMENT, "nope")).toBeNull();
    });

    it("keeps the id tiebreaker ASCENDING even when the sort is descending", async () => {
        const arrangement: Arrangement = { sortField: "rating", sortDir: "desc", groupBy: null };
        const { items } = await mockApi.queryAssets(LIBRARY, arrangement, ALL);
        for (let i = 1; i < items.length; i++) {
            expect(ratingKey(items[i - 1].rating)).toBeGreaterThanOrEqual(ratingKey(items[i].rating)); // primary desc
            if (items[i - 1].rating === items[i].rating) {
                expect(items[i - 1].id < items[i].id).toBe(true); // ties still id-ascending
            }
        }
    });

    it("evaluates OR groups", async () => {
        const where = { op: "or" as const, children: [leaf("fileType", "in", ["vector"]), leaf("rating", "gte", 5)] };
        const { items } = await mockApi.queryAssets(withWhere(where), DEFAULT_ARRANGEMENT, ALL);
        expect(items.length).toBeGreaterThan(0);
        expect(items.every((row) => row.fileType === "vector" || (row.rating !== null && row.rating >= 5))).toBe(true);
    });

    it("evaluates NOT groups", async () => {
        const where = { op: "not" as const, children: [leaf("fileType", "in", ["video"])] };
        const { items, total } = await mockApi.queryAssets(withWhere(where), DEFAULT_ARRANGEMENT, ALL);
        expect(total).toBe(51); // everything but the 13 videos
        expect(items.every((row) => row.fileType !== "video")).toBe(true);
    });

    it("filters on a half-open date interval (within)", async () => {
        // Wide margins around the seed's 2025 captures — tz-robust (the seed builds
        // dates in local time; the anchor parses as UTC). 7 seeds are undated
        // scans (null capturedAt) and never match a positive `within`.
        const contains2025 = leaf("capturedAt", "within", { anchor: "2024-06-01", duration: "P2Y" });
        const before = leaf("capturedAt", "within", { anchor: "2000-01-01", duration: "P1Y" });
        expect((await mockApi.queryAssets(withWhere(contains2025), DEFAULT_ARRANGEMENT, ALL)).total).toBe(57);
        expect((await mockApi.queryAssets(withWhere(before), DEFAULT_ARRANGEMENT, ALL)).total).toBe(0);
    });

    it("negation includes absent (D24 NULL policy): notWithin and notIn match null values", async () => {
        // notWithin an interval that contains every dated capture → only the 7
        // undated scans remain, BECAUSE absence counts as "not within".
        const allDated = leaf("capturedAt", "notWithin", { anchor: "2024-06-01", duration: "P2Y" });
        const undated = await mockApi.queryAssets(withWhere(allDated), DEFAULT_ARRANGEMENT, ALL);
        expect(undated.total).toBe(7);
        expect(undated.items.every((row) => row.capturedAt === null)).toBe(true);

        // notIn includes unlabeled assets alongside the other-colored ones.
        const notRed = await mockApi.queryAssets(withWhere(leaf("colorLabel", "notIn", ["red"])), DEFAULT_ARRANGEMENT, ALL);
        expect(notRed.items.every((row) => row.colorLabel !== "red")).toBe(true);
        expect(notRed.items.some((row) => row.colorLabel === null)).toBe(true);
    });

    it("parses negative ISO durations (backward-looking ranges)", async () => {
        // [now-30y, now) spans the whole 2025 seed range; only dated assets match.
        const lastDecades = leaf("capturedAt", "within", { anchor: "now", duration: "-P30Y" });
        expect((await mockApi.queryAssets(withWhere(lastDecades), DEFAULT_ARRANGEMENT, ALL)).total).toBe(57);
    });

    it("sorts by size", async () => {
        const arrangement: Arrangement = { sortField: "size", sortDir: "desc", groupBy: null };
        const { items } = await mockApi.queryAssets(LIBRARY, arrangement, ALL);
        for (let i = 1; i < items.length; i++) {
            expect(items[i - 1].sizeBytes).toBeGreaterThanOrEqual(items[i].sizeBytes);
        }
    });

    it("treats absence via empty / notEmpty (null = unrated / missing metadata)", async () => {
        expect((await mockApi.queryAssets(withWhere(leaf("rating", "empty", null)), DEFAULT_ARRANGEMENT, ALL)).total).toBe(24);
        const withCamera = await mockApi.queryAssets(withWhere(leaf("cameraModel", "notEmpty", null)), DEFAULT_ARRANGEMENT, ALL);
        expect(withCamera.total).toBe(54);
        expect(withCamera.items.every((row) => row.cameraModel !== null)).toBe(true);
    });

    it("returns an ids-only window matching the ordered result (assetIdSlice)", async () => {
        const { items } = await mockApi.queryAssets(LIBRARY, DEFAULT_ARRANGEMENT, ALL);
        const slice = await mockApi.assetIdSlice(LIBRARY, DEFAULT_ARRANGEMENT, 0, 4);
        expect(slice).toEqual(items.slice(0, 5).map((row) => row.id)); // inclusive [0,4]
    });

    it("serves the full detail projection by id (getAsset)", async () => {
        // mock-0000 is a pinned seed fact (i = 0): dated, Sony camera, exposure
        // set, extended blob present, folder segment on the path.
        const detail = await mockApi.getAsset("mock-0000");
        expect(detail.filename).toBe("DSC_04820.jpg");
        expect(detail.relativePath).toBe("2026/DSC_04820.jpg");
        expect(detail.mimeType).toBe("image/jpeg");
        expect(detail.aperture).toBe(1.8);
        expect(detail.shutterSpeed).toBe("1/1000");
        expect(detail.extendedMetadata?.["EXIF:Flash"]).toBe("Did not fire");
    });

    it("rejects an unknown id with a not_found ApiError (getAsset)", async () => {
        await expect(mockApi.getAsset("nope")).rejects.toMatchObject({
            name: "ApiError",
            kind: "domain",
            code: "not_found",
        });
    });
});

describe("mock updateAssets (the write path)", () => {
    // The mock mutates its shared seed in place (the SQL UPDATE stand-in), so each
    // case snapshots the subject's judgment and restores it, keeping the suite
    // order-independent.
    async function withRestored(id: string, body: () => Promise<void>): Promise<void> {
        const before = await mockApi.getAsset(id);
        try {
            await body();
        } finally {
            await mockApi.updateAssets(
                { ids: [id] },
                { rating: before.rating, colorLabel: before.colorLabel, flag: before.flag, note: before.note },
            );
        }
    }

    it("sets a value field (rating) and leaves absent fields untouched", async () => {
        await withRestored("mock-0002", async () => {
            const before = await mockApi.getAsset("mock-0002");
            await mockApi.updateAssets({ ids: ["mock-0002"] }, { rating: 5 });
            const after = await mockApi.getAsset("mock-0002");
            expect(after.rating).toBe(5);
            expect(after.colorLabel).toBe(before.colorLabel); // absent key untouched
            expect(after.flag).toBe(before.flag);
        });
    });

    it("clears a field with an explicit null (three-state: null = clear)", async () => {
        await withRestored("mock-0003", async () => {
            expect((await mockApi.getAsset("mock-0003")).rating).toBe(5); // pinned seed
            await mockApi.updateAssets({ ids: ["mock-0003"] }, { rating: null });
            expect((await mockApi.getAsset("mock-0003")).rating).toBeNull();
        });
    });

    it("applies label and flag together, and a note", async () => {
        await withRestored("mock-0006", async () => {
            await mockApi.updateAssets({ ids: ["mock-0006"] }, { colorLabel: "green", flag: "pick", note: "keep" });
            const after = await mockApi.getAsset("mock-0006");
            expect(after.colorLabel).toBe("green");
            expect(after.flag).toBe("pick");
            expect(after.note).toBe("keep");
        });
    });

    it("applies to every listed id and is visible in the query result too", async () => {
        await withRestored("mock-0000", async () => {
            await withRestored("mock-0001", async () => {
                await mockApi.updateAssets({ ids: ["mock-0000", "mock-0001"] }, { rating: 4 });
                const { items } = await mockApi.queryAssets(LIBRARY, DEFAULT_ARRANGEMENT, ALL);
                const byId = new Map(items.map((row) => [row.id, row]));
                expect(byId.get("mock-0000")?.rating).toBe(4);
                expect(byId.get("mock-0001")?.rating).toBe(4);
            });
        });
    });

    it("rejects with not_found when any id is unknown, without half-applying", async () => {
        const before = await mockApi.getAsset("mock-0004");
        await expect(mockApi.updateAssets({ ids: ["mock-0004", "nope"] }, { rating: 1 })).rejects.toMatchObject({
            name: "ApiError",
            kind: "domain",
            code: "not_found",
        });
        expect((await mockApi.getAsset("mock-0004")).rating).toBe(before.rating); // untouched
    });

    it("rejects an empty ids target with validation, like the real seam's target switch", async () => {
        await expect(mockApi.updateAssets({ ids: [] }, { rating: 1 })).rejects.toMatchObject({
            name: "ApiError",
            kind: "domain",
            code: "validation",
        });
    });

    it("treats an explicitly-undefined key as don't-touch, never a clear (JSON drops it at the seam)", async () => {
        await withRestored("mock-0003", async () => {
            expect((await mockApi.getAsset("mock-0003")).rating).toBe(5); // pinned seed
            await mockApi.updateAssets({ ids: ["mock-0003"] }, { rating: undefined, flag: "pick" });
            const after = await mockApi.getAsset("mock-0003");
            expect(after.rating).toBe(5); // undefined ≠ null: not cleared
            expect(after.flag).toBe("pick");
        });
    });
});
