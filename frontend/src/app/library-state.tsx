// The one piece of shared client state (_project-tracking/frontend/02-state-model.md):
// browse target, filter bar, view mode, selection. Server state lives in
// TanStack Query; machine prefs live in localStorage; everything else derives.
//
// A reducer (not scattered useState) because selection semantics are one
// cohesive state machine and the keyboard layer dispatches the same actions
// the mouse does. Exported pure for tests.

import { createContext, useContext, useReducer, type Dispatch, type ReactNode } from "react";
import type { AssetFilter, AssetScope, AssetSort, FileType, ListQuery } from "@/api/contract";

export type BrowseTarget =
    | { kind: "all" }
    | { kind: "recent" }
    | { kind: "source"; id: string }
    | { kind: "collection"; id: string }
    | { kind: "tag"; id: string };

export type SortKey = "captured-desc" | "captured-asc" | "rating-desc" | "name-asc" | "size-desc";
export type Density = "compact" | "comfortable";

export interface FilterBarState {
    search: string;
    fileType: FileType | "all";
    minRating: number;
    sort: SortKey;
    density: Density;
}

export interface LibraryState {
    target: BrowseTarget;
    filters: FilterBarState;
    viewMode: "grid" | "loupe";
    selection: ReadonlySet<string>;
    /** Range-select anchor AND the inspector's subject (most recent selection). */
    lastSelectedId: string | null;
}

export const initialState: LibraryState = {
    target: { kind: "all" },
    filters: { search: "", fileType: "all", minRating: 0, sort: "captured-desc", density: "comfortable" },
    viewMode: "grid",
    selection: new Set(),
    lastSelectedId: null,
};

export type LibraryAction =
    | { type: "selectTarget"; target: BrowseTarget }
    | { type: "setFilters"; patch: Partial<FilterBarState> }
    | { type: "setViewMode"; mode: LibraryState["viewMode"] }
    /** Plain click replaces; additive (mod-click) toggles; range (shift-click)
     *  passes the ordered ids between anchor and target — the grid knows row order. */
    | { type: "select"; id: string; additive?: boolean; rangeIds?: string[] }
    | { type: "selectMany"; ids: string[] }
    | { type: "clearSelection" };

export function libraryReducer(state: LibraryState, action: LibraryAction): LibraryState {
    switch (action.type) {
        case "selectTarget":
            // New view, new world: selection and search don't survive; sort/density/type do.
            return { ...state, target: action.target, selection: new Set(), lastSelectedId: null, filters: { ...state.filters, search: "" } };
        case "setFilters":
            return { ...state, filters: { ...state.filters, ...action.patch } };
        case "setViewMode":
            return { ...state, viewMode: action.mode };
        case "select": {
            if (action.rangeIds && state.lastSelectedId) {
                // Shift-click: extend from anchor; keep the anchor.
                const next = new Set(state.selection);
                for (const id of action.rangeIds) next.add(id);
                return { ...state, selection: next };
            }
            if (action.additive) {
                const next = new Set(state.selection);
                if (next.has(action.id)) {
                    next.delete(action.id);
                    const last = state.lastSelectedId === action.id ? ([...next][next.size - 1] ?? null) : state.lastSelectedId;
                    return { ...state, selection: next, lastSelectedId: last };
                }
                next.add(action.id);
                return { ...state, selection: next, lastSelectedId: action.id };
            }
            return { ...state, selection: new Set([action.id]), lastSelectedId: action.id };
        }
        case "selectMany":
            return { ...state, selection: new Set(action.ids), lastSelectedId: action.ids[action.ids.length - 1] ?? null };
        case "clearSelection":
            return { ...state, selection: new Set(), lastSelectedId: null };
    }
}

/** target + filters → the ListQuery (state-model doc: collections are scopes;
 *  sources/tags are filter fields). Pure; memoized at the call site. */
export function deriveListQuery(state: LibraryState): ListQuery {
    const { target, filters } = state;
    let scope: AssetScope = { kind: "library" };
    const filter: AssetFilter = {};

    if (target.kind === "collection") scope = { kind: "collection", id: target.id };
    if (target.kind === "source") filter.sourceIds = [target.id];
    if (target.kind === "tag") filter.tagIds = [target.id];

    if (filters.search.trim()) filter.searchText = filters.search.trim();
    if (filters.fileType !== "all") filter.fileTypes = [filters.fileType];
    if (filters.minRating > 0) filter.ratingMin = filters.minRating;

    const sortFor: Record<SortKey, AssetSort> = {
        "captured-desc": { field: "captured", dir: "desc" },
        "captured-asc": { field: "captured", dir: "asc" },
        "rating-desc": { field: "rating", dir: "desc" },
        "name-asc": { field: "filename", dir: "asc" },
        "size-desc": { field: "size", dir: "desc" },
    };

    return {
        scope,
        filter,
        sort: target.kind === "recent" ? { field: "added", dir: "desc" } : sortFor[filters.sort],
        ...(target.kind === "recent" ? { page: { limit: 24, offset: 0 } } : null),
    };
}

// --- contexts: state and dispatch split so dispatch-only consumers never re-render ---

const StateCtx = createContext<LibraryState | null>(null);
const DispatchCtx = createContext<Dispatch<LibraryAction> | null>(null);

export const LibraryProvider = ({ children }: { children: ReactNode }) => {
    const [state, dispatch] = useReducer(libraryReducer, initialState);
    return (
        <StateCtx.Provider value={state}>
            <DispatchCtx.Provider value={dispatch}>{children}</DispatchCtx.Provider>
        </StateCtx.Provider>
    );
};

export function useLibraryState(): LibraryState {
    const s = useContext(StateCtx);
    if (!s) throw new Error("useLibraryState outside LibraryProvider");
    return s;
}

export function useLibraryDispatch(): Dispatch<LibraryAction> {
    const d = useContext(DispatchCtx);
    if (!d) throw new Error("useLibraryDispatch outside LibraryProvider");
    return d;
}
