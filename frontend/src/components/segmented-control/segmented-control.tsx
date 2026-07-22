// SegmentedControl — the single-select button group (the macOS "segmented control"
// shape): a recessed track holding N segments where exactly one is always lit. Unlike
// Tabs (which carries tabpanel ARIA and swaps mounted panels), this swaps nothing — it
// drives a single value the consumer owns, so it's domain-blind chrome, wired to no
// store. Behavior is RAC's ToggleButtonGroup (selection, roving focus, keyboard); the
// look reuses the ratified chrome register (§7/§29) proven in ToggleButton, kept flat
// per the layering doctrine — no raised-pill shadow, accent nowhere but the focus ring.
//
// Content is per-segment children, so text / icon / icon+text all work with no API
// change (icon-only passes aria-label straight through). Size lives entirely on the
// track: the size class drives segment height + padding via descendant rules, so Segment
// stays a dumb content leaf.

import {
    ToggleButton as AriaToggleButton,
    ToggleButtonGroup as AriaToggleButtonGroup,
    type ToggleButtonProps as AriaToggleButtonProps,
    type Key,
} from "react-aria-components";
import { cx } from "@/lib/cx";
import type { ToggleButtonSize } from "@/components/toggle-button/toggle-button";
import styles from "./segmented-control.module.css";

// C10: exhaustive by construction. Size is the control-height family, shared with
// ToggleButton — control = 24px (dense default), control-lg = 28px.
const SIZE_CLASSES = {
    control: styles.control,
    "control-lg": styles.controlLarge,
} as const satisfies Record<ToggleButtonSize, string>;

export interface SegmentedControlProps {
    /** The lit segment's id (controlled). */
    value?: Key;
    /** The initially lit segment's id (uncontrolled). */
    defaultValue?: Key;
    /** Fires with the newly-lit segment's id. Empty selection is impossible. */
    onChange?: (key: Key) => void;
    /** Disables the whole group (segments keep their fill so state stays readable). */
    isDisabled?: boolean;
    size?: ToggleButtonSize;
    /** Required — the group needs an accessible name (or aria-labelledby). */
    "aria-label"?: string;
    "aria-labelledby"?: string;
    className?: string;
    /** Segment children. */
    children: React.ReactNode;
}

export function SegmentedControl({
    value,
    defaultValue,
    onChange,
    size = "control",
    className,
    children,
    ...labeling
}: SegmentedControlProps) {
    return (
        <AriaToggleButtonGroup
            {...labeling}
            selectionMode="single"
            disallowEmptySelection
            // Adapt RAC's Set-based selection to a friendly single-key API — call sites
            // never touch a Selection set.
            selectedKeys={value !== undefined ? [value] : undefined}
            defaultSelectedKeys={defaultValue !== undefined ? [defaultValue] : undefined}
            onSelectionChange={(keys) => {
                // single + disallowEmptySelection ⇒ exactly one key, always.
                for (const key of keys) {
                    onChange?.(key);
                    return;
                }
            }}
            className={cx(styles.segmentedControl, SIZE_CLASSES[size], className)}
        >
            {children}
        </AriaToggleButtonGroup>
    );
}

export interface SegmentProps extends Omit<AriaToggleButtonProps, "className" | "style"> {
    /** Identifies this segment in the group's selection. */
    id: Key;
    className?: string;
}

export function Segment({ className, ...ariaProps }: SegmentProps) {
    return <AriaToggleButton {...ariaProps} className={cx(styles.segment, className)} />;
}
