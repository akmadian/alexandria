// The jobs store (plane 1's second slice, frontend-architecture §Event pump) —
// background-work state the event pump feeds and the status bar / activity drawer
// render. Like the catalog store: one Zustand store, mutated ONLY through a single
// reducer-style transition (`apply`); components read curated selector hooks, never
// raw state.
//
// The C8 envelope is kept WHOLE, keyed by jobId — the store borrows no server
// state, it just holds the last event each job emitted. A jobs/done envelope
// REPLACES the running entry at its key (same jobId), and terminal entries are
// KEPT (never pruned) so the status bar can render "last result" after a job ends.
// A new kind of background work is a new `kind` string on the same envelope — zero
// new store surface (C9: no private progress paths).

import { useMemo } from "react";
import { create } from "zustand";
import type { Envelope, JobDone, JobProgress } from "@/_generated-types/models";
import { log } from "@/lib/logger";

// Both jobs payloads carry a jobId — the map key. Narrowed from the envelope's
// `unknown` payload at the store boundary (the pump only routes jobs envelopes
// here, but the store validates rather than trusting the caller).
type JobPayload = JobProgress | JobDone;

interface JobsViewState {
    /** Last envelope per job, keyed by jobId. Terminal entries persist. */
    byId: Record<string, Envelope>;
    apply: (envelope: Envelope) => void;
}

/** The jobId a jobs envelope is about, or null if the payload isn't a jobs payload. */
function jobIdOf(envelope: Envelope): string | null {
    const payload = envelope.payload as Partial<JobPayload> | null;
    return payload !== null && typeof payload.jobId === "string" ? payload.jobId : null;
}

/**
 * The pure transition (exported for tests — this is the store's real internal
 * API). Keys the whole envelope by its jobId; a later envelope for the same job
 * (a progress tick, or the terminal done) overwrites the prior one. An envelope
 * with no jobId is dropped (returns the same map) — a malformed event is a hint
 * that never lands, never a crash (events are hints, C8).
 */
export function reduceJobs(byId: Record<string, Envelope>, envelope: Envelope): Record<string, Envelope> {
    const jobId = jobIdOf(envelope);
    if (jobId === null) {
        log.warn("jobs store: envelope without a jobId dropped", { type: envelope.type });
        return byId;
    }
    return { ...byId, [jobId]: envelope };
}

const useJobsStore = create<JobsViewState>((set) => ({
    byId: {},
    apply: (envelope) => set((state) => ({ byId: reduceJobs(state.byId, envelope) })),
}));

/**
 * Apply a jobs envelope from NON-React code (the event pump) — the pushNotice
 * idiom. The pump routes only jobs-topic envelopes here.
 */
export function applyJobEnvelope(envelope: Envelope): void {
    useJobsStore.getState().apply(envelope);
}

/** Test-only reset so a suite doesn't inherit jobs from a prior case. */
export function resetJobsForTests(): void {
    useJobsStore.setState({ byId: {} });
}

// --- curated selectors (the only public surface) -------------------------------

/** The latest envelope for one job, or undefined if unseen. */
export const useJob = (jobId: string): Envelope | undefined => useJobsStore((state) => state.byId[jobId]);

/**
 * Every job's latest envelope. The map is a stable reference (it only changes on
 * `apply`), so the array derivation is memoized against it — a fresh `Object.values`
 * each render would defeat Zustand's reference equality and re-render forever.
 */
export function useJobs(): Envelope[] {
    const byId = useJobsStore((state) => state.byId);
    return useMemo(() => Object.values(byId), [byId]);
}
