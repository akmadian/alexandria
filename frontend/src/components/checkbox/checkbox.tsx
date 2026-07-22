// Checkbox — the third leaf (frontend/CLAUDE.md §6): a 16px box + label. The
// checked fill is the accent's REGISTERED "toggles-on" ledger row (§5) — the
// first consumer of the emitted --alx-accent-on pairing — and the mark glyph
// keeps the state readable with the accent unset (§4: hue is never the sole
// signal). Indeterminate is §25's mixed state, rendered as the registered
// `mixed` concept. The glyph is conditional in the tree, not hidden by CSS —
// an unchecked box contains nothing.

import {
    Checkbox as AriaCheckbox,
    type CheckboxProps as AriaCheckboxProps,
} from "react-aria-components";
import type { ReactNode } from "react";
import { Icon } from "@/components/icon/icon";
import { cx } from "@/lib/cx";
import styles from "./checkbox.module.css";

export type CheckboxSize = "xs" | "sm" | "md" | "lg";

// C10: exhaustive by construction, mirroring the control primitives. The tier scales
// everything (D33 proportional): label role, the box (via --alx-size-icon), min-height.
const SIZE_CLASSES = {
    xs: styles.sizeXs,
    sm: styles.sizeSm,
    md: styles.sizeMd,
    lg: styles.sizeLg,
} as const satisfies Record<CheckboxSize, string>;

export interface CheckboxProps
    extends Omit<AriaCheckboxProps, "children" | "className" | "style"> {
    /** The label text, in the value-text ramp (regular). */
    children?: ReactNode;
    /** §8 size ladder: xs = 16px (inspector dense), sm/md/lg = 20/24/28. Scales the label,
     * the box (via the icon ramp), and the hit-row together (D33 proportional). */
    size?: CheckboxSize;
    className?: string;
}

export function Checkbox({ children, size = "md", className, ...ariaProps }: CheckboxProps) {
    return (
        <AriaCheckbox {...ariaProps} className={cx(styles.checkbox, SIZE_CLASSES[size], className)}>
            {({ isSelected, isIndeterminate }) => (
                <>
                    <span className={styles.box}>
                        {isIndeterminate ? (
                            <Icon concept="mixed" className={styles.glyph} />
                        ) : (
                            isSelected && <Icon concept="check" className={styles.glyph} />
                        )}
                    </span>
                    {children}
                </>
            )}
        </AriaCheckbox>
    );
}
