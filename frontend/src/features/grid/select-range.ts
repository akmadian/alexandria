// Range materialization — the "ranges are a gesture, not a storage format" seam
// call (frontend-architecture): shift-click names two endpoints, the ordered id
// span between them comes from the engine (assetIdSlice, inclusive bounds), and
// the store receives pure identity.
//
// Orientation contract: the reducer sets the cursor to `ids.at(-1)`, so the
// array is oriented with the CLICKED end last — a range dragged upward arrives
// reversed. A failed slice drops the gesture loudly in the log; selection is
// never partially applied.

import type { AlexandriaAPI } from "@/api/contract";
import { log } from "@/lib/logger";
import type { Arrangement, Query } from "@/query-model/ast";
import type { CatalogAction } from "@/stores/catalog-store";

export async function commitRange(
    api: Pick<AlexandriaAPI, "assetIdSlice">,
    query: Query,
    arrangement: Arrangement,
    anchorIndex: number,
    targetIndex: number,
    dispatch: (action: CatalogAction) => void,
): Promise<void> {
    const [fromIndex, toIndex] =
        anchorIndex <= targetIndex ? [anchorIndex, targetIndex] : [targetIndex, anchorIndex];
    try {
        const ids = await api.assetIdSlice(query, arrangement, fromIndex, toIndex);
        const oriented = targetIndex < anchorIndex ? [...ids].reverse() : ids;
        log.debug("grid: range committed", { fromIndex, toIndex, count: oriented.length });
        dispatch({ type: "range-committed", ids: oriented });
    } catch (error) {
        log.error("grid: range materialization failed — gesture dropped", { error: String(error) });
    }
}
