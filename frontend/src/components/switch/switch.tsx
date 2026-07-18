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

export interface SwitchProps extends Omit<AriaSwitchProps, "children" | "className" | "style"> {
    /** The label text, in the value role. */
    children?: ReactNode;
    className?: string;
}

export function Switch({ children, className, ...ariaProps }: SwitchProps) {
    return (
        <AriaSwitch {...ariaProps} className={cx(styles.switch, className)}>
            <span className={styles.track}>
                <span className={styles.thumb} />
            </span>
            {children}
        </AriaSwitch>
    );
}
