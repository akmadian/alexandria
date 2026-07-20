// LabelSwatch — the §5 color-label mark. One registered ColorLabel → swatch
// dispatch (C10: promoted at the second call site — the grid cell face and the
// inspector both render it; the mapping lived in cell-face until then). The
// five assignable LrC labels resolve to their §5 semantic roles; `orange` has
// no §5 role (dropped from the palette 2026-07-18) but the enum keeps it for
// XMP round-trip, so an imported orange label still renders — it falls back to
// the raw orange hue scale. State never by color alone (§10): consumers pair
// the mark with a word or an aria-label.

import type { ColorLabel } from "@/_generated-types/enums";
import { cx } from "@/lib/cx";
import styles from "./label-swatch.module.css";

// Complete over the generated union — a new label fails to compile until mapped.
const LABEL_SWATCH = {
    red: "var(--alx-label-red)",
    yellow: "var(--alx-label-yellow)",
    green: "var(--alx-label-green)",
    blue: "var(--alx-label-blue)",
    purple: "var(--alx-label-purple)",
    orange: "var(--alx-color-orange-solid)",
} satisfies Record<ColorLabel, string>;

export function LabelSwatch({
    label,
    className,
    "aria-label": ariaLabel,
}: {
    label: ColorLabel;
    className?: string;
    "aria-label"?: string;
}) {
    return (
        <span
            className={cx(styles.swatch, className)}
            style={{ backgroundColor: LABEL_SWATCH[label] }}
            aria-label={ariaLabel}
            role={ariaLabel === undefined ? undefined : "img"}
        />
    );
}
