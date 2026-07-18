// The §19 grid cell: a mat carrying a letterboxed thumbnail. The mat rides the
// RAISING cell family (§20/§7 declared exception — never a dark surround beside
// a photograph); selection shades the MAT, never the photo (§11). The cursor
// renders the ACTIVE row (family ceiling + ink hairline frame — ratified
// interpretation, task 31). Each cell subscribes to its OWN selection/cursor
// bits via the curated hooks, so a click re-renders two cells, not the grid.
//
// Slots (index/rating/flag/label/badge/machinery dot) are the triage round's;
// keyboard focus arrives with the actions registry. This is a captured-face
// cell only — generated/glyph faces land with the assettype registry (§19).

import { memo, type MouseEvent } from "react";
import type { AssetRow } from "@/api/contract";
import { useIsCursor, useIsSelected } from "@/stores/catalog-store";
import styles from "./grid-cell.module.css";

export type CellClickHandler = (event: MouseEvent, id: string, index: number) => void;

export const GridCell = memo(function GridCell({
    row,
    index,
    onCellClick,
}: {
    /** Undefined = the row's block hasn't landed — the grey placeholder mat.
     * ponytail: unreachable under the single-page fetch (queries.ts loads the
     * whole vertical); the block-model widen is what produces gaps. */
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
            <img className={styles.thumb} src={row.thumbURL} alt={row.filename} draggable={false} />
        </div>
    );
}
