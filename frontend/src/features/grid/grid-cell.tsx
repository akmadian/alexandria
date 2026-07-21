// The grid cell: the interaction mat around CellFace. The mat rides the RAISING
// cell family (§20/§7 declared exception — never a dark surround beside a
// photograph); selection shades the MAT, never the photo (§11); the cursor
// renders the ACTIVE row (family ceiling + ink hairline frame — ratified task 31).
// Each cell subscribes to its OWN selection/cursor bits via the curated hooks,
// and the grid hands down a referentially stable click handler, so a click
// re-renders two cells, not the viewport.
//
// Division of labor (frontend-architecture, 2026-07-19): identity, state, and
// gestures live HERE; everything painted from the row lives in CellFace, a pure
// projection of AssetRow. Keyboard focus arrives with the actions registry; the
// machinery/attention marks with the enrichment inspector read (DEFERRED §13).
// Captured-face cell only — generated/glyph faces land with the assettype
// registry (§19).

import { memo, type MouseEvent } from "react";
import type { AssetRow } from "@/api/contract";
import { useIsCursor, useIsSelected } from "@/stores/catalog-store";
import { CellFace } from "./cell-face";
import styles from "./grid-cell.module.css";

export type CellClickHandler = (event: MouseEvent, id: string, index: number) => void;

export const GridCell = memo(function GridCell({
    row,
    index,
    onCellClick,
}: {
    /** Undefined = the row's block isn't resident — the grey placeholder mat.
     * The normal mid-scroll state under the block model: rows outside the
     * viewport's resident blocks render this until their block lands. */
    row: AssetRow | undefined;
    index: number;
    onCellClick: CellClickHandler;
}) {
    if (row === undefined) return <div className={styles.cell} />;
    return <LoadedCell row={row} index={index} onCellClick={onCellClick} />;
});

function LoadedCell({
    row,
    index,
    onCellClick,
}: {
    row: AssetRow;
    index: number;
    onCellClick: CellClickHandler;
}) {
    const selected = useIsSelected(row.id);
    const cursor = useIsCursor(row.id);
    return (
        // ponytail: mouse-only — keyboard (arrows, Enter/Space) is the
        // actions-registry round. The ARIA grid semantics are real today:
        // tabIndex -1 is the roving-tabindex REST state (that round promotes
        // the cursor cell to 0 and adds the key listeners).
        // eslint-disable-next-line jsx-a11y/click-events-have-key-events
        <div
            role="gridcell"
            aria-selected={selected}
            tabIndex={-1}
            className={styles.cell}
            data-selected={selected || undefined}
            data-cursor={cursor || undefined}
            onClick={(event) => onCellClick(event, row.id, index)}
        >
            <CellFace row={row} index={index} />
        </div>
    );
}
