// Runnable self-check for the mock API — no test framework.
//   node src/lib/mock-api.check.ts      (Node 23+ strips the types)
// Exits non-zero on the first failed assertion.

import assert from "node:assert";
import { assets as seed } from "./mock.ts";
import { buildFolderTree, createMockApi, dirOf, inScope, matchesFilter, sortAssets } from "./mock-api.ts";

// --- dirOf ---
assert.equal(dirOf("2026/06/Dolomites/a.arw"), "2026/06/Dolomites");
assert.equal(dirOf("a.jpg"), "");

// --- scope: folder recursion ---
const a = seed.find((x) => x.sourceId === "src-main")!;
const dir = dirOf(a.relativePath);
assert.ok(inScope({ kind: "folder", sourceId: "src-main", path: dir }, a, () => false), "recursive folder includes own dir");
assert.ok(!inScope({ kind: "folder", sourceId: "src-photos", path: dir }, a, () => false), "wrong source excluded");
assert.ok(inScope({ kind: "library" }, a, () => false), "library includes all");

// --- filter: absence + predicate ---
assert.ok(matchesFilter({ ...a, rating: 0 }, { unrated: true }));
assert.ok(!matchesFilter({ ...a, rating: 3 }, { unrated: true }));
assert.ok(matchesFilter({ ...a, fileType: "raw" }, { fileTypes: ["raw"] }));
assert.ok(!matchesFilter({ ...a, fileType: "image" }, { fileTypes: ["raw"] }));

// --- sort: deterministic id tie-break ---
const tie = [
    { ...a, id: "z", capturedAt: "2026-01-01T00:00:00Z" },
    { ...a, id: "a", capturedAt: "2026-01-01T00:00:00Z" },
];
const asc = sortAssets(tie, { field: "captured", dir: "asc" });
assert.deepEqual(asc.map((x) => x.id), ["a", "z"], "equal keys break ties by id");

// --- folder tree: totals roll up, direct counts sum to asset count ---
const mainAssets = seed.filter((x) => x.sourceId === "src-main");
const tree = buildFolderTree(mainAssets);
assert.equal(tree.totalCount, mainAssets.length, "root total == all source assets");
const sumDirect = (n: { directCount: number; children: any[] }): number =>
    n.directCount + n.children.reduce((s, c) => s + sumDirect(c), 0);
assert.equal(sumDirect(tree), mainAssets.length, "direct counts sum to total");

// --- stateful: list projection, patch + undo round-trip, event fires ---
const api = createMockApi();

const listed = await api.listAssets({ scope: { kind: "library" } });
assert.ok(listed.total > 0 && listed.items.length === listed.total);
assert.equal(listed.items[0].kind, "asset");
assert.ok("thumbURL" in listed.items[0] && !("cameraModel" in listed.items[0]), "row is the slim projection");

const target = { ids: [seed[0].id] };
let caught: string[] | undefined;
const off = api.onCatalogChanged((c) => (caught = c.ids));
await api.patchAssets(target, { rating: 1 });
assert.deepEqual(caught, [seed[0].id], "catalog:changed carries ids");
assert.equal((await api.getAsset(seed[0].id))!.rating, 1, "patch applied");
await api.undo();
assert.equal((await api.getAsset(seed[0].id))!.rating, seed[0].rating, "undo restores prior value");
off();

// --- soft delete hides from list and getAsset; undo brings it back ---
const before = (await api.listAssets({ scope: { kind: "library" } })).total;
await api.removeFromCatalog({ ids: [seed[1].id] });
assert.equal((await api.listAssets({ scope: { kind: "library" } })).total, before - 1, "soft-deleted excluded");
assert.equal(await api.getAsset(seed[1].id), null, "getAsset hides soft-deleted");
await api.undo();
assert.equal((await api.listAssets({ scope: { kind: "library" } })).total, before, "undo un-deletes");

// --- keybinding conflict is a typed domain error ---
await assert.rejects(() => api.setKeybinding("rate_2", "5", "grid"), (e: any) => e.code === "keybinding_conflict");

console.log("mock-api.check: all assertions passed");
