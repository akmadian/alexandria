// ControlGroup — a flush stack of ControlRows that share one label column (D34). Its
// job is the grouped-list idiom (Apple HIG / SwiftUI Form Section): rows within a group
// stack flush (§8: space lives inside rows, not between them — separation goes BETWEEN
// groups, which is the parent's gap), and the group owns the shared label-column width
// so every row's label aligns (the property-inspector convention: label width is set at
// the form/group level, not per row).
//
// Presentational: it sets --control-row-label for its rows and stacks them; the rows and
// their hosted controls own their own behavior and accessible names.

import type { CSSProperties, ReactNode } from "react";
import { cx } from "@/lib/cx";
import styles from "./control-group.module.css";

export interface ControlGroupProps {
    /** Shared label-column width every row aligns to — any CSS length; keep ≤ 60% (the
     * row's cap). Default 40%. Set at the group level, per the inspector convention. */
    labelWidth?: string;
    /** The ControlRows. */
    children: ReactNode;
    className?: string;
}

export function ControlGroup({ labelWidth = "40%", children, className }: ControlGroupProps) {
    return (
        <div
            className={cx(styles.group, className)}
            style={{ "--control-row-label": labelWidth } as CSSProperties}
        >
            {children}
        </div>
    );
}
