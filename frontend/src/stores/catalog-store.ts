// Catalog view-state store (plane 1 of the three-plane model, frontend/09). One
// Zustand store, mutated ONLY through a single reducer-style `dispatch(action)` —
// the action vocabulary is the app's internal API. Components never read raw
// state: import the curated selector hooks below (store internals stay private).
//
// It holds the full C2 state equation shape — viewMode(query + arrangement,
// selection + cursor). This slice wires the selection/cursor action families the
// grid exercises; scope/filter/arrangement mutators (+ their reset invariants)
// arrive with the sidebar and filter bar (widen), which is why they default to
// library / no-filter / captured-desc here.

import { useMemo } from "react";
import { create } from "zustand";
import { type Arrangement, DEFAULT_ARRANGEMENT, type Query, type Scope, type WhereNode } from "@/query-model/ast";
import type { AssetID } from "@/stores/ids";

export type ViewMode = "grid" | "loupe" | "compare" | "cull";

// Selection is `{ids} | {all, except}` (frontend/02, frontend/09): "select all"
// NEVER enumerates — it flips to the `all` kind and tracks only de-selections.
export type Selection =
    | { kind: "ids"; ids: ReadonlySet<AssetID> }
    | { kind: "all"; except: ReadonlySet<AssetID> };

interface CatalogViewState {
    scope: Scope;
    filter: WhereNode | null;
    arrangement: Arrangement;
    viewMode: ViewMode;
    selection: Selection;
    cursorId: AssetID | null;
    dispatch: (action: CatalogAction) => void;
}

// The modifier grammar lives in the reducer, not the click handler. A range is a
// gesture: the grid materializes the id span (it owns order) and hands it in — the
// store stays pure identity (frontend/09 "ranges are a gesture, not a storage format").
export type CatalogAction =
    | { type: "asset-clicked"; id: AssetID; additive: boolean } // plain / cmd-click
    | { type: "range-committed"; ids: readonly AssetID[] } // shift-click / shift-arrow
    | { type: "cursor-set"; id: AssetID; select: boolean } // arrow-key nav
    | { type: "select-all" }
    | { type: "selection-cleared" }
    | { type: "filter-replaced"; filter: WhereNode | null } // scope/filter family — a query change
    | { type: "working-set-changed"; total: number; firstId: AssetID | null }; // data echo from the fetch

const EMPTY: ReadonlySet<AssetID> = new Set();

// The reducer reads only these fields; narrowing the parameter makes it a pure
// function unit tests can drive without constructing the whole store.
type ReducerState = Pick<CatalogViewState, "selection" | "cursorId" | "filter">;

// Exported for unit tests — this reducer is the app's real internal API (frontend/09).
export function reduce(state: ReducerState, action: CatalogAction): Partial<ReducerState> {
    switch (action.type) {
        case "asset-clicked": {
            if (!action.additive) return { selection: { kind: "ids", ids: new Set([action.id]) }, cursorId: action.id };
            if (state.selection.kind === "all") {
                const except = new Set(state.selection.except);
                if (except.has(action.id)) except.delete(action.id);
                else except.add(action.id);
                return { selection: { kind: "all", except }, cursorId: action.id };
            }
            const ids = new Set(state.selection.ids);
            if (ids.has(action.id)) ids.delete(action.id);
            else ids.add(action.id);
            return { selection: { kind: "ids", ids }, cursorId: action.id };
        }
        case "range-committed":
            return { selection: { kind: "ids", ids: new Set(action.ids) }, cursorId: action.ids.at(-1) ?? state.cursorId };
        case "cursor-set":
            return action.select
                ? { selection: { kind: "ids", ids: new Set([action.id]) }, cursorId: action.id }
                : { cursorId: action.id };
        case "select-all":
            return { selection: { kind: "all", except: EMPTY } };
        case "selection-cleared":
            return { selection: { kind: "ids", ids: EMPTY } };
        case "filter-replaced":
            // A query change resets the ephemeral tiers (frontend/02): selection
            // cleared, cursor dropped so the working-set-changed echo re-seeds it to
            // the new first row. ponytail: keeping the cursor's asset if it survives
            // the new query (LrC) needs an async IndexOfAsset lookup in the feature
            // layer; TRIGGER: build cursor keep-if-present across queries.
            return { filter: action.filter, selection: { kind: "ids", ids: EMPTY }, cursorId: null };
        case "working-set-changed":
            // The cursor exists iff the working set is non-empty (frontend/09): seed
            // it to the first row when we hold none, clear it when the set empties.
            // Selection is untouched — membership doesn't move with the data (C4).
            if (action.total === 0) return { cursorId: null };
            return state.cursorId === null ? { cursorId: action.firstId } : {};
    }
}

const useCatalogStore = create<CatalogViewState>((set) => ({
    scope: { kind: "library" },
    filter: null,
    arrangement: DEFAULT_ARRANGEMENT,
    viewMode: "grid",
    selection: { kind: "ids", ids: EMPTY },
    // Starts null; the grid's working-set-changed echo seeds it to the first row
    // once the query resolves and clears it when the set empties, satisfying the
    // frontend/09 invariant "cursor exists whenever the working set is non-empty".
    cursorId: null,
    dispatch: (action) => set((state) => reduce(state, action)),
}));

// Pure selection math (exported for tests); the hooks below are thin wrappers.
export function selectionHas(selection: Selection, id: AssetID): boolean {
    return selection.kind === "all" ? !selection.except.has(id) : selection.ids.has(id);
}
/** Selection size given the working-set total (needed to size an `all` selection). */
export function selectionSize(selection: Selection, total: number): number {
    return selection.kind === "all" ? total - selection.except.size : selection.ids.size;
}

// --- curated selectors (the only public surface) -------------------------------
// Each returns a primitive or a stable reference so a cell re-renders only when
// its own bit changes.

export const useIsSelected = (id: AssetID): boolean => useCatalogStore((s) => selectionHas(s.selection, id));
export const useIsCursor = (id: AssetID): boolean => useCatalogStore((s) => s.cursorId === id);
/** The whole cursor, reactively — for the surfaces whose SUBJECT it is (§15
 * active: inspector today, Loupe when it lands). Cells keep using the per-id
 * `useIsCursor` so a cursor move re-renders two cells, never the column. */
export const useCursorId = (): AssetID | null => useCatalogStore((s) => s.cursorId);

/** Non-reactive cursor read for EVENT HANDLERS (Zustand getState): gesture code
 * needs the current cursor without subscribing its owner to every cursor move,
 * so the grid's click handler stays referentially stable and memoized cells
 * bail. Never call during render — reactive reads go through the curated hooks
 * above (a whole-cursor `useCursorId` returns when Loupe needs its subject). */
export const readCursorId = (): AssetID | null => useCatalogStore.getState().cursorId;

/**
 * The C5 write target as PURE selection math (exported for tests): the selection
 * if non-empty, else the cursor as a singleton. `[]` = nothing to act on (no
 * selection, no cursor); `null` = an `all`-shaped selection, a mass write the
 * frontend deliberately does NOT send until the undo round lands the net (task 34
 * ruling; the seam accepts the query form, the frontend gates it).
 */
export function triageTargetIds(selection: Selection, cursorId: AssetID | null): AssetID[] | null {
    if (selection.kind === "all") return null;
    if (selection.ids.size > 0) return [...selection.ids];
    return cursorId === null ? [] : [cursorId];
}

/** The C5 write target resolved NON-reactively for verb dispatch (keyboard triage) —
 * a getState read for event handlers, like readCursorId. Never call during render. */
export function readTriageTargetIds(): AssetID[] | null {
    const { selection, cursorId } = useCatalogStore.getState();
    return triageTargetIds(selection, cursorId);
}
export const useFilter = (): WhereNode | null => useCatalogStore((s) => s.filter);
export const useViewMode = (): ViewMode => useCatalogStore((s) => s.viewMode);
export const useCatalogDispatch = (): ((action: CatalogAction) => void) => useCatalogStore((s) => s.dispatch);
export const useSelectionCount = (total: number): number => useCatalogStore((s) => selectionSize(s.selection, total));

/**
 * The canonical query + arrangement — a memoized DERIVATION, never stored (C2), so
 * the fetch key and the pills can't disagree. scope/filter/arrangement are stable
 * store references, so the memo only recomputes when one actually changes.
 */
export function useCatalogQuery(): { query: Query; arrangement: Arrangement } {
    const scope = useCatalogStore((s) => s.scope);
    const filter = useCatalogStore((s) => s.filter);
    const arrangement = useCatalogStore((s) => s.arrangement);
    return useMemo(() => ({ query: { version: 1, scope, where: filter }, arrangement }), [scope, filter, arrangement]);
}
