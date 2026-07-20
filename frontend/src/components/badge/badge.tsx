// Badge — the tag / label / status chip (design registries.json `tagRecipes`):
// four styles (tint · outline · fill · dot) across the hue scales, one grammar for
// every hue. "Components never assemble chips ad hoc" (tagRecipes) — the recipe is
// encoded here once, mapping (style, hue) onto the emitted per-hue scale tokens, so
// tags, type badges, and (later) filter pills all render through this one primitive.
//
// Domain-blind: it takes a style + hue + size + text. The hue union mirrors
// registries.json `tagRecipes.hues` (the 13 scales incl. gray); a hue's tokens are
// the strict path mirror (`color.<hue>.tint` → `--alx-color-<hue>-tint`). Sizes
// are the tagRecipes.sizes placement rungs (2026-07-19): each binds a full type
// role — never a bare font-size (§13 type units).

import type { CSSProperties, ReactNode } from "react";
import { cx } from "@/lib/cx";
import styles from "./badge.module.css";

export type BadgeStyle = "tint" | "outline" | "fill" | "dot";

export type BadgeSize = "inline" | "standard" | "prominent";

// C10: a new size fails to compile until it has a class.
const SIZE_CLASSES = {
    inline: styles.inline,
    standard: styles.standard,
    prominent: styles.prominent,
} as const satisfies Record<BadgeSize, string>;

export type BadgeHue =
    | "red"
    | "peach"
    | "orange"
    | "amber"
    | "lime"
    | "green"
    | "teal"
    | "cyan"
    | "blue"
    | "indigo"
    | "purple"
    | "magenta"
    | "gray";

// (style, hue) → the recipe's scale tokens. The `dot` style is neutral (ink.1 text
// + hairline border come from CSS); its colored mark is set on the mark element.
function recipeColors(style: BadgeStyle, hue: BadgeHue): CSSProperties {
    switch (style) {
        case "tint":
            return { background: `var(--alx-color-${hue}-tint)`, color: `var(--alx-color-${hue}-tint-ink)` };
        case "outline":
            return {
                background: `var(--alx-color-${hue}-tint)`,
                color: `var(--alx-color-${hue}-tint-ink)`,
                borderColor: `var(--alx-color-${hue}-line)`,
            };
        case "fill":
            return { background: `var(--alx-color-${hue}-solid)`, color: `var(--alx-color-${hue}-on-solid)` };
        case "dot":
            return {};
    }
}

export function Badge({
    hue,
    style = "tint",
    size = "standard",
    children,
}: {
    hue: BadgeHue;
    style?: BadgeStyle;
    /** Placement rung (tagRecipes.sizes): `inline` rides a text line without
     * expanding it; `standard` is the chip default; `prominent` stands alone. */
    size?: BadgeSize;
    children: ReactNode;
}) {
    return (
        <span
            className={cx(styles.badge, SIZE_CLASSES[size], style === "dot" && styles.dot)}
            style={recipeColors(style, hue)}
        >
            {style === "dot" && <span className={styles.mark} style={{ background: `var(--alx-color-${hue}-solid)` }} />}
            {children}
        </span>
    );
}
