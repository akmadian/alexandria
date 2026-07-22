// The jobs store: the pure reducer keys envelopes by jobId, a done replaces the
// running entry, terminal entries persist, and a payload without a jobId is
// dropped. The curated hooks are exercised through the store's non-React apply.

import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { Envelope, JobDone, JobProgress } from "@/_generated-types/models";
import { applyJobEnvelope, reduceJobs, resetJobsForTests, useJob, useJobs } from "./jobs";

function progress(jobId: string, done: number, total: number): Envelope {
    const payload: JobProgress = {
        jobId,
        kind: "import",
        label: "jobs.kind.import",
        state: "running",
        done,
        total,
        totalKnown: true,
        stage: "write",
        cancelable: true,
    };
    return { topic: "jobs", type: "progress", payload, timestamp: "2026-07-21T00:00:00Z" };
}

function done(jobId: string): Envelope {
    const payload: JobDone = {
        jobId,
        kind: "import",
        state: "done",
        summary: { added: 34, updated: 3, skipped: 3, errors: 0 },
    };
    return { topic: "jobs", type: "done", payload, timestamp: "2026-07-21T00:00:01Z" };
}

afterEach(() => resetJobsForTests());

describe("reduceJobs (pure)", () => {
    it("keys the whole envelope by jobId", () => {
        const next = reduceJobs({}, progress("job-1", 10, 40));
        expect(next["job-1"].type).toBe("progress");
        expect((next["job-1"].payload as JobProgress).done).toBe(10);
    });

    it("a later envelope for the same job overwrites the prior one (done replaces running)", () => {
        let state = reduceJobs({}, progress("job-1", 20, 40));
        state = reduceJobs(state, done("job-1"));
        expect(state["job-1"].type).toBe("done");
        expect(Object.keys(state)).toHaveLength(1); // same key, replaced not appended
    });

    it("keeps terminal entries alongside other jobs", () => {
        let state = reduceJobs({}, done("job-1"));
        state = reduceJobs(state, progress("job-2", 5, 40));
        expect(state["job-1"].type).toBe("done"); // terminal entry persists
        expect(state["job-2"].type).toBe("progress");
    });

    it("drops an envelope whose payload carries no jobId (returns the same map)", () => {
        const before = { "job-1": progress("job-1", 1, 2) };
        const malformed: Envelope = { topic: "jobs", type: "progress", payload: {}, timestamp: "t" };
        expect(reduceJobs(before, malformed)).toBe(before);
    });
});

describe("apply + curated selectors", () => {
    it("lands an applied envelope under useJob and useJobs", () => {
        applyJobEnvelope(progress("job-1", 12, 40));
        const { result: one } = renderHook(() => useJob("job-1"));
        expect((one.current?.payload as JobProgress).done).toBe(12);

        applyJobEnvelope(progress("job-2", 1, 40));
        const { result: all } = renderHook(() => useJobs());
        expect(all.current).toHaveLength(2);
    });

    it("useJob is undefined for an unseen job", () => {
        const { result } = renderHook(() => useJob("nope"));
        expect(result.current).toBeUndefined();
    });
});
