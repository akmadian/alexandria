// CellFace — the LrC expanded-cell face, a PURE PROJECTION of one AssetRow:
// row in, pixels out. No store hooks, no api imports, no dispatch — identity,
// selection state, and gestures live on the GridCell mat. Feature-local by the
// data rule (frontend-architecture, 2026-07-19): primitives receive resolved
// presentation values; feature components may take domain objects, and this
// face is the grid's own §19 anatomy.
//
// Anatomy (the hybrid treatment, pending the CellLab ratification):
//   header   — 2×2 configurable metadata (top row brighter, LrC's emphasis)
//   thumbBox — framed, letterboxed thumbnail; flag top-left, type badge
//              bottom-right — opaque chrome marks only, never a scrim (§11)
//   footer   — five-position rating (components/rating) + label swatch (§5)
//
// Restraint (§10, ratified by eye 2026-07-19 — overturns the task-32 "no
// stars" ruling): a null flag/label and the baseline image type render
// NOTHING, but the rating slot ALWAYS shows the five-position readout —
// unrated renders hollow positions at the ramp's faintest member (Rating's
// `off` rung), present but nearly silent.

import { useTranslation } from "react-i18next";
import type { FileType } from "@/_generated-types/enums";
import type { AssetRow } from "@/api/contract";
import { Badge } from "@/components/badge/badge";
import { Icon } from "@/components/icon/icon";
import { LabelSwatch } from "@/components/label-swatch/label-swatch";
import { Rating } from "@/components/rating/rating";
import { cx } from "@/lib/cx";
import { formatBytes, formatDate } from "@/lib/format";
import styles from "./cell-face.module.css";

// The metadata a header slot can show. The grid's "configurable card overlays"
// (frontend-state-model §Grid) let the user pick what each of the four slots
// shows; the picker is a view-options surface (deferred). ponytail:
// DEFAULT_HEADER is the standing config until that surface lands.
export type CellField = "index" | "filename" | "dimensions" | "camera" | "capturedAt" | "size" | "none";

/** The four header slots — top-left, top-right, bottom-left, bottom-right. */
export type CellHeaderFields = [CellField, CellField, CellField, CellField];

export const DEFAULT_HEADER: CellHeaderFields = ["index", "filename", "dimensions", "camera"];

function resolveField(field: CellField, row: AssetRow, index: number): string {
    switch (field) {
        case "index":
            return String(index + 1);
        case "filename":
            return row.filename;
        case "dimensions":
            // Raw pixel counts, LrC-style — no thousands grouping (it only steals
            // width in a narrow cell and no one reads dimensions grouped).
            return row.width !== null && row.height !== null ? `${row.width} × ${row.height}` : "";
        case "camera":
            return row.cameraModel ?? "";
        case "capturedAt":
            return row.capturedAt !== null ? formatDate(row.capturedAt) : "";
        case "size":
            return formatBytes(row.sizeBytes);
        case "none":
            return "";
    }
}

// Type-badge i18n keys, complete over the union (C10/C14 — display text lives in
// the catalog, never in code). The baseline image is silent; the rest chip the
// thumbnail corner (the LrC badge spot) as achromatic intrinsic data (§10).
const TYPE_BADGE_KEY = {
    image: null,
    raw: "fileTypeBadge.raw",
    video: "fileTypeBadge.video",
    vector: "fileTypeBadge.vector",
    audio: "fileTypeBadge.audio",
    document: "fileTypeBadge.document",
} as const satisfies Record<FileType, string | null>;

export function CellFace({
    row,
    index,
    header = DEFAULT_HEADER,
}: {
    row: AssetRow;
    /** Position in the working set — grid state, not a row fact. */
    index: number;
    /** Which metadata each of the four header slots shows (user-configurable later). */
    header?: CellHeaderFields;
}) {
    const { t } = useTranslation();
    const [topLeft, topRight, bottomLeft, bottomRight] = header;
    const badgeKey = TYPE_BADGE_KEY[row.fileType];
    return (
        <div className={styles.face}>
            <div className={styles.header}>
                <span className={cx(styles.field, styles.fieldTop)}>{resolveField(topLeft, row, index)}</span>
                <span className={cx(styles.field, styles.fieldTop, styles.right)}>
                    {resolveField(topRight, row, index)}
                </span>
                <span className={styles.field}>{resolveField(bottomLeft, row, index)}</span>
                <span className={cx(styles.field, styles.right)}>{resolveField(bottomRight, row, index)}</span>
            </div>

            <div className={styles.thumbBox}>
                <img className={styles.thumb} src={row.thumbURL} alt={row.filename} draggable={false} />
                {row.flag === "pick" && (
                    <span className={styles.flag} aria-label={t("flag.pick")}>
                        <Icon concept="flag" className={styles.flagOn} />
                    </span>
                )}
                {row.flag === "reject" && (
                    <span className={cx(styles.flag, styles.reject)} aria-label={t("flag.reject")}>
                        <Icon concept="reject" />
                    </span>
                )}
                {badgeKey !== null && (
                    <span className={styles.typeBadge}>
                        <Badge hue="gray" style="outline" size="inline">
                            {t(badgeKey)}
                        </Badge>
                    </span>
                )}
            </div>

            <div className={styles.footer}>
                <Rating value={row.rating} />
                {row.colorLabel !== null && (
                    <LabelSwatch
                        label={row.colorLabel}
                        className={styles.label}
                        aria-label={t("cell.labelSwatch", { label: t(`colorLabel.${row.colorLabel}`) })}
                    />
                )}
            </div>
        </div>
    );
}
