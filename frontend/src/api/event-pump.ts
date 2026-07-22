// The event pump (frontend-architecture §Event pump) — THE one subscriber to the
// C8 event stream. It routes each envelope by topic to its sink and nothing else
// subscribes: features read the routed sinks (the jobs store, the query cache),
// never the raw stream.
//
//   catalog → the invalidation gate (catalog-cache.invalidateCatalog): the engine
//             owns the freshness signal, so a catalog event marks the asset caches
//             stale and the reconciling refetch pulls engine truth.
//   jobs    → the jobs store (applyJobEnvelope): the whole envelope, keyed by jobId.
//   watcher → connectivity / toasts (DEFERRED — no producer yet; logged for the dev
//   sync      corner until those surfaces land).
//
// Wired into app boot where the QueryClient lives (app/providers). Boot calls
// startEventPump(queryClient); its return value tears the subscription down.

import type { QueryClient } from "@tanstack/react-query";
import type { Envelope } from "@/_generated-types/models";
import { log } from "@/lib/logger";
import { applyJobEnvelope } from "@/stores/jobs";
import { invalidateCatalog } from "./catalog-cache";
import { api } from "./client";

/**
 * Route one envelope to its sink (exported for tests — the pure routing decision,
 * given a QueryClient). An unknown topic is logged and ignored (forward-compatible:
 * a new generated topic without a sink degrades to a no-op, never a throw).
 */
export function routeEnvelope(queryClient: QueryClient, envelope: Envelope): void {
    switch (envelope.topic) {
        case "catalog":
            // Fire-and-forget: invalidation marks stale; the mutation lane's gate
            // (mutations.ts) defers the refetch while writes are in flight.
            void invalidateCatalog(queryClient);
            log.debug("event pump: catalog invalidated", { type: envelope.type });
            return;
        case "jobs":
            applyJobEnvelope(envelope);
            log.debug("event pump: job envelope applied", { type: envelope.type });
            return;
        case "watcher":
        case "sync":
            // No sink yet (connectivity + sync toasts are later rounds). Keep the
            // envelope visible in the log rather than dropping it silently.
            log.debug("event pump: no sink for topic yet", { topic: envelope.topic, type: envelope.type });
            return;
    }
}

/**
 * Subscribe the pump to the seam's event stream. Returns an unsubscribe function
 * (boot calls this from an effect and returns it as the cleanup). Idempotent per
 * call — each start opens exactly one subscription and hands back its teardown.
 */
export function startEventPump(queryClient: QueryClient): () => void {
    log.info("event pump: starting");
    const unsubscribe = api.subscribe((envelope) => routeEnvelope(queryClient, envelope));
    return () => {
        unsubscribe();
        log.info("event pump: stopped");
    };
}
