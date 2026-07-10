import { FileText, Image, Music, Shapes, Video } from "lucide-react";
import type { ComponentType, PointerEvent } from "react";
import type { ColorLabel, FileType } from "@/_generated-types/enums";
import type { AssetRow } from "@/api/contract";
import { formatDuration, formatNumber } from "@/lib/format";
import { useIsCursor, useIsSelected } from "@/stores/catalog-store";
import s from "./grid-cell.module.css";

// Registry dispatch, not a conditional (C10): one glyph per file type, exhaustive.
const TYPE_GLYPH = {
    audio: Music,
    document: FileText,
    image: Image,
    raw: Image,
    vector: Shapes,
    video: Video,
} satisfies Record<FileType, ComponentType<{ size?: number }>>;

// The six color labels are the ONE sanctioned user-data hue — the DS keeps them in
// the primitive layer, surfaced only as user data (never as chrome). A registry map
// (not `var(--label-${x})` interpolation) makes the ColorLabel→token coupling
// compile-checked (C10/C13): a newly generated label without a swatch fails the build.
const LABEL_SWATCH = {
    blue: "var(--label-blue)",
    green: "var(--label-green)",
    orange: "var(--label-orange)",
    purple: "var(--label-purple)",
    red: "var(--label-red)",
    yellow: "var(--label-yellow)",
} satisfies Record<ColorLabel, string>;

interface GridCellProps {
    asset: AssetRow;
    index: number;
    onSelect: (index: number, event: PointerEvent) => void;
}

export function GridCell({ asset, index, onSelect }: GridCellProps) {
    const selected = useIsSelected(asset.id);
    const cursor = useIsCursor(asset.id);
    const TypeGlyph = TYPE_GLYPH[asset.fileType];

    return (
        <div
            role="gridcell"
            aria-selected={selected}
            className={s.cell}
            data-selected={selected}
            data-cursor={cursor}
            data-rejected={asset.flag === "reject"}
            onPointerDown={(event) => onSelect(index, event)}
        >
            <div className={s.thumb}>
                <img src={asset.thumbURL} alt="" draggable={false} />
                <div className={s.br}>
                    {asset.durationSecs !== null && <span className={s.duration}>{formatDuration(asset.durationSecs)}</span>}
                    <span className={s.type}>
                        <TypeGlyph size={13} />
                    </span>
                </div>
            </div>

            <div className={s.foot}>
                <div className={s.caption}>
                    <span className={s.name}>{asset.filename}</span>
                    <span className={s.badges}>
                        {asset.flag === "pick" && <span className={s.flag}>⚑</span>}
                        {asset.rating > 0 && <span className={s.stars}>{"★".repeat(asset.rating)}</span>}
                        {asset.colorLabel && (
                            <span className={s.chip} style={{ background: LABEL_SWATCH[asset.colorLabel] }} />
                        )}
                    </span>
                </div>
                {/* ponytail: "dim"/"cam" field labels are literals until the i18n key
                    catalog + per-field display registry land (widen); C14. */}
                <div className={s.meta}>
                    <span className={s.f}>
                        <span className={s.k}>dim</span>
                        <span className={s.v}>
                            {formatNumber(asset.width)} × {formatNumber(asset.height)}
                        </span>
                    </span>
                    {asset.cameraModel && (
                        <span className={s.f}>
                            <span className={s.k}>cam</span>
                            <span className={s.v}>{asset.cameraModel}</span>
                        </span>
                    )}
                </div>
            </div>
        </div>
    );
}
