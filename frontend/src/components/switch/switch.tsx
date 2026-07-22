// Switch — the fourth leaf (frontend/CLAUDE.md §6): the immediate-effect
// boolean. Same ledger row as Checkbox (§5 toggles-on): ON = the accent fill.
// The shape signal is thumb POSITION — left/right reads with the accent unset,
// so §4 holds without a glyph. The classic RAC Switch API carries no
// validation, so neither does this component.

import {
    Switch as AriaSwitch,
    type SwitchProps as AriaSwitchProps,
} from "react-aria-components";
import type { ReactNode } from "react";
import { cx } from "@/lib/cx";
import styles from "./switch.module.css";

export type SwitchSize = "xs" | "sm" | "md" | "lg";

// C10: exhaustive by construction. The tier scales everything (D33 proportional): label
// role, the track/thumb geometry (derived from --alx-size-icon), min-height.
const SIZE_CLASSES = {
    xs: styles.sizeXs,
    sm: styles.sizeSm,
    md: styles.sizeMd,
    lg: styles.sizeLg,
} as const satisfies Record<SwitchSize, string>;

export interface SwitchProps extends Omit<AriaSwitchProps, "children" | "className" | "style"> {
    /** The label text, in the value-text ramp (regular). */
    children?: ReactNode;
    /** §8 size ladder: xs = 16px (inspector dense), sm/md/lg = 20/24/28. Scales the label,
     * the track/thumb (via the icon ramp), and the hit-row together (D33 proportional). */
    size?: SwitchSize;
    className?: string;
}

export function Switch({ children, size = "md", className, ...ariaProps }: SwitchProps) {
    return (
        <AriaSwitch {...ariaProps} className={cx(styles.switch, SIZE_CLASSES[size], className)}>
            <span className={styles.track}>
                <span className={styles.thumb} />
            </span>
            {children}
        </AriaSwitch>
    );
}
