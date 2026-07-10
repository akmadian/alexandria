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
    | { type: "selection-cleared" };

const EMPTY: ReadonlySet<AssetID> = new Set();

// The reducer reads only these fields; narrowing the parameter makes it a pure
// function unit tests can drive without constructing the whole store.
type SelectionState = Pick<CatalogViewState, "selection" | "cursorId">;

// Exported for unit tests — this reducer is the app's real internal API (frontend/09).
export function reduce(state: SelectionState, action: CatalogAction): Partial<SelectionState> {
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
    }
}

const useCatalogStore = create<CatalogViewState>((set) => ({
    scope: { kind: "library" },
    filter: null,
    arrangement: DEFAULT_ARRANGEMENT,
    viewMode: "grid",
    selection: { kind: "ids", ids: EMPTY },
    // ponytail: cursor starts null and is seeded on first interaction only. The
    // frontend/09 invariant "cursor exists whenever the working set is non-empty"
    // needs a `working-set-changed(total)` echo action that seeds cursor→index 0
    // (or clears it when empty). TRIGGER: wire it when the grid feeds the query
    // total back into the store — the same point the windowed-fetch widen lands.
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
export const useCursorId = (): AssetID | null => useCatalogStore((s) => s.cursorId);
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
