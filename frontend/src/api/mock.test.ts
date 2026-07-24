import { afterEach, describe, expect, it } from "vitest";
import type { Envelope, JobDone, JobProgress } from "@/_generated-types/models";
import { DEFAULT_ARRANGEMENT, type Arrangement, type Query } from "@/query-model/ast";
import { leaf } from "@/query-model/registry";
import type { FolderNode, Page } from "./contract";
import { configureMockImport, mockApi, resetMockBrowserRail } from "./mock";

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


describe("mock import lifecycle (the ticking job)", () => {
    // Zero-delay ticks so the job runs through in a handful of macrotasks; the
    // default watchable pace is restored after each case.
    afterEach(() => configureMockImport({ tickMs: 350 }));

    // Drive an import to its terminal event, collecting every envelope. Cancel is
    // requested from within the first progress tick, using the envelope's own
    // jobId (startImport's resolved id races the ticker).
    function runImport(volumeId: string, cancel = false): Promise<Envelope[]> {
        configureMockImport({ tickMs: 0 });
        return new Promise((resolve, reject) => {
            const seen: Envelope[] = [];
            let cancelPending = cancel;
            const unsubscribe = mockApi.subscribe((envelope) => {
                seen.push(envelope);
                if (cancelPending && envelope.type === "progress") {
                    cancelPending = false;
                    void mockApi.cancelJob((envelope.payload as JobProgress).jobId);
                }
                if (envelope.type === "done") {
                    unsubscribe();
                    resolve(seen);
                }
            });
            mockApi.startImport(volumeId).catch(reject);
        });
    }

    it("ticks an indeterminate scan, flips totalKnown, climbs to done, and carries the summary", async () => {
        const events = await runImport("src-0");
        const progress = events.filter((envelope) => envelope.type === "progress").map((envelope) => envelope.payload as JobProgress);
        const terminal = events.at(-1)?.payload as JobDone;

        // Every progress envelope rides the jobs topic and one jobId.
        expect(events.filter((envelope) => envelope.type === "progress").every((envelope) => envelope.topic === "jobs")).toBe(true);
        // The scan phase is indeterminate (totalKnown false), then the flip.
        expect(progress[0].totalKnown).toBe(false);
        expect(progress[0].stage).toBe("scan");
        expect(progress.some((frame) => frame.totalKnown)).toBe(true);
        expect(progress.at(-1)?.stage).toBe("write");
        // done climbs to the total by the last progress frame.
        expect(progress.at(-1)?.done).toBe(progress.at(-1)?.total);
        // The terminal done carries the four-count summary that sums to the total.
        expect(terminal.state).toBe("done");
        const { added, updated, skipped, errors } = terminal.summary ?? { added: 0, updated: 0, skipped: 0, errors: 0 };
        expect(added + updated + skipped + errors).toBe(progress.at(-1)?.total);
    });

    it("rejects an unknown source with not_found", async () => {
        await expect(mockApi.startImport("no-such-source")).rejects.toMatchObject({
            name: "ApiError",
            kind: "domain",
            code: "not_found",
        });
    });

    it("cancels mid-run, producing a cancelled terminal with the partial tally", async () => {
        const events = await runImport("src-1", true);
        const terminal = events.at(-1)?.payload as JobDone;
        expect(terminal.state).toBe("cancelled");
        expect(terminal.summary?.added).toBeGreaterThanOrEqual(0);
        // Cancelled short-circuits before the full write climb finishes.
        const progress = events.filter((envelope) => envelope.type === "progress");
        expect(progress.length).toBeLessThan(7);
    });

    it("cancelJob is a no-op for an unknown job (never rejects)", async () => {
        await expect(mockApi.cancelJob("mock-job-9999")).resolves.toBeUndefined();
    });
});

// --- the browser rail (D41) --------------------------------------------------

// Walk every node, asserting a parent's count is exactly the sum of its
// children's (intermediate nodes hold no direct assets in the mock, so the
// subtree count sums at every level — the folder-count invariant).
function assertSubtreeSums(node: FolderNode): void {
    if (node.children.length === 0) return;
    const childSum = node.children.reduce((sum, child) => sum + child.assetCount, 0);
    expect(node.assetCount).toBe(childSum);
    node.children.forEach(assertSubtreeSums);
}

describe("mock folder tree (getFolderTree)", () => {
    it("returns the volume forest with subtree counts that sum at every level", async () => {
        const volumes = await mockApi.getFolderTree();
        expect(volumes.length).toBeGreaterThanOrEqual(3);
        for (const volume of volumes) {
            const rootSum = volume.folders.reduce((sum, folder) => sum + folder.assetCount, 0);
            expect(volume.assetCount).toBe(rootSum);
            volume.folders.forEach(assertSubtreeSums);
        }
        // Every seeded asset maps to a seeded volume (src-0..2), so the forest's
        // total equals the whole catalog — no asset is dropped or double-counted.
        const total = volumes.reduce((sum, volume) => sum + volume.assetCount, 0);
        const { total: catalogTotal } = await mockApi.queryAssets(LIBRARY, DEFAULT_ARRANGEMENT, ALL);
        expect(total).toBe(catalogTotal);
    });

    it("keeps an offline volume present and browsable (never dropped or disabled)", async () => {
        const volumes = await mockApi.getFolderTree();
        const offline = volumes.filter((volume) => volume.connectivity === "offline");
        expect(offline).toHaveLength(1);
        expect(offline[0].folders.length).toBeGreaterThan(0); // still has its tree
    });

    it("carries syncMode on tracked roots only, never on derived path nodes", async () => {
        const volumes = await mockApi.getFolderTree();
        let derivedSeen = 0;
        const check = (node: FolderNode): void => {
            if (node.id.includes(":")) {
                // Synthetic id (volumeId + ":" + path) = a derived node: no syncMode.
                derivedSeen += 1;
                expect(node.syncMode).toBeUndefined();
            } else {
                expect(node.syncMode).toBeDefined(); // a tracked root
            }
            node.children.forEach(check);
        };
        volumes.forEach((volume) => volume.folders.forEach(check));
        expect(derivedSeen).toBeGreaterThan(0); // the tree actually has derived nodes
    });
});

describe("mock collections (listCollections + scope narrowing)", () => {
    it("computes manual counts from membership and smart counts through the query engine", async () => {
        const collections = await mockApi.listCollections();
        const byId = new Map(collections.map((collection) => [collection.id, collection]));

        // Manual: count is the membership size.
        expect(byId.get("col-select")?.assetCount).toBe(4);
        expect(byId.get("col-select")?.kind).toBe("manual");

        // Smart: the badge equals what the same predicate yields through the query
        // engine (scope and badge can't disagree).
        const highRated = await mockApi.queryAssets(withWhere(leaf("rating", "gte", 4)), DEFAULT_ARRANGEMENT, ALL);
        expect(byId.get("col-highrated")?.assetCount).toBe(highRated.total);
    });

    it("distinguishes empty (0) from count-unavailable (null), and carries parentId", async () => {
        const collections = await mockApi.listCollections();
        const byId = new Map(collections.map((collection) => [collection.id, collection]));

        expect(byId.get("col-cull")?.assetCount).toBe(0); // genuinely empty
        expect(byId.get("col-cull")?.parentId).toBe("col-select"); // nesting via adjacency
        expect(byId.get("col-portfolio")?.assetCount).toBeNull(); // declined, not empty
        expect(byId.get("col-select")?.parentId).toBeUndefined(); // top-level
    });

    it("narrows a manual collection scope to exactly its members and nothing else", async () => {
        const scope: Query = { version: 1, scope: { kind: "collection", id: "col-select" }, where: null };
        const { items, total } = await mockApi.queryAssets(scope, DEFAULT_ARRANGEMENT, ALL);
        const members = ["mock-0000", "mock-0003", "mock-0006", "mock-0012"];
        expect(total).toBe(members.length);
        expect(items.map((row) => row.id).sort()).toEqual([...members].sort());
    });

    it("narrows a smart collection scope by re-running its predicate", async () => {
        const scope: Query = { version: 1, scope: { kind: "collection", id: "col-highrated" }, where: null };
        const { items, total } = await mockApi.queryAssets(scope, DEFAULT_ARRANGEMENT, ALL);
        const direct = await mockApi.queryAssets(withWhere(leaf("rating", "gte", 4)), DEFAULT_ARRANGEMENT, ALL);
        expect(total).toBe(direct.total);
        expect(items.every((row) => row.rating !== null && row.rating >= 4)).toBe(true);
    });

    it("yields nothing for an unknown collection id", async () => {
        const scope: Query = { version: 1, scope: { kind: "collection", id: "no-such" }, where: null };
        expect((await mockApi.queryAssets(scope, DEFAULT_ARRANGEMENT, ALL)).total).toBe(0);
    });
});

describe("mock createFolder outcomes (the D41 table)", () => {
    afterEach(() => resetMockBrowserRail());

    it("created — a disjoint path mints a new tracked root and a volume for it", async () => {
        const outcome = await mockApi.createFolder("/Volumes/NewDrive/Fresh");
        expect(outcome.kind).toBe("created");
        expect(outcome.folderId).toBeDefined();
        const volumes = await mockApi.getFolderTree();
        expect(volumes.some((volume) => volume.folders.some((folder) => folder.path === "/Volumes/NewDrive/Fresh"))).toBe(true);
    });

    it("already_tracked_within — a subfolder of a tracked root redirects to it", async () => {
        const outcome = await mockApi.createFolder("/Volumes/StudioSSD/Photos/2026");
        expect(outcome.kind).toBe("already_tracked_within");
        expect(outcome.folderId).toBe("folder-studio");
    });

    it("already_tracked_within — an exact duplicate selects the existing root", async () => {
        const outcome = await mockApi.createFolder("/Volumes/FieldDrive/2024");
        expect(outcome.kind).toBe("already_tracked_within");
        expect(outcome.folderId).toBe("folder-field-2024");
    });

    it("absorbed — a parent of two like-moded roots merges quietly (no confirmation)", async () => {
        const outcome = await mockApi.createFolder("/Volumes/FieldDrive");
        expect(outcome.kind).toBe("absorbed");
        expect(outcome.absorbedFolderIds).toEqual(expect.arrayContaining(["folder-field-2024", "folder-field-2025"]));
        expect(outcome.behaviorChanges ?? []).toHaveLength(0);
        // The two roots now nest under the new parent, counts still summing.
        const field = (await mockApi.getFolderTree()).find((volume) => volume.id === "vol-field");
        expect(field?.folders).toHaveLength(1);
        expect(field?.folders[0].path).toBe("/Volumes/FieldDrive");
        expect(field?.folders[0].children.map((child) => child.path)).toEqual(
            expect.arrayContaining(["/Volumes/FieldDrive/2024", "/Volumes/FieldDrive/2025"]),
        );
    });

    it("needs_confirmation — absorbing a watched root asks first and does not mutate", async () => {
        const outcome = await mockApi.createFolder("/Volumes/StudioSSD");
        expect(outcome.kind).toBe("needs_confirmation");
        expect(outcome.absorbedFolderIds).toContain("folder-studio");
        expect(outcome.behaviorChanges).toEqual([
            { folderId: "folder-studio", folderName: "Photos", currentSyncMode: "watched", newSyncMode: "manual" },
        ]);
        // No mutation happened: the studio volume's top root is still Photos.
        const studio = (await mockApi.getFolderTree()).find((volume) => volume.id === "vol-studio");
        expect(studio?.folders[0].path).toBe("/Volumes/StudioSSD/Photos");
    });

    it("needs_confirmation — a scheduled root triggers the same ask as a watched one", async () => {
        const outcome = await mockApi.createFolder("/Volumes/ArchiveNAS");
        expect(outcome.kind).toBe("needs_confirmation");
        expect(outcome.behaviorChanges).toEqual([
            { folderId: "folder-archive", folderName: "Archive", currentSyncMode: "scheduled", newSyncMode: "manual" },
        ]);
    });

    it("needs_confirmation → the confirmed re-call proceeds to absorb", async () => {
        const first = await mockApi.createFolder("/Volumes/StudioSSD");
        expect(first.kind).toBe("needs_confirmation");
        const confirmed = await mockApi.createFolder("/Volumes/StudioSSD", true);
        expect(confirmed.kind).toBe("absorbed");
        expect(confirmed.absorbedFolderIds).toContain("folder-studio");
        const studio = (await mockApi.getFolderTree()).find((volume) => volume.id === "vol-studio");
        expect(studio?.folders[0].path).toBe("/Volumes/StudioSSD"); // the new parent
    });
});

describe("mock folder mutations (remove / update / pick)", () => {
    afterEach(() => resetMockBrowserRail());

    it("removeFolder drops the root from the tree and rejects an unknown id", async () => {
        await mockApi.removeFolder("folder-field-2024");
        const field = (await mockApi.getFolderTree()).find((volume) => volume.id === "vol-field");
        expect(field?.folders.some((folder) => folder.path === "/Volumes/FieldDrive/2024")).toBe(false);
        await expect(mockApi.removeFolder("nope")).rejects.toMatchObject({ name: "ApiError", code: "not_found" });
    });

    it("updateFolder patches sync mode and name, and rejects an unknown id", async () => {
        await mockApi.updateFolder("folder-studio", { syncMode: "manual", name: "Renamed" });
        const studio = (await mockApi.getFolderTree()).find((volume) => volume.id === "vol-studio");
        expect(studio?.folders[0].syncMode).toBe("manual");
        expect(studio?.folders[0].name).toBe("Renamed");
        await expect(mockApi.updateFolder("nope", { syncMode: "manual" })).rejects.toMatchObject({
            name: "ApiError",
            code: "not_found",
        });
    });

    it("pickDirectory resolves with a fake chosen path", async () => {
        await expect(mockApi.pickDirectory()).resolves.toBe("/Volumes/Untitled/New Folder");
    });
});
