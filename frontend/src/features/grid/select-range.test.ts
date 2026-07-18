// Range materialization against the REAL mock (never a fake of the seam): the
// id span matches the mock's compiled ordering, upward gestures arrive
// reversed (the clicked end is last — where the reducer puts the cursor), and
// a failed slice drops the gesture without touching the store.

import { expect, test, vi } from "vitest";
import { mockApi } from "@/api/mock";
import { DEFAULT_ARRANGEMENT, type Query } from "@/query-model/ast";
import { commitRange } from "./select-range";

const LIBRARY: Query = { version: 1, scope: { kind: "library" }, where: null };

async function orderedIds(): Promise<string[]> {
    const { items } = await mockApi.queryAssets(LIBRARY, DEFAULT_ARRANGEMENT, { offset: 0, limit: 500 });
    return items.map((row) => row.id);
}

test("a downward range commits the inclusive ascending span", async () => {
    const ids = await orderedIds();
    const dispatch = vi.fn();
    await commitRange(mockApi, LIBRARY, DEFAULT_ARRANGEMENT, 1, 4, dispatch);
    expect(dispatch).toHaveBeenCalledWith({ type: "range-committed", ids: ids.slice(1, 5) });
});

test("an upward range arrives reversed so the clicked end carries the cursor", async () => {
    const ids = await orderedIds();
    const dispatch = vi.fn();
    await commitRange(mockApi, LIBRARY, DEFAULT_ARRANGEMENT, 4, 1, dispatch);
    const expected = ids.slice(1, 5).reverse();
    expect(dispatch).toHaveBeenCalledWith({ type: "range-committed", ids: expected });
    expect(expected.at(-1)).toBe(ids[1]);
});

test("a failed slice drops the gesture — nothing reaches the store", async () => {
    const dispatch = vi.fn();
    const failing = { assetIdSlice: () => Promise.reject(new Error("catalog busy")) };
    await commitRange(failing, LIBRARY, DEFAULT_ARRANGEMENT, 0, 3, dispatch);
    expect(dispatch).not.toHaveBeenCalled();
});
