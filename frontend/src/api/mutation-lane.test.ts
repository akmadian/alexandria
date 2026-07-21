// The ordered lane is the write path's backbone (frontend-architecture point 2):
// interleaved rapid writes MUST settle in dispatch order, never reorder. These
// tests pin that invariant and the chain's survival across a rejected task.

import { beforeEach, describe, expect, it } from "vitest";
import { enqueue, resetLaneForTests } from "./mutation-lane";

beforeEach(() => {
    resetLaneForTests();
});

const settleAfter = <T>(value: T, ms: number, record: () => void): Promise<T> =>
    enqueue(
        () =>
            new Promise<T>((resolve) =>
                setTimeout(() => {
                    record();
                    resolve(value);
                }, ms),
            ),
    );

describe("mutation lane (FIFO)", () => {
    it("settles in dispatch order even when later tasks are faster", async () => {
        const settled: number[] = [];
        // Dispatch 1 (slow), 2 (fast), 3 (fast): a fast task must still wait its
        // turn behind the slow one it was dispatched after.
        const first = settleAfter(1, 40, () => settled.push(1));
        const second = settleAfter(2, 5, () => settled.push(2));
        const third = settleAfter(3, 5, () => settled.push(3));
        await Promise.all([first, second, third]);
        expect(settled).toEqual([1, 2, 3]);
    });

    it("resolves each task with its own value, in order", async () => {
        const results = await Promise.all([
            settleAfter("a", 20, () => {}),
            settleAfter("b", 1, () => {}),
            settleAfter("c", 10, () => {}),
        ]);
        expect(results).toEqual(["a", "b", "c"]);
    });

    it("keeps ordering and the chain alive across a rejected task", async () => {
        const settled: string[] = [];
        const ok = (name: string): Promise<string> =>
            enqueue(async () => {
                settled.push(name);
                return name;
            });
        const fail = (name: string): Promise<string> =>
            enqueue(async () => {
                settled.push(name);
                throw new Error(name);
            });

        const first = ok("a");
        const rejected = fail("b");
        const third = ok("c");

        await expect(first).resolves.toBe("a");
        await expect(rejected).rejects.toThrow("b"); // the rejection reaches its caller
        await expect(third).resolves.toBe("c"); // …and the next task still ran
        expect(settled).toEqual(["a", "b", "c"]);
    });
});
