// Row — the registered row grammar (§8/§12; registries.json rowIntents) made
// structural. The intent binds height + inset + the PERMITTED type roles by
// construction: Row renders its own slots with the intent's registered roles,
// so body type in a 16px text row is unrepresentable rather than linted. The
// SECTION chooses the intent (§8: density switches at section boundaries only)
// via context; a standalone row may state its own. Only control rows may carry
// children (the control slot) — the discriminated props make that a compile
// error elsewhere.
//
// ponytail: presentational structure only — interaction states (§25 hover/
// selected) land when the first real consumer (tree, list selection) defines
// them; heights already honor the interactive hit-target floor.

import { createContext, useContext, type ReactNode } from "react";
import { cx } from "@/lib/cx";
import styles from "./row.module.css";

export type RowIntent = "control" | "list" | "text";

const RowIntentContext = createContext<RowIntent | null>(null);

export function RowIntentProvider({ intent, children }: { intent: RowIntent; children: ReactNode }) {
    return <RowIntentContext.Provider value={intent}>{children}</RowIntentContext.Provider>;
}

// C10: every intent maps to its structure class and its registered slot roles —
// the pairings mirror registries.json rowIntents.typeRoles.
const INTENT_CLASSES = {
    control: styles.control,
    list: styles.list,
    text: styles.text,
} as const satisfies Record<RowIntent, string>;

const LABEL_ROLES = {
    control: "alx-type-label",
    list: "alx-type-value",
    text: "alx-type-label-sm",
} as const satisfies Record<RowIntent, string>;

const VALUE_ROLES = {
    control: "alx-type-value",
    list: "alx-type-data-sm",
    text: "alx-type-data-sm",
} as const satisfies Record<RowIntent, string>;

interface RowSlotProps {
    /** Left slot: the row's name, in the intent's registered label role. End-truncates (§13). */
    label?: ReactNode;
    /** Right slot: value/count, in the intent's registered data role (tabular via the role class).
     * End-truncates; string values hover-reveal in full (§13). */
    value?: ReactNode;
    className?: string;
}

export type RowProps = RowSlotProps &
    (
        | { intent: "control"; children?: ReactNode }
        | { intent?: "list" | "text"; children?: never }
    );

export function Row({ label, value, className, ...rest }: RowProps) {
    const inheritedIntent = useContext(RowIntentContext);
    const intent = rest.intent ?? inheritedIntent ?? "control";
    return (
        <div className={cx(styles.row, INTENT_CLASSES[intent], className)}>
            {label !== undefined && (
                <span
                    className={cx(styles.label, LABEL_ROLES[intent])}
                    title={typeof label === "string" ? label : undefined}
                >
                    {label}
                </span>
            )}
            {"children" in rest && rest.children}
            {value !== undefined && (
                <span
                    className={cx(styles.value, VALUE_ROLES[intent])}
                    title={typeof value === "string" ? value : undefined}
                >
                    {value}
                </span>
            )}
        </div>
    );
}
