// The single binding boundary — the app's shared language with the Go backend.
// Everything the frontend knows about the backend goes through `AlexandriaAPI`;
// nothing else touches the Wails runtime. The mock (./mock.ts) and, later, the
// Wails adapter (./wails-api.ts) both implement this; swapping is one line in
// ./client.ts.
//
// This is the reconciled contract per seam/01's ledger — the query model is the
// AST (C6), not the retired flat AssetFilter. Thin-vertical subset: the read
// surface the grid needs. Sources/tags/collections/settings/jobs/events + the
// mutation surface grow this interface in the widen phase.
//
// Conventions (seam/01–02, C7/C8/C9): resource verbs, envelopes absorb field
// growth, one job envelope, bytes never cross the seam (thumbnails via URL on the
// row), codes not strings (ApiError), forward-compatible enum handling.

import type { ColorLabel, Flag } from "@/_generated-types/enums";
import type { ApiErrorKind, ErrorCode } from "@/_generated-types/errors";
import type { AssetDetail, AssetRow as AssetRowModel } from "@/_generated-types/models";
import type { Arrangement, Page, Query } from "@/query-model/ast";

// Re-export the AST so consumers have one door for the query types.
export type { Arrangement, Page, Query };
export type { Scope, WhereNode, GroupNode, Leaf } from "@/query-model/ast";

// The detail read's wire model passes through unchanged (no presentation
// layering — the inspector renders it directly); re-exported so features
// import it from the contract, never the generated tree.
export type { AssetDetail };

/**
 * The slim grid-card projection (seam/01). The engine truth is the GENERATED
 * AssetRow model (C13/C15 — reflected from catalog.AssetRow's json tags);
 * the adapter layers two presentation facts on top: `kind` (the discriminator
 * that admits asset groups later) and `thumbURL` (the binary channel — a URL
 * derived from the asset id, never bytes). `rating: null` = unrated — NULL is
 * the truth end to end (03-data-model); 0 is not a rating.
 */
export type AssetRow = AssetRowModel & {
    kind: "asset";
    thumbURL: string;
};

export interface AssetQueryResult {
    items: AssetRow[];
    /** Total matching query, ignoring paging — sizes the grid scrollbar. */
    total: number;
}

/**
 * The write target (C5/C7 — mirrors seam.UpdateTarget): explicit ids, or
 * "everything matching this query except these ids". The seam accepts both, and
 * the query form compiles to ONE statement backend-side — but until the undo
 * round lands the net, the frontend only ever sends the `ids` form (task 34
 * ruling: no mass write without the net). This shape is hand-authored, a sibling
 * to the AST wire shapes in query-model/ast.ts: it references the generated
 * `Query`, which the model emitter can't project into the generated tree without
 * inverting the generated→hand dependency. Its keys are pinned to the generated
 * seam.UpdateTarget by a types-only assertion in wails-api.ts (C15's mechanism).
 */
export interface UpdateTarget {
    ids?: string[];
    query?: Query;
    exceptIds?: string[];
}

/**
 * The sparse triage patch (the wire face of seam.TriagePatchInput / the engine's
 * catalog.TriagePatch). Three states per field, encoded the way the seam decodes
 * them: KEY ABSENT = don't touch, `null` = clear, a value = set. Patches carry
 * ABSOLUTE values, never deltas, so a write is idempotent and safe to auto-retry
 * (frontend-architecture §Retry). Hand-authored over the generated ColorLabel/Flag
 * unions — the RawMessage three-state fields can't be reflected into typed
 * nullable-optional TS. Two drift mechanisms pin it (C15): the Go crosswalk
 * (checkTriagePatchInputWire, wire names ⇔ catalog.TriagePatch) and a types-only
 * keyof pin against the generated seam.TriagePatchInput in wails-api.ts.
 */
export interface TriagePatch {
    rating?: number | null;
    colorLabel?: ColorLabel | null;
    flag?: Flag | null;
    note?: string | null;
}

// Every failure normalizes here so consumers switch on kind/code, never sniff
// strings; display text stays frontend-owned (C14). Kind/code are generated.
export class ApiError extends Error {
    kind: ApiErrorKind;
    code?: ErrorCode;
    detail?: unknown;
    constructor(kind: ApiErrorKind, message: string, code?: ErrorCode, detail?: unknown) {
        super(message);
        this.name = "ApiError";
        this.kind = kind;
        this.code = code;
        this.detail = detail;
    }
}

export interface AlexandriaAPI {
    /** The workhorse (C7): absorbs every predicate over assets. Ordered window. */
    queryAssets(query: Query, arrangement: Arrangement, page: Page): Promise<AssetQueryResult>;

    /** Ids-only window over the compiled ordering — range-selection materialization. */
    assetIdSlice(query: Query, arrangement: Arrangement, fromIndex: number, toIndex: number): Promise<string[]>;

    /** Position of an asset in the ordered result, or null if absent — cursor keep-if-present. */
    indexOfAsset(query: Query, arrangement: Arrangement, id: string): Promise<number | null>;

    /** The full-asset detail projection — the inspector's read (C7: a distinct result shape). */
    getAsset(id: string): Promise<AssetDetail>;

    /**
     * The write workhorse (C7): absorbs every triage write. Applies a sparse,
     * absolute-valued patch to the target; resolves void on success, rejects with
     * an ApiError (unknown ids → not_found). Ordering across concurrent calls is
     * the mutation lane's job (api/mutation-lane.ts), not this contract's.
     */
    updateAssets(target: UpdateTarget, patch: TriagePatch): Promise<void>;
}
