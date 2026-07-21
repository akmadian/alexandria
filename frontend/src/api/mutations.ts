// TanStack write hooks — the catalog-editing counterpart to queries.ts. Every
// triage write goes through useUpdateAssets, which implements the locked
// optimistic-mutation discipline (frontend-architecture §Optimistic mutation ×
// undo — copy it, don't invent):
//
//   1. cancel-on-mutate + the invalidation GATE (invalidate only when the last
//      in-flight write settles, via `isMutating() === 1`), so a reconciling
//      refetch is always at least as new as the optimistic state it replaces;
//   2. the ordered LANE (api/mutation-lane.ts) serializes the SERVER calls in
//      dispatch order — retries ride INSIDE the lane task so a retry can never
//      jump behind a later write;
//   4. optimistic cache patch for ids-targets (prior values snapshotted), reverted
//      on failure after the quiet retries, with a LOUD notice.
//
// Undo/redo (point 3, pessimistic) and `all`-shaped optimism are later rounds.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { log } from "@/lib/logger";
import { pushNotice } from "@/stores/notices";
import { applyOptimisticPatch, cancelCatalogReads, type CacheSnapshot, invalidateCatalog, rollbackCatalog } from "./catalog-cache";
import { api } from "./client";
import { ApiError, type TriagePatch, type UpdateTarget } from "./contract";
import { enqueue } from "./mutation-lane";

// Quiet auto-retries before a visible rollback (frontend-architecture §Retry:
// idempotent-by-construction ⇒ 1–2). Short, capped backoff — local IPC, not a
// network. Retries live in the lane task, so this whole loop is ONE ordered slot.
const RETRY_ATTEMPTS = 1;
const BACKOFF_MS = 120;

const sleep = (ms: number): Promise<void> => new Promise((resolve) => setTimeout(resolve, ms));

async function attemptWrite(target: UpdateTarget, patch: TriagePatch): Promise<void> {
    let lastError: unknown;
    for (let attempt = 0; attempt <= RETRY_ATTEMPTS; attempt++) {
        try {
            await api.updateAssets(target, patch);
            return;
        } catch (error) {
            lastError = error;
            // A domain rejection (validation, not_found) is deterministic —
            // retrying re-sends an identical request against the same rule and
            // fails identically. Bail straight to the visible failure path;
            // the quiet retries exist for transient transport/busy failures.
            if (error instanceof ApiError && error.kind === "domain") break;
            if (attempt < RETRY_ATTEMPTS) {
                log.debug("api: updateAssets retrying", { attempt });
                await sleep(BACKOFF_MS * (attempt + 1));
            }
        }
    }
    throw lastError;
}

export interface TriageWrite {
    target: UpdateTarget;
    patch: TriagePatch;
}

interface WriteContext {
    snapshot?: CacheSnapshot;
}

/**
 * The triage write hook. Returns `writeTriage(target, patch)` — fire-and-forget:
 * feedback is the optimistic cache patch (instant), reconciliation is the gated
 * refetch, failure is a rollback + notice. The frontend only sends ids-targets
 * this round; the optimistic patch is skipped for anything else (no id list to
 * patch — that path invalidates, per point 4).
 */
export function useUpdateAssets() {
    const queryClient = useQueryClient();
    const mutation = useMutation<void, unknown, TriageWrite, WriteContext>({
        // The lane owns ordering: the mutationFn resolves only when this write's
        // turn comes and its retries finish. React Query's own retry stays OFF
        // (provider default) so a retry can't re-enter the lane out of order.
        mutationFn: ({ target, patch }) => enqueue(() => attemptWrite(target, patch)),
        onMutate: async ({ target, patch }) => {
            const ids = target.ids;
            if (ids === undefined || ids.length === 0) return {};
            await cancelCatalogReads(queryClient, ids);
            return { snapshot: applyOptimisticPatch(queryClient, ids, patch) };
        },
        onError: (error, variables, context) => {
            log.error("api: updateAssets failed", { error: String(error), target: variables.target });
            // Roll back only when no OTHER catalog-editing write is in flight
            // (self still counts here, so 1 = just us). With a concurrent write
            // pending, this snapshot predates that write's optimistic patch —
            // restoring it would clobber newer optimism (the TkDodo
            // concurrent-optimistic-updates hazard). Skip the cache revert and
            // let the settle-gate invalidation reconcile to engine truth; the
            // failure stays loud either way.
            if (context?.snapshot && queryClient.isMutating() === 1) {
                rollbackCatalog(queryClient, context.snapshot);
            }
            pushNotice("errors.writeFailed");
        },
        onSettled: () => {
            // The invalidation gate: refetch only when THIS is the last write in
            // flight (self still counts here), so the snapshot that replaces the
            // optimistic state reflects every settled write, not a mid-burst one.
            if (queryClient.isMutating() === 1) return invalidateCatalog(queryClient);
            return undefined;
        },
    });

    // Referentially stable across renders (mutate is stable in TanStack v5), so
    // effect consumers — the grid's window keydown listener — subscribe ONCE
    // instead of resubscribing every render.
    const { mutate } = mutation;
    const writeTriage = useCallback(
        (target: UpdateTarget, patch: TriagePatch): void => {
            log.debug("api: updateAssets dispatched", { count: target.ids?.length ?? 0 });
            mutate({ target, patch });
        },
        [mutate],
    );

    return { writeTriage };
}
