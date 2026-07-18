// PanelSection — §12's one row grammar for panels: a sentence-case bold head at
// body size (§9, the `head` role), the registered disclosure chevron (§14
// concept, not a glyph choice), and a run of Rows whose intent the SECTION
// chooses (§8: density switches at section boundaries only). React Aria's
// Disclosure owns the expand/collapse behavior and accessibility.
//
// ponytail: collapse is instant — a height transition is motion-round material
// (§26 register-shift budget), not a primitive default.

import {
    Button as AriaButton,
    Disclosure as AriaDisclosure,
    DisclosurePanel as AriaDisclosurePanel,
} from "react-aria-components";
import { Icon } from "@/components/icon/icon";
import { RowIntentProvider, type RowIntent } from "@/components/row/row";
import { cx } from "@/lib/cx";
import styles from "./panel-section.module.css";

export interface PanelSectionProps {
    head: React.ReactNode;
    /** The row intent for every row inside (§8). Rows may still state their own. */
    intent?: RowIntent;
    defaultExpanded?: boolean;
    children: React.ReactNode;
}

export function PanelSection({ head, intent = "control", defaultExpanded = true, children }: PanelSectionProps) {
    return (
        <AriaDisclosure className={styles.section} defaultExpanded={defaultExpanded}>
            <AriaButton slot="trigger" className={styles.head}>
                <Icon concept="disclose" className={styles.chevron} />
                <span className={cx(styles.headText, "alx-type-head")}>{head}</span>
            </AriaButton>
            <AriaDisclosurePanel className={styles.panel}>
                <RowIntentProvider intent={intent}>{children}</RowIntentProvider>
            </AriaDisclosurePanel>
        </AriaDisclosure>
    );
}
