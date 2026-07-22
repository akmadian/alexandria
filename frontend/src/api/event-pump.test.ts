// The event pump: catalog envelopes invalidate the query cache, jobs envelopes
// land in the jobs store, watcher/sync are ignored (no sink yet), and the
// subscription tears down on stop. The seam is mocked at the client boundary so
// this exercises the pump's routing, not the mock engine.

import { QueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EventType } from "@/_generated-types/events";
import type { Envelope } from "@/_generated-types/models";

vi.mock("./client", () => ({ api: { subscribe: vi.fn() } }));
vi.mock("@/stores/jobs", () => ({ applyJobEnvelope: vi.fn() }));

import { api } from "./client";
import { routeEnvelope, startEventPump } from "./event-pump";
import { applyJobEnvelope } from "@/stores/jobs";

const subscribe = vi.mocked(api.subscribe);
const applyJob = vi.mocked(applyJobEnvelope);

function envelope(topic: Envelope["topic"], type: string): Envelope {
    return { topic, type: type as EventType, payload: { jobId: "job-1" }, timestamp: "2026-07-21T00:00:00Z" };
}

afterEach(() => {
    vi.clearAllMocks();
});

describe("routeEnvelope", () => {
    it("invalidates the asset caches on a catalog envelope", async () => {
        const queryClient = new QueryClient();
        const invalidate = vi.spyOn(queryClient, "invalidateQueries");

        routeEnvelope(queryClient, envelope("catalog", "changed"));

        // invalidateCatalog hits both the list ("assets") and detail ("asset")
        // prefixes; the second await lands a microtask later, so wait it out.
        await vi.waitFor(() => {
            expect(invalidate).toHaveBeenCalledWith({ queryKey: ["assets"] });
            expect(invalidate).toHaveBeenCalledWith({ queryKey: ["asset"] });
        });
        expect(applyJob).not.toHaveBeenCalled();
    });

    it("routes a jobs envelope into the jobs store", () => {
        const queryClient = new QueryClient();
        const invalidate = vi.spyOn(queryClient, "invalidateQueries");
        const jobEnvelope = envelope("jobs", "progress");

        routeEnvelope(queryClient, jobEnvelope);

        expect(applyJob).toHaveBeenCalledWith(jobEnvelope);
        expect(invalidate).not.toHaveBeenCalled();
    });

    it("ignores watcher/sync envelopes (no sink yet)", () => {
        const queryClient = new QueryClient();
        const invalidate = vi.spyOn(queryClient, "invalidateQueries");

        routeEnvelope(queryClient, envelope("watcher", "sourceStatus"));
        routeEnvelope(queryClient, envelope("sync", "reserved"));

        expect(invalidate).not.toHaveBeenCalled();
        expect(applyJob).not.toHaveBeenCalled();
    });
});

describe("startEventPump", () => {
    it("subscribes on start, routes through the handler, and unsubscribes on stop", () => {
        const unsubscribe = vi.fn();
        subscribe.mockReturnValue(unsubscribe);
        const queryClient = new QueryClient();

        const stop = startEventPump(queryClient);
        expect(subscribe).toHaveBeenCalledTimes(1);

        // Drive the captured handler with a jobs envelope — end-to-end routing.
        const handler = subscribe.mock.calls[0][0];
        const jobEnvelope = envelope("jobs", "done");
        handler(jobEnvelope);
        expect(applyJob).toHaveBeenCalledWith(jobEnvelope);

        stop();
        expect(unsubscribe).toHaveBeenCalledTimes(1);
    });
});
