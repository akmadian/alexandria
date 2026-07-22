// Button — the first v3 primitive (frontend/CLAUDE.md §6): React Aria owns the
// behavior, the emitted tokens own the look. Prominence is the §4 style-rung
// ladder; hue never enters — chrome is achromatic, and the hero rung's
// polychrome is the injected fun layer (§17), not a color choice.

import { Button as AriaButton, type ButtonProps as AriaButtonProps } from "react-aria-components";
import { cx } from "@/lib/cx";
import styles from "./button.module.css";

export type ButtonRung = "ghost" | "outline" | "tint" | "fill" | "hero";
export type ButtonSize = "xs" | "sm" | "md" | "lg";

// C10: exhaustive by construction — a new rung fails to compile until it has a class.
const RUNG_CLASSES = {
    ghost: styles.ghost,
    outline: styles.outline,
    tint: styles.tint,
    fill: styles.fill,
    hero: styles.hero,
} as const satisfies Record<ButtonRung, string>;

const SIZE_CLASSES = {
    xs: styles.controlXsmall,
    sm: styles.controlSmall,
    md: styles.controlMedium,
    lg: styles.controlLarge,
} as const satisfies Record<ButtonSize, string>;

export interface ButtonProps extends Omit<AriaButtonProps, "className" | "style"> {
    /** §4 prominence rung. Outline is the default affordance; ghost is opt-in quiet. */
    rung?: ButtonRung;
    /** §8 size ladder: xs = 16px (inspector inline-edit; mouse-only sub-floor), sm = 20px
     * (dense/inline; keeps a 24px hit target), md = 24px (the dense-tool default), lg = 28px
     * (dialog CTAs, hero spots). */
    size?: ButtonSize;
    className?: string;
}

export function Button({ rung = "outline", size = "md", className, ...ariaProps }: ButtonProps) {
    return (
        <AriaButton
            {...ariaProps}
            className={cx(styles.button, RUNG_CLASSES[rung], SIZE_CLASSES[size], className)}
        />
    );
}
