// ControlRow — a label paired with any control/value, on a row of its OWN height.
// Unlike Row (§8: density is an intent bound to type roles, for read-only metadata),
// ControlRow sizes on the CONTROL ladder (16/20/24/28) decoupled from that binding:
// the row owns only its height + its label's role; the hosted content brings its OWN
// size (no cascade — D33). Content is centered vertically; the label sits left, the
// control fills the rest left-aligned (form style, D34). A ControlGroup can set
// --control-row-label to a shared column width so a stack of rows aligns.
//
// Presentational structure only (mirrors Row): the hosted control owns its accessible
// name via its own aria-label/label, as the judgment editors do. If a consumer needs
// the visible label associated, role="group" + aria-labelledby is the easy add.

import type { ReactNode } from "react";
import { cx } from "@/lib/cx";
import styles from "./control-row.module.css";

export type ControlRowSize = "xs" | "sm" | "md" | "lg";

// C10: exhaustive by construction, mirroring Button. Height only — the tier does NOT
// reach into the hosted control's size.
const SIZE_CLASSES = {
    xs: styles.controlXsmall,
    sm: styles.controlSmall,
    md: styles.controlMedium,
    lg: styles.controlLarge,
} as const satisfies Record<ControlRowSize, string>;

// The row's own label steps with the height (§8 density↔type, for the label): the
// medium control-text ramp Button rides. The hosted content is exempt by design.
const LABEL_ROLES = {
    xs: "alx-type-control-xs",
    sm: "alx-type-control-sm",
    md: "alx-type-control",
    lg: "alx-type-control-lg",
} as const satisfies Record<ControlRowSize, string>;

export interface ControlRowProps {
    /** Left slot: the row's name. End-truncates; hover-reveals the full string. */
    label: ReactNode;
    /** Row height on the control ladder: xs = 16px, sm = 20px, md = 24px, lg = 28px. */
    size?: ControlRowSize;
    /** Recessed filled-chip treatment (D35 control-container): the row reads as a
     * recessed chip on the panel, for value-list rows (the reference "field token"
     * look). Off = the row sits flat on the panel. */
    filled?: boolean;
    /** Right slot: the control / badge / text value, at its own size. Omit for a
     * label-only row (a filled field token). */
    children?: ReactNode;
    className?: string;
}

export function ControlRow({ label, size = "md", filled = false, children, className }: ControlRowProps) {
    return (
        <div className={cx(styles.row, SIZE_CLASSES[size], filled && styles.filled, className)}>
            <span
                className={cx(styles.label, LABEL_ROLES[size])}
                title={typeof label === "string" ? label : undefined}
            >
                {label}
            </span>
            {children !== undefined && <span className={styles.value}>{children}</span>}
        </div>
    );
}
