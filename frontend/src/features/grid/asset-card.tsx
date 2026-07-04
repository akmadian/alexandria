// One AssetRow → one tile. The most-instantiated component in the app: pure,
// memoized, emits pointer intent upward, renders only what the slim row carries.

import { Check, Star, X } from "lucide-react";
import { memo } from "react";
import { cx } from "@/lib/cx";
import { fileTypeDisplay } from "@/lib/enum-display";
import { formatDuration } from "@/lib/format";
import { Icon } from "@/components/icon/icon";
import type { AssetRow } from "@/api/contract";
import s from "./asset-card.module.css";

interface AssetCardProps {
    row: AssetRow;
    selected: boolean;
    onPointerDown: (row: AssetRow, e: React.PointerEvent) => void;
}

export const AssetCard = memo(({ row, selected, onPointerDown }: AssetCardProps) => {
    const type = fileTypeDisplay(row.fileType);
    return (
        <div
            role="gridcell"
            aria-selected={selected}
            className={cx(s.card, selected && s.selected, row.flag === "reject" && s.rejected)}
            onPointerDown={(e) => onPointerDown(row, e)}
        >
            <div className={s.thumb}>
                <img src={row.thumbURL} alt="" loading="lazy" draggable={false} />
                {row.colorLabel && <span className={s.labelStrip} style={{ background: `var(--label-${row.colorLabel})` }} />}
                {row.flag === "pick" && <Icon icon={Check} size={12} className={s.pick} />}
                {row.flag === "reject" && <Icon icon={X} size={12} className={s.reject} />}
                {row.durationSecs != null && <span className={cx(s.duration, "u-data")}>{formatDuration(row.durationSecs)}</span>}
            </div>
            <div className={s.meta}>
                <Icon icon={type.icon} size={12} className={s.typeIcon} />
                <span className={s.name}>{row.filename}</span>
                {row.rating > 0 && (
                    <span className={cx(s.rating, "u-data")}>
                        {row.rating}
                        <Star size={10} strokeWidth={1.5} fill="var(--accent)" color="var(--accent)" />
                    </span>
                )}
            </div>
        </div>
    );
});
AssetCard.displayName = "AssetCard";
