// The mutation lane — ONE ordered FIFO for every catalog-editing call
// (frontend-architecture §Optimistic mutation × undo, point 2). Mutations, and
// later undo/redo, all enqueue here so the engine applies them in dispatch order:
// the next task starts only once the previous SETTLES (resolve or reject). An undo
// deterministically lands after the command it follows; interleaved rapid writes
// can't reorder underneath us. IPC is fast, so serial costs nothing perceptible.
//
// Deliberately tiny: a single promise chain, no priorities, no cancellation, no
// per-key lanes. Optimistic cache feedback stays keystroke-fast because it happens
// in the mutation's onMutate (immediate) — the lane only orders the SERVER calls.

import { log } from "@/lib/logger";

// The tail of the chain. Each enqueued task chains off it and becomes the new
// tail; `.catch` keeps a rejected task from breaking the chain for the next one.
let tail: Promise<unknown> = Promise.resolve();

/**
 * Run `task` after every previously enqueued task has settled, and resolve/reject
 * with its outcome. Ordering is by call order (dispatch order) — the invariant the
 * lane exists to guarantee.
 */
export function enqueue<T>(task: () => Promise<T>): Promise<T> {
    const run = tail.then(task, task);
    tail = run.catch(() => {
        // Swallowed only for the CHAIN's sake — the caller still sees the rejection
        // through `run`. A lane task that throws is logged where it's handled
        // (the mutation's onError), not here.
    });
    return run;
}

/**
 * Test-only reset so a suite doesn't inherit a poisoned or pending tail from a
 * prior case. Not part of the runtime surface.
 */
export function resetLaneForTests(): void {
    tail = Promise.resolve();
    log.debug("mutation lane reset (tests)");
}
